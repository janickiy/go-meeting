package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"github.com/janickiy/go-recorder/internal/infrastructure/contentproviders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	contentusecase "github.com/janickiy/go-recorder/internal/usecase/content"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stageSevenAudio явно заменяет audio extraction в DB orchestration test, не выдавая fake за STT.
type stageSevenAudio struct{}

// Open выдаёт bounded test reader без доступа к пользовательским файлам.
// @parameters: ctx/source — worker contract fixture.
// @return reader, закрываемый сценарием.
func (stageSevenAudio) Open(context.Context, domain.RecordingSource) (domain.Audio, error) {
	return domain.Audio{Reader: io.NopCloser(strings.NewReader("test-WAV")), Size: 8, ContentType: "audio/wav"}, nil
}

// stageSevenContentFixture содержит отдельную БД, принятых участников и ready recording.
type stageSevenContentFixture struct {
	db                      *gorm.DB
	owner, member, outsider users.User
	conference              conferences.View
	recording               records.Record
	service                 *contentusecase.Service
	repo                    *pg.ContentRepository
	jobs                    *pg.JobRepository
}

// stageSevenContent подготавливает полный durable orchestration без платных провайдеров.
// @parameters: t — test runner, stageOneDatabase создаёт точную отдельную БД.
// @return fixture с явно mock providers и реальной PostgreSQL queue/FTS.
func stageSevenContent(t *testing.T) *stageSevenContentFixture {
	t.Helper()
	f := &stageSevenContentFixture{db: stageOneDatabase(t)}
	ctx := context.Background()
	ur := pg.NewUserRepository(f.db)
	for _, user := range []*users.User{&f.owner, &f.member, &f.outsider} {
		*user = users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@stage7.example", PasswordHash: "test-only-not-used-for-login"}
		if _, e := ur.Create(ctx, *user); e != nil {
			t.Fatal(e)
		}
	}
	cs := conferenceusecase.NewService(pg.NewConferenceRepository(f.db), ur, security.GenerateInviteCode)
	var e error
	f.conference, e = cs.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Этап семь English search проект"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Join(ctx, f.owner.ID, f.conference.ID, conferences.JoinRequest{}); e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Join(ctx, f.member.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode}); e != nil {
		t.Fatal(e)
	}
	if _, e = cs.Transition(ctx, f.owner.ID, f.conference.ID, conferences.Active); e != nil {
		t.Fatal(e)
	}
	f.recording, _, e = pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, f.conference.ID, 5)
	if e != nil {
		t.Fatal(e)
	}
	key := "recordings/" + f.conference.ID + "/" + f.recording.UUID + "/artifacts/" + uuid.NewString() + "/final.mp4"
	if e = f.db.Model(&records.Record{}).Where("uuid=?", f.recording.UUID).Updates(map[string]any{"status": records.StatusReady, "storage_object_key": key, "size_bytes": 100, "duration_sec": 10}).Error; e != nil {
		t.Fatal(e)
	}
	f.repo = pg.NewContentRepository(f.db)
	f.jobs = pg.NewJobRepository(f.db)
	cfg := contentusecase.DefaultConfig()
	cfg.TranscriptionEnabled = true
	cfg.AIEnabled = true
	cfg.ReprocessCooldown = time.Second
	stt, _ := contentproviders.NewTranscriptionProvider("mock", "", "", time.Second)
	ai, _ := contentproviders.NewAIProvider("mock", "", "", "", time.Second)
	f.service, e = contentusecase.NewService(f.repo, stageSevenAudio{}, stt, ai, cfg)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

// contentClaim требует настоящую bounded lease из общей PostgreSQL queue.
// @parameters: t — runner; f — отдельная fixture; kind — фиксированный job kind.
// @return один действующий job, пригодный для fenced commit.
func contentClaim(t *testing.T, f *stageSevenContentFixture, kind string) jobs.Job {
	t.Helper()
	job, ok, e := f.jobs.Claim(context.Background(), kind, time.Minute)
	if e != nil || !ok {
		t.Fatal("missing queued job", kind, e)
	}
	return job
}

