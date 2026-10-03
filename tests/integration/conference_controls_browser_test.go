package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestStageFourBrowserControls проверяет сценарий «этап четыре браузер управление», фиксируя ошибки поведения как регрессию.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourBrowserControls(t *testing.T) {
	if os.Getenv("RECORDER_STAGE4_BROWSER_E2E") != "true" {
		t.Skip("set RECORDER_STAGE4_BROWSER_E2E=true with PostgreSQL/Redis test settings")
	}
	f := stageTwo(t)
	// Используем рабочие ограничения сигнализации: браузер не должен создавать всплеск ICE-транспортов
	// для каждого заранее выделенного приёмника и отключаться до начала передачи медиа.
	f.config.MessagesPerSecond = 20
	f.config.Burst = 40
	engine, _ := startMediaWithLimits(t, f, 4, 2, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "npm", "run", "test:e2e", "--", "e2e/media.spec.ts", "--workers=1")
	command.Dir = "../../frontend"
	command.Env = append(os.Environ(), "API_PROXY_TARGET="+f.servers[0].URL,
		"MEET_LIVE_TEST_URL=http://127.0.0.1:5175", "MEET_STAGE4=true",
		"MEET_MEDIA_CONFERENCE="+f.conference.ID, "MEET_MEDIA_ALICE_EMAIL="+f.owner.Email,
		"MEET_MEDIA_BOB_EMAIL="+f.member.Email, "MEET_MEDIA_PASSWORD="+stageOneTestPassword)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser media controls: %v, worker resources: %+v\n%s", err, engine.Snapshot(), output)
	}
	t.Logf("%s", output)
	deadline := time.Now().Add(5 * time.Second)
	for engine.Snapshot().Peers != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := engine.Snapshot(); s.Peers != 0 || s.Rooms != 0 || s.Tracks != 0 || s.Subscriptions != 0 {
		t.Fatalf("browser teardown leaked resources: %+v", s)
	}
}
