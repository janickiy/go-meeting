package integration_test

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Isolated Chromium uses fake camera/microphone and a synthetic canvas screen;
// no physical devices, user browser profile or application database are used.
func TestStageFourBrowserControls(t *testing.T) {
	if os.Getenv("RECORDER_STAGE4_BROWSER_E2E") != "true" {
		t.Skip("set RECORDER_STAGE4_BROWSER_E2E=true with PostgreSQL/Redis test settings")
	}
	f := stageTwo(t)
	// Use production signaling limits: a browser must not burst one ICE
	// transport per preallocated receiver and disconnect before media starts.
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