// TestStageSevenContentPipelineAndSearch проверяет ready→STT→AI→notifications и permission-first FTS.
// @parameters: t — runner, live provider не используется.
func TestStageSevenContentPipelineAndSearch(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	cid, rid := f.conference.ID, f.recording.UUID
	initial, e := f.service.Transcript(ctx, f.member.ID, cid, rid)
	if e != nil || initial.Item != nil || !initial.Enabled || initial.CanRetry {
		t.Fatal(initial, e)
	}
	job := contentClaim(t, f, "content.transcribe")
	if e = f.service.Handle(ctx, job); e != nil {
		t.Fatal(e)
	}
	// Повтор обработки после commit, до ACK/Finish не создаёт второй transcript/AI job.
	if e = f.service.Handle(ctx, job); !errors.Is(e, jobs.ErrSkip) {
		t.Fatal("duplicate STT not skipped", e)
	}
	if e = f.jobs.Finish(ctx, job, "done", "", nil); e != nil {
		t.Fatal(e)
	}
	state, e := f.service.Transcript(ctx, f.member.ID, cid, rid)
	if e != nil || state.Item == nil || state.Item.Status != domain.Ready || state.Item.Provider != "mock" {
		t.Fatal(state, e)
	}
	page, e := f.service.Segments(ctx, f.member.ID, cid, rid, 10, 0)
	if e != nil || len(page.Items) != 1 || page.Items[0].StartMS != 0 {
		t.Fatal(page, e)
	}
	aiJob := contentClaim(t, f, "content.summarize")
	if e = f.service.Handle(ctx, aiJob); e != nil {
		t.Fatal(e)
	}
	if e = f.jobs.Finish(ctx, aiJob, "done", "", nil); e != nil {
		t.Fatal(e)
	}
	summary, e := f.service.Summary(ctx, f.member.ID, cid, rid)
	if e != nil || summary.Item == nil || summary.Item.Status != domain.Ready || !strings.Contains(summary.Item.Summary, "ТЕСТОВАЯ") {
		t.Fatal(summary, e)
	}
	var counts int64
	if e = f.db.Table("background_jobs").Where("kind='integrations.event'").Count(&counts).Error; e != nil || counts != 2 {
		t.Fatal("ready notifications not atomic/dedup", counts, e)
	}
	search, e := f.service.Search(ctx, f.member.ID, domain.SearchQuery{Query: "обсуждение", Source: "transcript", Limit: 20})
	if e != nil || len(search.Items) != 1 || search.Items[0].RecordingID == nil || *search.Items[0].RecordingID != rid || search.Items[0].StartMS == nil || search.Items[0].SegmentID == nil {
		t.Fatal(search, e)
	}
	english, e := f.service.Search(ctx, f.member.ID, domain.SearchQuery{Query: "English", Source: "conference", Limit: 20})
	if e != nil || len(english.Items) != 1 {
		t.Fatal(english, e)
	}
	outsider, e := f.service.Search(ctx, f.outsider.ID, domain.SearchQuery{Query: "обсуждение", Source: "all", Limit: 20})
	if e != nil || outsider.Total != 0 || len(outsider.Items) != 0 {
		t.Fatal("unauthorized FTS leaked", outsider, e)
	}
	if _, e = f.service.Transcript(ctx, f.outsider.ID, cid, rid); !errors.Is(e, apperrors.ErrForbidden) {
		t.Fatal("IDOR transcript", e)
	}
	if _, e = f.service.RetryTranscript(ctx, f.member.ID, cid, rid); !errors.Is(e, apperrors.ErrForbidden) {
		t.Fatal("participant allowed costly retry", e)
	}
	if e = f.db.Exec("UPDATE conference_participants SET admission_state='kicked',status='kicked' WHERE conference_id=? AND user_id=?", cid, f.member.ID).Error; e != nil {
		t.Fatal(e)
	}
	search, e = f.service.Search(ctx, f.member.ID, domain.SearchQuery{Query: "обсуждение", Source: "all", Limit: 20})
	if e != nil || search.Total != 0 {
		t.Fatal("kicked membership leaked", search, e)
	}
	if e = f.db.Exec("UPDATE record SET deleted_at=now() WHERE uuid=?", rid).Error; e != nil {
		t.Fatal(e)
	}
	search, e = f.service.Search(ctx, f.owner.ID, domain.SearchQuery{Query: "обсуждение", Source: "all", Limit: 20})
	if e != nil || search.Total != 0 {
		t.Fatal("deleted recording leaked transcript", search, e)
	}
	if _, e = f.service.Transcript(ctx, f.owner.ID, cid, rid); !errors.Is(e, apperrors.ErrNotFound) {
		t.Fatal("deleted recording accessible", e)
	}
}

