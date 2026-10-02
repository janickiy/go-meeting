package content

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Service отделяет HTTP чтение/очередь от ограниченной внешней STT/AI обработки.
type Service struct {
	repo  Repository
	audio AudioSource
	stt   domain.TranscriptionProvider
	ai    domain.AIProvider
	cfg   Config
}

// NewService проверяет бюджеты и связывает заменяемые зависимости.
// @parameters: repo — durable данные; audio — приватное извлечение WAV;
// stt/ai — адаптеры без SDK в usecase; cfg — enable flags и конечные лимиты.
// @return готовый сценарий либо ошибка неполной/опасной конфигурации.
func NewService(repo Repository, audio AudioSource, stt domain.TranscriptionProvider, ai domain.AIProvider, cfg Config) (*Service, error) {
	if repo == nil || stt == nil || ai == nil || (cfg.TranscriptionEnabled && audio == nil) {
		return nil, fmt.Errorf("content dependencies are required")
	}
	if cfg.MaxDurationSec < 1 || cfg.MaxDurationSec > 86400 || cfg.MaxSegments < 1 || cfg.MaxSegments > 10000 || cfg.MaxTranscriptBytes < 1024 || cfg.MaxTranscriptBytes > 64<<20 || cfg.ChunkRunes < 512 || cfg.ChunkRunes > 50000 || cfg.MaxChunks < 1 || cfg.MaxChunks > 512 || cfg.AIConcurrency < 1 || cfg.AIConcurrency > 16 || cfg.STTTimeout <= 0 || cfg.STTTimeout > time.Hour || cfg.AITimeout <= 0 || cfg.AITimeout > 30*time.Minute || cfg.ReprocessCooldown < time.Second || cfg.MaxReprocess < 0 || cfg.MaxReprocess > 20 || cfg.MaxAttempts < 1 || cfg.MaxAttempts > 10 {
		return nil, fmt.Errorf("invalid content limits")
	}
	return &Service{repo: repo, audio: audio, stt: stt, ai: ai, cfg: cfg}, nil
}

// Transcript возвращает nullable metadata и серверные capability flags.
// @parameters: ctx — deadline; userID/cid/rid — актор и область чтения.
// @return ready/processing состояние или ошибку доступа.
func (s *Service) Transcript(ctx context.Context, userID, cid, rid string) (domain.TranscriptState, error) {
	t, manage, err := s.repo.Transcript(ctx, userID, cid, rid)
	enabled := s.cfg.TranscriptionEnabled && s.stt.Name() != "noop"
	can := enabled && manage && (t == nil || (t.Status == domain.Failed && t.Generation < int64(s.cfg.MaxReprocess+1) && time.Since(t.UpdatedAt) >= s.cfg.ReprocessCooldown)) && s.cfg.MaxReprocess > 0
	return domain.TranscriptState{Item: t, Enabled: enabled, CanRetry: can, ProviderMode: s.stt.Name()}, err
}

// Segments проверяет bounds перед серверной пагинацией timestamped текста.
// @parameters: ctx — deadline; userID/cid/rid — область чтения; limit/offset — страница.
// @return ограниченная страница и ошибка входа/доступа.
func (s *Service) Segments(ctx context.Context, userID, cid, rid string, limit, offset int) (domain.SegmentPage, error) {
	if limit < 1 || limit > 200 || offset < 0 || offset > 100000 {
		return domain.SegmentPage{}, apperrors.ErrInvalidInput
	}
	return s.repo.Segments(ctx, userID, cid, rid, limit, offset)
}

// Summary возвращает только summary текущего transcript generation и реальные права.
// @parameters: ctx — deadline; userID/cid/rid — область чтения.
// @return nullable summary/enable/canRegenerate и ошибка.
func (s *Service) Summary(ctx context.Context, userID, cid, rid string) (domain.SummaryState, error) {
	summary, manage, err := s.repo.Summary(ctx, userID, cid, rid)
	enabled := s.cfg.AIEnabled && s.ai.Name() != "noop"
	can := false
	if err == nil && enabled && manage {
		t, _, e := s.repo.Transcript(ctx, userID, cid, rid)
		if e != nil {
			return domain.SummaryState{}, e
		}
		can = t != nil && t.Status == domain.Ready && (summary == nil || ((summary.Status == domain.Ready || summary.Status == domain.Failed) && summary.Generation < int64(s.cfg.MaxReprocess+1) && time.Since(summary.UpdatedAt) >= s.cfg.ReprocessCooldown))
	}
	return domain.SummaryState{Item: summary, Enabled: enabled, CanRegenerate: can, ProviderMode: s.ai.Name()}, err
}

