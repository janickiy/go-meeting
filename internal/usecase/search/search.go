// Package search координирует ограниченную индексацию и поиск с проверкой прав в repository.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	content "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	domain "github.com/janickiy/go-recorder/internal/domain/search"
	"github.com/janickiy/go-recorder/internal/operations"
	"time"
)

// Repository сохраняет результаты только с актуальными lease/generation и фильтрует доступ внутри SQL.
type Repository interface {
	Available(context.Context) (bool, error)
	Source(context.Context, jobs.Job) (content.Transcript, []content.Segment, error)
	Cached(context.Context, string, []string) (map[string][]float32, error)
	Save(context.Context, jobs.Job, config.StageEightConfig, []domain.Chunk) error
	Fail(context.Context, jobs.Job, config.StageEightConfig, string) error
	Search(context.Context, string, content.SearchQuery, string, []float32) (content.SearchPage, error)
}

// Service разделяет обработчик durable jobs и ограниченное векторизование пользовательского запроса.
type Service struct {
	Repo       Repository
	Provider   domain.EmbeddingProvider
	Config     config.StageEightConfig
	querySlots chan struct{}
}

// New создаёт сценарий с конечным числом одновременных query embeddings.
// @args repo — SQL; provider — внешний adapter; cfg — лимиты и идентичность модели.
// @return сервис, не выполняющий сеть при создании.
func New(repo Repository, provider domain.EmbeddingProvider, cfg config.StageEightConfig) *Service {
	return &Service{repo, provider, cfg, make(chan struct{}, cfg.EmbeddingWorkers)}
}

// ModelKey включает модель, версию и размерность, исключая смешивание разных пространств.
// @args cfg — настройки embedding модели.
// @return стабильный SHA-256 идентичности.
func ModelKey(cfg config.StageEightConfig) string {
	raw, _ := json.Marshal([]any{cfg.EmbeddingModel, cfg.EmbeddingVersion, cfg.EmbeddingDimensions})
	return Hash(string(raw))
}

// Hash вычисляет content identity без включения исходного текста в jobs/логи.
// @args text — ограниченный UTF-8 фрагмент.
// @return hex SHA-256.
func Hash(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }

// Chunks детерминированно разделяет каждый сегмент, сохраняя speaker и timestamps.
// @args transcript — поколение; segments — ordered источники; cfg — предел и модель.
// @return chunks не более 20000 либо ошибка бюджета.
func Chunks(transcript content.Transcript, segments []content.Segment, cfg config.StageEightConfig) ([]domain.Chunk, error) {
	result := []domain.Chunk{}
	model := ModelKey(cfg)
	for _, segment := range segments {
		runes := []rune(segment.Text)
		for offset := 0; offset < len(runes); offset += cfg.EmbeddingChunkRunes {
			text := strings.TrimSpace(string(runes[offset:min(len(runes), offset+cfg.EmbeddingChunkRunes)]))
			if text == "" {
				continue
			}
			if len(result) >= 20000 {
				return nil, jobs.Error{Code: "embedding_chunk_limit"}
			}
			speaker := ""
			if segment.SpeakerLabel != nil {
				speaker = *segment.SpeakerLabel
			}
			result = append(result, domain.Chunk{ID: uuid.NewSHA1(uuid.MustParse(transcript.ID), []byte(fmt.Sprintf("%d:%s:%d", transcript.Generation, model, len(result)))).String(), ConferenceID: transcript.ConferenceID, RecordingID: transcript.RecordingID, TranscriptID: transcript.ID, SegmentID: segment.ID, Generation: transcript.Generation, Ordinal: len(result), StartMS: segment.StartMS, EndMS: segment.EndMS, SpeakerID: segment.SpeakerID, Speaker: speaker, Text: text, ContentHash: Hash(text), ModelKey: model})
		}
	}
	return result, nil
}

// ValidVectors отклоняет неверный batch, NaN/Infinity и нулевые векторы до pgvector.
// @args vectors — непроверенный output; count,dimensions — ожидаемая форма.
// @return true при корректном конечном результате.
func ValidVectors(vectors [][]float32, count, dimensions int) bool {
	if len(vectors) != count {
		return false
	}
	for _, vector := range vectors {
		if len(vector) != dimensions {
			return false
		}
		var norm float64
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return false
			}
			norm += float64(value) * float64(value)
		}
		if norm <= 0 {
			return false
		}
	}
	return true
}