// TestStageSevenContentFailureFencingAndReprocess проверяет изоляцию ошибки и bounded retry generation.
// @parameters: t — runner с отдельной БД.
func TestStageSevenContentFailureFencingAndReprocess(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	job := contentClaim(t, f, "content.transcribe")
	if _, _, e := f.repo.StartTranscript(ctx, job); e != nil {
		t.Fatal(e)
	}
	stale := job
	stale.LeaseToken = uuid.NewString()
	if e := f.repo.SaveTranscript(ctx, stale, domain.Transcript{}, domain.TranscriptionResult{}, "mock", false, 5); !errors.Is(e, jobs.ErrLeaseLost) {
		t.Fatal("stale worker accepted", e)
	}
	if e := f.service.FailJob(ctx, job, "provider_rejected"); e != nil {
		t.Fatal(e)
	}
	if e := f.jobs.Finish(ctx, job, "failed", "provider_rejected", nil); e != nil {
		t.Fatal(e)
	}
	state, e := f.service.Transcript(ctx, f.owner.ID, f.conference.ID, f.recording.UUID)
	if e != nil || state.Item.Status != domain.Failed {
		t.Fatal(state, e)
	}
	var record records.Record
	if e = f.db.Where("uuid=?", f.recording.UUID).Take(&record).Error; e != nil || record.Status != records.StatusReady {
		t.Fatal("STT error changed recording", record, e)
	}
	if _, e = f.service.RetryTranscript(ctx, f.owner.ID, f.conference.ID, f.recording.UUID); !errors.Is(e, apperrors.ErrConflict) {
		t.Fatal("cooldown ignored", e)
	}
	if e = f.db.Table("transcripts").Where("id=?", state.Item.ID).Update("updated_at", time.Now().Add(-time.Minute)).Error; e != nil {
		t.Fatal(e)
	}
	retried, e := f.service.RetryTranscript(ctx, f.owner.ID, f.conference.ID, f.recording.UUID)
	if e != nil || retried.Item.Generation != 2 {
		t.Fatal(retried, e)
	}
	if _, e = f.service.RetryTranscript(ctx, f.owner.ID, f.conference.ID, f.recording.UUID); !errors.Is(e, apperrors.ErrConflict) {
		t.Fatal("duplicate expensive reprocess", e)
	}
	newJob := contentClaim(t, f, "content.transcribe")
	if newJob.Version != 2 {
		t.Fatal(newJob)
	}
	if e = f.service.Handle(ctx, newJob); e != nil {
		t.Fatal(e)
	}
	// Ранее завершённое поколение не может затереть новый успешный результат.
	if e = f.service.FailJob(ctx, job, "exhausted"); !errors.Is(e, jobs.ErrLeaseLost) {
		t.Fatal("old lease failure accepted", e)
	}
}

// TestStageSevenReadyTriggerStrictAndTransactional проверяет strict ready/rollback/no-backfill.
// @parameters: t — runner с isolated DB.
func TestStageSevenReadyTriggerStrictAndTransactional(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	record, _, e := pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, f.conference.ID, 5)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.db.Model(&record).Update("status", records.StatusPartialReady).Error; e != nil {
		t.Fatal(e)
	}
	var count int64
	f.db.Table("background_jobs").Where("entity_id=? AND kind='content.transcribe'", record.UUID).Count(&count)
	if count != 0 {
		t.Fatal("partial_ready triggered STT")
	}
	rollback := errors.New("test rollback")
	e = f.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&record).Update("status", records.StatusReady).Error; e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(e, rollback) {
		t.Fatal(e)
	}
	f.db.Table("background_jobs").Where("entity_id=? AND kind='content.transcribe'", record.UUID).Count(&count)
	if count != 0 {
		t.Fatal("rolled back recording produced STT job")
	}
	for range 3 {
		if e = f.db.Model(&record).Update("status", records.StatusReady).Error; e != nil {
			t.Fatal(e)
		}
	}
	f.db.Table("background_jobs").Where("entity_id=? AND kind='content.transcribe'", record.UUID).Count(&count)
	if count != 1 {
		t.Fatal("ready trigger duplicated", count)
	}
	var payload struct{ Payload json.RawMessage }
	if e = f.db.Raw("SELECT payload FROM background_jobs WHERE entity_id=? AND kind='content.transcribe'", record.UUID).Scan(&payload).Error; e != nil {
		t.Fatal(e)
	}
	if string(payload.Payload) != "{}" {
		t.Fatal("ready job unexpectedly contains private text", string(payload.Payload))
	}
	if e = f.db.Exec("DELETE FROM background_jobs WHERE entity_id=? AND kind='content.transcribe'", record.UUID).Error; e != nil {
		t.Fatal(e)
	}
	if e = pg.RunMigrations(f.db, filepath.Join("..", "..", "database", "migrations")); e != nil {
		t.Fatal(e)
	}
	f.db.Table("background_jobs").Where("entity_id=? AND kind='content.transcribe'", record.UUID).Count(&count)
	if count != 0 {
		t.Fatal("startup replayed historical recording into STT")
	}
}

