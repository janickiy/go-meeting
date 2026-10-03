package integration_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

// TestStageNineAdminRepository проверяет повторные миграции на изолированной базе PostgreSQL.
func TestStageNineAdminRepository(t *testing.T) {
	db := stageOneDatabase(t)
	ctx := context.Background()
	userID := uuid.NewString()
	if err := db.Exec(`INSERT INTO users(id,email,password_hash) VALUES(?,?,?)`, userID, "admin-stage-nine@example.test", "test-hash").Error; err != nil {
		t.Fatal(err)
	}
	repository := postgresinfra.NewPlatformRepository(db)
	if allowed, err := repository.IsAdmin(ctx, userID); err != nil || allowed {
		t.Fatalf("new user gained admin access: %t %v", allowed, err)
	}
	if err := db.Exec(`UPDATE users SET is_admin = TRUE WHERE id = ?`, userID).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := repository.IsAdmin(ctx, userID); err != nil || !allowed {
		t.Fatalf("explicit promotion failed: %t %v", allowed, err)
	}
	jobID := uuid.NewString()
	if err := db.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,dedup_key,payload,state,error_code) VALUES('content.transcribe',?,?,?,?::jsonb,'failed','provider_timeout')`, jobID, uuid.NewString(), "stage9:"+jobID, `{"privateToken":"must-not-appear"}`).Error; err != nil {
		t.Fatal(err)
	}
	summary, err := repository.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FailedJobs24h != 1 || len(summary.RecentFailures) != 1 || summary.RecentFailures[0].Code != "provider_timeout" {
		t.Fatalf("unexpected aggregates: %+v", summary)
	}
	raw, err := json.Marshal(summary)
	if err != nil || strings.Contains(string(raw), "must-not-appear") || strings.Contains(string(raw), jobID) {
		t.Fatalf("summary leaked payload or entity ID: %s %v", raw, err)
	}
	if err := db.Exec(`UPDATE users SET is_admin = FALSE WHERE id = ?`, userID).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err := repository.IsAdmin(ctx, userID); err != nil || allowed {
		t.Fatalf("demotion was not effective: %t %v", allowed, err)
	}
}