// RetryTranscript ставит только разрешённую дорогую обработку в persistent queue.
// @parameters: ctx — короткий HTTP deadline; userID/cid/rid — проверяемый актор/ресурс.
// @return queued state; внешние вызовы здесь отсутствуют.
func (s *Service) RetryTranscript(ctx context.Context, userID, cid, rid string) (domain.TranscriptState, error) {
	if !s.cfg.TranscriptionEnabled || s.stt.Name() == "noop" {
		return domain.TranscriptState{}, apperrors.New(apperrors.ErrUnavailable, "transcription is disabled")
	}
	t, err := s.repo.QueueTranscript(ctx, userID, cid, rid, s.cfg.MaxReprocess, s.cfg.ReprocessCooldown, s.cfg.MaxAttempts)
	return domain.TranscriptState{Item: t, Enabled: true, ProviderMode: s.stt.Name()}, err
}

// RegenerateSummary ставит новое поколение summary с authorization/cost budget в БД.
// @parameters: ctx — короткий HTTP deadline; userID/cid/rid — область доступа.
// @return queued state; transcript/recording остаются пригодными к чтению.
func (s *Service) RegenerateSummary(ctx context.Context, userID, cid, rid string) (domain.SummaryState, error) {
	if !s.cfg.AIEnabled || s.ai.Name() == "noop" {
		return domain.SummaryState{}, apperrors.New(apperrors.ErrUnavailable, "AI summary is disabled")
	}
	item, err := s.repo.QueueSummary(ctx, userID, cid, rid, s.cfg.MaxReprocess, s.cfg.ReprocessCooldown, s.cfg.MaxAttempts)
	return domain.SummaryState{Item: item, Enabled: true, ProviderMode: s.ai.Name()}, err
}

// Search ограничивает запрос и вызывает permission-filtered PostgreSQL FTS.
// @parameters: ctx — deadline; userID — текущий пользователь; query — фильтры/страница.
// @return только разрешённые plain-text результаты и ошибку.
func (s *Service) Search(ctx context.Context, userID string, query domain.SearchQuery) (domain.SearchPage, error) {
	query.Query = strings.TrimSpace(query.Query)
	if query.Source == "" {
		query.Source = "all"
	}
	if !validText(query.Query, 500, false) || len(query.Query) > 2000 || query.Limit < 1 || query.Limit > 100 || query.Offset < 0 || query.Offset > 10000 {
		return domain.SearchPage{}, apperrors.ErrInvalidInput
	}
	if query.Source != "all" && query.Source != "conference" && query.Source != "transcript" && query.Source != "summary" {
		return domain.SearchPage{}, apperrors.ErrInvalidInput
	}
	if query.ConferenceID != "" {
		id, e := uuid.Parse(query.ConferenceID)
		if e != nil || id == uuid.Nil {
			return domain.SearchPage{}, apperrors.ErrInvalidInput
		}
		query.ConferenceID = id.String()
	}
	if (query.From != nil && query.From.IsZero()) || (query.To != nil && query.To.IsZero()) || (query.From != nil && query.To != nil && query.From.After(*query.To)) {
		return domain.SearchPage{}, apperrors.ErrInvalidInput
	}
	return s.repo.Search(ctx, userID, query)
}