// TestStageSevenAIFailurePreservesTranscript проверяет schema failure из fake HTTP provider.
// @parameters: t — runner; платные provider calls отсутствуют.
func TestStageSevenAIFailurePreservesTranscript(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	sttJob := contentClaim(t, f, "content.transcribe")
	if e := f.service.Handle(ctx, sttJob); e != nil {
		t.Fatal(e)
	}
	if e := f.jobs.Finish(ctx, sttJob, "done", "", nil); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"summary":"command","keyPoints":[],"actionItems":[],"topics":[],"execute":"https://attacker.invalid"}`)
	}))
	defer server.Close()
	stt, _ := contentproviders.NewTranscriptionProvider("mock", "", "", time.Second)
	ai, _ := contentproviders.NewAIProvider("http", server.URL, "test-only-secret", "model", time.Second)
	cfg := contentusecase.DefaultConfig()
	cfg.TranscriptionEnabled = true
	cfg.AIEnabled = true
	service, e := contentusecase.NewService(f.repo, stageSevenAudio{}, stt, ai, cfg)
	if e != nil {
		t.Fatal(e)
	}
	job := contentClaim(t, f, "content.summarize")
	e = service.Handle(ctx, job)
	var failure *jobs.Error
	if !errors.As(e, &failure) || failure.Code != "invalid_summary" || failure.Retryable {
		t.Fatal("untrusted schema accepted", e)
	}
	if e = service.FailJob(ctx, job, failure.Code); e != nil {
		t.Fatal(e)
	}
	if e = f.jobs.Finish(ctx, job, "failed", failure.Code, nil); e != nil {
		t.Fatal(e)
	}
	transcript, e := service.Transcript(ctx, f.member.ID, f.conference.ID, f.recording.UUID)
	if e != nil || transcript.Item.Status != domain.Ready {
		t.Fatal("AI failure damaged transcript", transcript, e)
	}
	summary, e := service.Summary(ctx, f.owner.ID, f.conference.ID, f.recording.UUID)
	if e != nil || summary.Item.Status != domain.Failed || summary.Item.Summary != "" {
		t.Fatal(summary, e)
	}
	var count int64
	if e = f.db.Table("background_jobs").Where("kind='integrations.event' AND payload->>'event'='summary.failed' AND user_id=?", f.owner.ID).Count(&count).Error; e != nil || count != 1 {
		t.Fatal("owner failure notification missing", count, e)
	}
}

// TestStageSevenReprocessRevocationRace сериализует revoke и дорогой POST через conference lock.
// @parameters: t — runner; проверяется реальный перекрывающийся DB/HTTP-usecase порядок.
func TestStageSevenReprocessRevocationRace(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	job := contentClaim(t, f, "content.transcribe")
	if _, _, e := f.repo.StartTranscript(ctx, job); e != nil {
		t.Fatal(e)
	}
	if e := f.service.FailJob(ctx, job, "provider_rejected"); e != nil {
		t.Fatal(e)
	}
	if e := f.jobs.Finish(ctx, job, "failed", "provider_rejected", nil); e != nil {
		t.Fatal(e)
	}
	if e := f.db.Table("transcripts").Where("recording_id=?", f.recording.UUID).Update("updated_at", time.Now().Add(-time.Minute)).Error; e != nil {
		t.Fatal(e)
	}
	if e := f.db.Exec("UPDATE conference_participants SET role='co_host' WHERE conference_id=? AND user_id=?", f.conference.ID, f.member.ID).Error; e != nil {
		t.Fatal(e)
	}
	tx := f.db.Begin()
	defer tx.Rollback()
	if e := tx.Exec("SELECT id FROM conferences WHERE id=? FOR UPDATE", f.conference.ID).Error; e != nil {
		t.Fatal(e)
	}
	if e := tx.Exec("UPDATE conference_participants SET role='participant' WHERE conference_id=? AND user_id=?", f.conference.ID, f.member.ID).Error; e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := f.service.RetryTranscript(ctx, f.member.ID, f.conference.ID, f.recording.UUID)
		done <- e
	}()
	select {
	case e := <-done:
		t.Fatal("reprocess did not wait for current authorization commit", e)
	case <-time.After(100 * time.Millisecond):
	}
	if e := tx.Commit().Error; e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, apperrors.ErrForbidden) {
			t.Fatal("revoked cohost queued paid work", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reprocess stuck after authorization commit")
	}
	var count int64
	if e := f.db.Table("background_jobs").Where("kind='content.transcribe' AND entity_id=?", f.recording.UUID).Count(&count).Error; e != nil || count != 1 {
		t.Fatal("revoked cohost created expensive generation", count, e)
	}
}
