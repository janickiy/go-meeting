package integration_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	caption "github.com/janickiy/go-recorder/internal/domain/captions"
	content "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/infrastructure/contentproviders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	analytics "github.com/janickiy/go-recorder/internal/usecase/analytics"
	search "github.com/janickiy/go-recorder/internal/usecase/search"
	"testing"
	"time"
)

// TestStageEightCaptionsReconciliationAnalytics проверяет права, защиту аренды, постоянный курсор и каноническую расшифровку.
// @args t — исполнитель; создаётся и удаляется только отдельная тестовая БД.
func TestStageEightCaptionsReconciliationAnalytics(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	cid := f.conference.ID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(f.db.Exec(`UPDATE conferences SET started_at=clock_timestamp()-interval '10 seconds' WHERE id=?`, cid).Error)
	var pid string
	must(f.db.Raw(`SELECT id FROM conference_participants WHERE conference_id=? AND user_id=?`, cid, f.member.ID).Scan(&pid).Error)
	repo := pg.NewCaptionsRepository(f.db)
	if _, err := repo.Set(ctx, f.member.ID, cid, true, "ru"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("member enabled paid STT", err)
	}
	state, err := repo.Set(ctx, f.owner.ID, cid, true, "ru")
	must(err)
	repeat, err := repo.Set(ctx, f.owner.ID, cid, true, "ru")
	must(err)
	if state.Generation != repeat.Generation {
		t.Fatal("duplicate opt-in changed generation")
	}
	lease, err := repo.Claim(ctx, true, 2, 3)
	must(err)
	if _, err = repo.Claim(ctx, false, 2, 3); err == nil {
		t.Fatal("duplicate lease")
	}
	text := "ТЕСТОВАЯ РАСШИФРОВКА: обсуждение проекта и следующих шагов. Это демонстрационный текст, не результат распознавания аудио."
	c := caption.Caption{ID: uuid.NewString(), ParticipantID: pid, TrackInstanceID: uuid.NewString(), Event: caption.Event{UtteranceID: "one", Sequence: 2, Revision: 2, StartMS: 0, EndMS: 5000, Text: text, Language: "ru", Final: true}}
	saved, err := repo.SaveFinal(ctx, lease, c, 100)
	must(err)
	if saved.Cursor <= 0 {
		t.Fatal("no durable cursor")
	}
	page, err := repo.Finals(ctx, f.member.ID, cid, 0, 10)
	must(err)
	if len(page) != 1 || !page[0].Final || page[0].Text != text {
		t.Fatalf("final lost %#v", page)
	}
	if _, err = repo.Finals(ctx, f.outsider.ID, cid, 0, 10); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("caption IDOR", err)
	}
	c.Revision = 3
	c.Sequence = 3
	saved, err = repo.SaveFinal(ctx, lease, c, 100)
	must(err)
	changed, err := repo.Finals(ctx, f.member.ID, cid, page[0].Cursor, 10)
	must(err)
	if len(changed) != 1 || changed[0].Cursor <= page[0].Cursor {
		t.Fatal("revision cannot recover")
	}
	must(f.db.Exec(`UPDATE record SET media_started_at=? WHERE uuid=?`, lease.Origin, f.recording.UUID).Error)
	must(f.service.Handle(ctx, contentClaim(t, f, "content.transcribe")))
	segments, err := f.service.Segments(ctx, f.member.ID, cid, f.recording.UUID, 20, 0)
	must(err)
	if len(segments.Items) != 1 || segments.Items[0].SpeakerID == nil || *segments.Items[0].SpeakerID != pid {
		t.Fatalf("speaker reconciliation failed %+v", segments.Items)
	}
	state, err = repo.Read(ctx, f.member.ID, cid)
	must(err)
	if state.CanonicalRecordingID == nil || *state.CanonicalRecordingID != f.recording.UUID {
		t.Fatal("canonical missing")
	}
	// Два соединения одного человека пересекаются: присутствие должно быть объединением, не суммой.
	for _, span := range [][2]int{{0, 6}, {4, 8}} {
		must(f.db.Exec(`INSERT INTO participant_sessions(id,conference_id,participant_id,user_id,connection_id,status,connected_at,last_seen_at,disconnected_at) VALUES(?,?,?,?,?,'disconnected',?,?,?)`, uuid.NewString(), cid, pid, f.member.ID, uuid.NewString(), lease.Origin.Add(time.Duration(span[0])*time.Second), lease.Origin.Add(time.Duration(span[1])*time.Second), lease.Origin.Add(time.Duration(span[1])*time.Second)).Error)
	}
	must(repo.Observe(ctx, lease, pid, 1000, 2000))
	ar := pg.NewAnalyticsRepository(f.db)
	hand := realtime.Hand{ParticipantID: pid, RaisedAt: time.Now()}
	must(ar.RecordHand(ctx, cid, hand))
	must(ar.RecordHand(ctx, cid, hand))
	must(ar.Tick(ctx))
	must((&analytics.Service{Repo: ar, Enabled: true}).Handle(ctx, contentClaim(t, f, "analytics.aggregate")))
	snapshot, err := ar.Read(ctx, f.member.ID, cid)
	must(err)
	if !snapshot.Enabled || len(snapshot.Participants) != 1 || snapshot.Participants[0].ParticipationMS != 8000 || snapshot.Participants[0].SpeakingMS != 1000 || snapshot.Participants[0].HandRaises != 1 || !snapshot.TranscriptAvailable {
		t.Fatalf("invalid aggregates %+v", snapshot)
	}
	if _, err = ar.Read(ctx, f.outsider.ID, cid); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("analytics leaked", err)
	}
	_, err = repo.Set(ctx, f.owner.ID, cid, false, "ru")
	must(err)
	if ok, err := repo.Renew(ctx, lease); err != nil || ok {
		t.Fatal("stale lease renewed", err)
	}
	c.ID = uuid.NewString()
	if _, err = repo.SaveFinal(ctx, lease, c, 100); err == nil {
		t.Fatal("stale final accepted")
	}
}