// Handle переиспользует content-hash cache и обрабатывает bounded batches вне recording lifecycle.
// @args ctx — deadline leased job; job — transcript generation и токен.
// @return ошибка провайдера/fencing либо успешная атомарная публикация индекса.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	started := time.Now()
	defer func() { operations.Observe("embedding", time.Since(started).Seconds()) }()
	if !s.Config.EmbeddingsEnabled {
		return jobs.ErrSkip
	}
	available, err := s.Repo.Available(ctx)
	if err != nil {
		return err
	}
	if !available {
		return jobs.ErrSkip
	}
	transcript, segments, err := s.Repo.Source(ctx, job)
	if err != nil {
		return err
	}
	chunks, err := Chunks(transcript, segments, s.Config)
	if err != nil {
		return err
	}
	for offset := 0; offset < len(chunks); offset += s.Config.EmbeddingBatch {
		batch := chunks[offset:min(len(chunks), offset+s.Config.EmbeddingBatch)]
		hashes := make([]string, len(batch))
		for i := range batch {
			hashes[i] = batch[i].ContentHash
		}
		cache, err := s.Repo.Cached(ctx, ModelKey(s.Config), hashes)
		if err != nil {
			return err
		}
		inputs := []string{}
		positions := []int{}
		for i := range batch {
			if vector, ok := cache[batch[i].ContentHash]; ok && ValidVectors([][]float32{vector}, 1, s.Config.EmbeddingDimensions) {
				batch[i].Vector = vector
			} else {
				inputs = append(inputs, batch[i].Text)
				positions = append(positions, i)
			}
		}
		if len(inputs) == 0 {
			continue
		}
		started := time.Now()
		vectors, err := s.Provider.Embed(ctx, domain.EmbeddingRequest{Model: s.Config.EmbeddingModel, Version: s.Config.EmbeddingVersion, Dimensions: s.Config.EmbeddingDimensions, Inputs: inputs, IdempotencyKey: Hash(ModelKey(s.Config) + strings.Join(hashes, ":"))})
		operations.ProviderCall("embedding", time.Since(started), err)
		if err != nil {
			return err
		}
		if !ValidVectors(vectors, len(inputs), s.Config.EmbeddingDimensions) {
			return jobs.Error{Code: "embedding_schema"}
		}
		for i, index := range positions {
			batch[index].Vector = vectors[i]
		}
	}
	return s.Repo.Save(ctx, job, s.Config, chunks)
}

// FailJob фиксирует только безопасный код отказа индекса, не меняя расшифровку.
// @args ctx — deadline; job — lease; code — классификация.
// @return ошибка SQL.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	return s.Repo.Fail(ctx, job, s.Config, code)
}

// Search векторизует только поисковый запрос; при недоступности использует тот же авторизованный FTS.
// @args ctx — HTTP deadline; user — актор; query — уже проверенные фильтры.
// @return результаты с явным effectiveMode/fallbackReason.
func (s *Service) Search(ctx context.Context, user string, query content.SearchQuery) (content.SearchPage, error) {
	started := time.Now()
	defer func() { operations.Observe("hybrid_search", time.Since(started).Seconds()) }()
	fallback := func(reason string) (content.SearchPage, error) {
		query.Mode = "keyword"
		page, e := s.Repo.Search(ctx, user, query, ModelKey(s.Config), nil)
		page.EffectiveMode = "keyword"
		page.FallbackReason = reason
		return page, e
	}
	if query.Mode == "" || query.Mode == "keyword" {
		return fallback("")
	}
	if query.Mode == "semantic" && (query.Source == "conference" || query.Source == "summary") {
		return fallback("source_keyword_only")
	}
	if !s.Config.EmbeddingsEnabled {
		return fallback("semantic_disabled")
	}
	available, err := s.Repo.Available(ctx)
	if err != nil || !available {
		return fallback("vector_unavailable")
	}
	select {
	case s.querySlots <- struct{}{}:
		defer func() { <-s.querySlots }()
	default:
		return fallback("provider_busy")
	}
	op, cancel := context.WithTimeout(ctx, min(s.Config.EmbeddingTimeout, 3*time.Second))
	defer cancel()
	start := time.Now()
	vectors, err := s.Provider.Embed(op, domain.EmbeddingRequest{Model: s.Config.EmbeddingModel, Version: s.Config.EmbeddingVersion, Dimensions: s.Config.EmbeddingDimensions, Inputs: []string{query.Query}, IdempotencyKey: "query:" + Hash(ModelKey(s.Config)+query.Query)})
	operations.ProviderCall("embedding", time.Since(start), err)
	if err != nil || !ValidVectors(vectors, 1, s.Config.EmbeddingDimensions) {
		return fallback("provider_unavailable")
	}
	page, err := s.Repo.Search(ctx, user, query, ModelKey(s.Config), vectors[0])
	if err != nil {
		return fallback("vector_unavailable")
	}
	page.EffectiveMode = query.Mode
	return page, nil
}
