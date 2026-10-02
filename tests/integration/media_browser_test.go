package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestStageThreeBrowserMedia проверяет сценарий «этап три браузер медиа», фиксируя ошибки поведения как регрессию.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageThreeBrowserMedia(t *testing.T) {
	if os.Getenv("RECORDER_STAGE3_BROWSER_E2E") != "true" {
		t.Skip("set RECORDER_STAGE3_BROWSER_E2E=true with local PostgreSQL/Redis test settings")
	}
	f := stageTwo(t)
	engine, _ := startStageThreeMedia(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "npm", "run", "test:e2e", "--", "e2e/media.spec.ts", "--workers=1")
	command.Dir = "../../frontend"
	command.Env = append(os.Environ(),
		"API_PROXY_TARGET="+f.servers[0].URL,
		"MEET_LIVE_TEST_URL=http://127.0.0.1:5175",
		"MEET_MEDIA_CONFERENCE="+f.conference.ID,
		"MEET_MEDIA_ALICE_EMAIL="+f.owner.Email,
		"MEET_MEDIA_BOB_EMAIL="+f.member.Email,
		"MEET_MEDIA_PASSWORD="+stageOneTestPassword,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated browser SFU media: %v, worker resources: %+v\n%s", err, engine.Snapshot(), output)
	}
	t.Logf("%s", output)
	deadline := time.Now().Add(5 * time.Second)
	for engine.Snapshot().Peers != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if snapshot := engine.Snapshot(); snapshot.Peers != 0 || snapshot.Rooms != 0 || snapshot.Tracks != 0 || snapshot.Subscriptions != 0 {
		t.Fatalf("browser teardown left media resources: %+v", snapshot)
	}
}