// TestStageEightSemanticAuthorizationAndModels проверяет реальный pgvector, гибридный рейтинг и модельные границы.
// @args t — исполнитель теста; embedding использует явно тестовый адаптер.
func TestStageEightSemanticAuthorizationAndModels(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := pg.NewSearchRepository(f.db)
	available, err := repo.Available(ctx)
	must(err)
	if !available {
		t.Skip("pgvector not installed; FTS-only installation")
	}
	cfg, err := config.LoadStageEight(true)
	must(err)
	cfg.EmbeddingsEnabled = true
	cfg.EmbeddingModel = "stage8-test"
	cfg.EmbeddingVersion = "v1"
	provider, err := contentproviders.NewEmbeddingProvider("mock", "", "", time.Second)
	must(err)
	service := search.New(repo, provider, cfg)
	f.service.SetSearch(service)
	must(f.service.Handle(ctx, contentClaim(t, f, "content.transcribe")))
	must(f.db.Exec(`UPDATE transcript_segments SET text='Бюджет запуск проекта'`).Error)
	job := contentClaim(t, f, "content.embed")
	must(service.Handle(ctx, job))
	must(service.Handle(ctx, job))
	var count int64
	must(f.db.Table("content_embeddings").Count(&count).Error)
	if count != 1 {
		t.Fatal("duplicate chunks", count)
	}
	for _, mode := range []string{"keyword", "semantic", "hybrid"} {
		q := "стоимость"
		if mode == "keyword" {
			q = "бюджет"
		}
		page, err := f.service.Search(ctx, f.member.ID, content.SearchQuery{Query: q, Mode: mode, Source: "transcript", Limit: 20})
		must(err)
		if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Rank <= 0 || page.Items[0].StartMS == nil || page.EffectiveMode != mode {
			t.Fatalf("%s failed %+v", mode, page)
		}
		if mode == "hybrid" && (page.Items[0].Rank < 0.34 || page.Items[0].Rank > 0.35) {
			t.Fatal("hybrid weights changed", page.Items[0].Rank)
		}
		denied, err := f.service.Search(ctx, f.outsider.ID, content.SearchQuery{Query: q, Mode: mode, Source: "all", Limit: 20})
		must(err)
		if denied.Total != 0 || len(denied.Items) != 0 {
			t.Fatal("vector IDOR", denied)
		}
	}
	owned, err := f.service.Search(ctx, f.member.ID, content.SearchQuery{Query: "бюджет", Mode: "hybrid", Source: "all", Membership: "owned", Limit: 20})
	must(err)
	if owned.Total != 0 {
		t.Fatal("ownership filter")
	}
	cfg.EmbeddingVersion = "v2"
	newModel := search.New(repo, provider, cfg)
	f.service.SetSearch(newModel)
	empty, err := f.service.Search(ctx, f.member.ID, content.SearchQuery{Query: "стоимость", Mode: "semantic", Source: "transcript", Limit: 20})
	must(err)
	if empty.Total != 0 {
		t.Fatal("incompatible version mixed")
	}
	must(newModel.Handle(ctx, job))
	must(f.db.Table("content_embeddings").Count(&count).Error)
	if count != 2 {
		t.Fatal("reindex version missing", count)
	}
	cfg.EmbeddingsEnabled = false
	f.service.SetSearch(search.New(repo, provider, cfg))
	fallback, err := f.service.Search(ctx, f.member.ID, content.SearchQuery{Query: "бюджет", Mode: "semantic", Source: "transcript", Limit: 20})
	must(err)
	if fallback.Total != 1 || fallback.EffectiveMode != "keyword" || fallback.FallbackReason == "" {
		t.Fatal("FTS fallback", fallback)
	}
}