// Handle обрабатывает одно leased задание, никогда не изменяя media recording lifecycle.
// @parameters: ctx — product-worker deadline; job — постоянный job с generation/lease.
// @return классифицированную ошибку, ErrSkip либо успешный commit.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	switch job.Kind {
	case "content.transcribe":
		if !s.cfg.TranscriptionEnabled || s.stt.Name() == "noop" {
			if err := s.repo.FailJob(ctx, job, "processing_disabled"); err != nil {
				return err
			}
			return jobs.ErrSkip
		}
		ctx, cancel := context.WithTimeout(ctx, s.cfg.STTTimeout)
		defer cancel()
		t, source, err := s.repo.StartTranscript(ctx, job)
		if err != nil {
			return err
		}
		if source.DurationSec < 1 || source.DurationSec > s.cfg.MaxDurationSec {
			return &jobs.Error{Code: "recording_ineligible", Retryable: false}
		}
		audio, err := s.audio.Open(ctx, source)
		if err != nil {
			return providerError(err, "audio_unavailable")
		}
		if audio.Cleanup != nil {
			defer audio.Cleanup()
		}
		if audio.Reader == nil {
			return &jobs.Error{Code: "invalid_audio", Retryable: false}
		}
		defer audio.Reader.Close()
		result, err := s.stt.Transcribe(ctx, domain.TranscriptionRequest{Audio: audio.Reader, Size: audio.Size, ContentType: audio.ContentType, Language: "auto", IdempotencyKey: job.DedupKey, DurationSec: source.DurationSec})
		if err != nil {
			return providerError(err, "stt_unavailable")
		}
		if err = ValidateTranscript(&result, source.DurationSec, s.cfg.MaxSegments, s.cfg.MaxTranscriptBytes); err != nil {
			return &jobs.Error{Code: "invalid_transcript", Retryable: false}
		}
		return s.repo.SaveTranscript(ctx, job, t, result, s.stt.Name(), s.cfg.AIEnabled && s.ai.Name() != "noop", s.cfg.MaxAttempts)
	case "content.summarize":
		if !s.cfg.AIEnabled || s.ai.Name() == "noop" {
			if err := s.repo.FailJob(ctx, job, "processing_disabled"); err != nil {
				return err
			}
			return jobs.ErrSkip
		}
		ctx, cancel := context.WithTimeout(ctx, s.cfg.AITimeout)
		defer cancel()
		summary, segments, err := s.repo.StartSummary(ctx, job)
		if err != nil {
			return err
		}
		// Отсутствие распознанной речи — валидный пустой результат, а не повод
		// платить за AI-запрос или придумывать содержание встречи.
		if len(segments) == 0 {
			empty := domain.SummaryOutput{KeyPoints: []string{}, ActionItems: []domain.ActionItem{}, Topics: []string{}}
			return s.repo.SaveSummary(ctx, job, summary, empty, "none", "", PromptVersion, SchemaVersion, s.cfg.MaxAttempts)
		}
		output, err := s.summarize(ctx, job.DedupKey, segments)
		if err != nil {
			return err
		}
		return s.repo.SaveSummary(ctx, job, summary, output, s.ai.Name(), s.ai.Model(), PromptVersion, SchemaVersion, s.cfg.MaxAttempts)
	default:
		return jobs.ErrSkip
	}
}

// FailJob фиксирует terminal content status до освобождения job lease общим runner.
// @parameters: ctx — свежий bounded deadline; job — актуальный job; code — техническая категория.
// @return безопасную ошибку DB commit/fencing.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	return s.repo.FailJob(ctx, job, code)
}

// providerError сохраняет классифицированные ошибки, скрывая vendor diagnostics.
// @parameters: err — исходная ошибка; code — безопасная категория для прочих ошибок.
// @return Error с конечным retry policy, без transcript/token в сообщении.
func providerError(err error, code string) error {
	if errors.Is(err, jobs.ErrSkip) || errors.Is(err, jobs.ErrLeaseLost) {
		return err
	}
	var p *jobs.Error
	if errors.As(err, &p) {
		return p
	}
	var value jobs.Error
	if errors.As(err, &value) {
		return value
	}
	return &jobs.Error{Code: code, Retryable: true}
}

// ValidateTranscript проверяет размер, временную монотонность и confidence поставщика.
// @parameters: result — изменяемый STT output; duration — длина записи; maxSegments/maxBytes — бюджеты.
// @return ошибку схемы; speaker identity очищается, потому что надёжного mapping нет.
func ValidateTranscript(result *domain.TranscriptionResult, duration, maxSegments int, maxBytes int64) error {
	if result == nil || duration < 1 || len(result.Segments) > maxSegments {
		return fmt.Errorf("invalid transcript size")
	}
	if result.Language == "" {
		result.Language = "auto"
	}
	if len(result.Language) > 32 || !utf8.ValidString(result.Language) {
		return fmt.Errorf("invalid language")
	}
	for _, ch := range result.Language {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-') {
			return fmt.Errorf("invalid language")
		}
	}
	var total int64
	var previous int64 = -1
	for i := range result.Segments {
		seg := &result.Segments[i]
		seg.Text = strings.TrimSpace(seg.Text)
		total += int64(len(seg.Text))
		seg.ID = ""
		seg.TranscriptID = ""
		seg.SpeakerID = nil
		if seg.StartMS < 0 || seg.StartMS < previous || seg.EndMS < seg.StartMS || seg.EndMS > int64(duration)*1000+1000 || !validText(seg.Text, 20000, false) || total > maxBytes {
			return fmt.Errorf("invalid segment")
		}
		previous = seg.StartMS
		if seg.SpeakerLabel != nil && !validText(*seg.SpeakerLabel, 100, true) {
			return fmt.Errorf("invalid speaker label")
		}
		if seg.Confidence != nil && (math.IsNaN(*seg.Confidence) || math.IsInf(*seg.Confidence, 0) || *seg.Confidence < 0 || *seg.Confidence > 1) {
			return fmt.Errorf("invalid confidence")
		}
	}
	return nil
}
