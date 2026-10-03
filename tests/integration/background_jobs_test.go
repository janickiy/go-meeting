package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

// TestStageSevenJobDedupLeaseAndMigrationRepeat проверяет реальные SQL-дедупликацию и контроль актуальности аренды.
// Используется отдельная случайная тестовая БД, никогда не база пользователя.
// @args t — контекст изолированной интеграционной проверки.
func TestStageSevenJobDedupLeaseAndMigrationRepeat(t *testing.T) {
	db := stageOneDatabase(t)
	ctx := context.Background()
	if err := pg.RunMigrations(db, filepath.Join("..", "..", "database", "migrations")); err != nil {
		t.Fatal("repeated startup migrations:", err)
	}
	id, entity, cid := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for range 3 {
		if err := db.Exec(`INSERT INTO background_jobs(id,kind,entity_id,conference_id,payload,dedup_key,max_attempts)
		VALUES (?,'content.transcribe',?,?, '{"generation":1}'::jsonb,?,2) ON CONFLICT(dedup_key) DO NOTHING`, id, entity, cid, "test:"+id).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := pg.NewJobRepository(db)
	claimed := []jobs.Job{}
	var mu sync.Mutex
	runConcurrent(8, func(int) {
		job, found, err := repo.Claim(ctx, "content.transcribe", time.Minute)
		if err != nil {
			t.Error(err)
		}
		if found {
			mu.Lock()
			claimed = append(claimed, job)
			mu.Unlock()
		}
	})
	if len(claimed) != 1 || claimed[0].ID != id || claimed[0].Attempts != 1 {
		t.Fatalf("claim not atomic: %+v", claimed)
	}
	var payload map[string]int
	if err := json.Unmarshal(claimed[0].Payload, &payload); err != nil || payload["generation"] != 1 {
		t.Fatal("job JSON payload not preserved", err)
	}
	if err := db.Exec(`UPDATE background_jobs SET lease_until=now()-interval '1 second' WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	second, found, err := repo.Claim(ctx, "content.transcribe", time.Minute)
	if err != nil || !found || second.Attempts != 2 || second.LeaseToken == claimed[0].LeaseToken {
		t.Fatal("lease recovery failed", err)
	}
	if err := repo.Finish(ctx, claimed[0], "done", "", nil); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatal("stale worker completed new lease", err)
	}
	if err := repo.Finish(ctx, second, "done", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, found, err := repo.Claim(ctx, "content.transcribe", time.Minute); err != nil || found {
		t.Fatal("completed job replayed", err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || len(counts) != 0 {
		t.Fatal("completed work appears in queue", counts, err)
	}
}
