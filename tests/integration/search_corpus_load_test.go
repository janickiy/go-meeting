package integration_test

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	content "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/infrastructure/contentproviders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	search "github.com/janickiy/go-recorder/internal/usecase/search"
	"os"
	"sort"
	"testing"
	"time"
)

// TestStageEightSearchCorpus измеряет реальные SQL/pgvector запросы на 5000 синтетических RU/EN реплик.
// @args t — исполнитель ручного замера; fake embedding измеряет инфраструктуру, не качество semantic модели.
func TestStageEightSearchCorpus(t *testing.T) {
	if os.Getenv("RECORDER_SEARCH_LOAD") != "true" {
		t.Skip("set RECORDER_SEARCH_LOAD=true")
	}
	f := stageSevenContent(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := pg.NewSearchRepository(f.db)
	ok, err := repo.Available(ctx)
	must(err)
	if !ok {
		t.Fatal("pgvector required for corpus measurement")
	}
	must(f.service.Handle(ctx, contentClaim(t, f, "content.transcribe")))
	var transcript content.Transcript
	must(f.db.Table("transcripts").Where("recording_id=?", f.recording.UUID).Take(&transcript).Error)
	must(f.db.Exec(`DELETE FROM transcript_segments WHERE transcript_id=?`, transcript.ID).Error)
	topics := []string{"Обсудили бюджет проекта и стоимость инфраструктуры для запуска новой версии сервиса.", "The release schedule includes launch preparation, API review and database migration.", "Проверили требования безопасности, запись встречи и распределение задач между командами.", "Customer support needs an updated troubleshooting guide and meeting notes.", "Решили перенести демонстрацию экрана и тестирование аудио на следующий спринт."}
	segments := make([]content.Segment, 5000)
	for i := range segments {
		segments[i] = content.Segment{ID: uuid.NewString(), TranscriptID: transcript.ID, Ordinal: i, StartMS: int64(i * 1000), EndMS: int64((i + 1) * 1000), Text: fmt.Sprintf("%s Контекст обсуждения %d.", topics[i%len(topics)], i)}
	}
	must(f.db.Table("transcript_segments").CreateInBatches(&segments, 100).Error)
	must(f.db.Exec(`UPDATE record SET duration_sec=5000 WHERE uuid=?`, f.recording.UUID).Error)
	cfg, err := config.LoadStageEight(true)
	must(err)
	cfg.EmbeddingsEnabled = true
	cfg.EmbeddingModel = "corpus-test"
	provider, err := contentproviders.NewEmbeddingProvider("mock", "", "", time.Second)
	must(err)
	service := search.New(repo, provider, cfg)
	f.service.SetSearch(service)
	started := time.Now()
	must(service.Handle(ctx, contentClaim(t, f, "content.embed")))
	t.Logf("SEARCH_CORPUS chunks=5000 dimensions=%d index_time=%s", cfg.EmbeddingDimensions, time.Since(started).Round(time.Millisecond))
	must(f.db.Exec(`ANALYZE content_embeddings`).Error)
	must(f.db.Exec(`ANALYZE transcript_segments`).Error)
	for _, mode := range []string{"keyword", "semantic", "hybrid"} {
		times := make([]float64, 30)
		var total int64
		for i := range times {
			start := time.Now()
			page, err := f.service.Search(ctx, f.member.ID, content.SearchQuery{Query: "бюджет", Mode: mode, Source: "transcript", Limit: 20})
			must(err)
			if page.EffectiveMode != mode {
				t.Fatal("unexpected fallback", page.FallbackReason)
			}
			times[i] = float64(time.Since(start).Microseconds()) / 1000
			total = page.Total
			if len(page.Items) != 20 {
				t.Fatal("corpus search incomplete")
			}
		}
		sort.Float64s(times)
		t.Logf("SEARCH_CORPUS mode=%s n=%d p50_ms=%.3f p95_ms=%.3f max_ms=%.3f matches=%d", mode, len(times), times[15], times[28], times[29], total)
		denied, err := f.service.Search(ctx, f.outsider.ID, content.SearchQuery{Query: "бюджет", Mode: mode, Source: "transcript", Limit: 20})
		must(err)
		if denied.Total != 0 {
			t.Fatal("corpus vector leak")
		}
	}
}
