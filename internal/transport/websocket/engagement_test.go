package websocket

import (
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"testing"
	"time"
)

// TestCollaborationFloodDoesNotOccupyCriticalQueue проверяет сценарий «Collaboration всплеск выполняет не Occupy критичный очередь», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCollaborationFloodDoesNotOccupyCriticalQueue(t *testing.T) {
	c := newClient(nil, &Handler{cfg: config.RealtimeConfig{QueueSize: 4}}, domain.Session{}, time.Now().Add(time.Hour))
	for i := 0; i < 10000; i++ {
		if !c.Offer(domain.Event("reaction.created", "", nil)) || !c.Offer(domain.Event("chat.message.created", "", nil)) {
			t.Fatal("ephemeral overflow closed socket")
		}
	}
	if len(c.low) != 8 || len(c.out) != 0 {
		t.Fatal("unbounded/critical queue occupied")
	}
	for _, kind := range []string{"media.answer", "conference.state", "recording.updated", "media.policy"} {
		if !c.Offer(domain.Event(kind, "", nil)) {
			t.Fatalf("critical %s rejected", kind)
		}
	}
	select {
	case <-c.done:
		t.Fatal("media disconnected by reactions")
	default:
	}
}
