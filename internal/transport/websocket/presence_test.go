package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// TestMissingHeartbeatCannotBeKeptAliveByMessages проверяет настоящий сокет:
// ни чужой pong, ни текстовые сообщения не продлевают присутствие без верного
// подтверждения связи. Сокращённые интервалы сохраняют алгоритм пятисекундного
// рабочего тайм-аута, но позволяют проверить границу без долгого ожидания.
//
// @args
//   - t: контекст сетевого теста с изолированным HTTP-сервером.
func TestMissingHeartbeatCannotBeKeptAliveByMessages(t *testing.T) {
	for _, kind := range []string{"silent", "wrong pong", "application messages"} {
		t.Run(kind, func(t *testing.T) {
			result := make(chan string, 1)
			started := make(chan struct{})
			cfg := config.RealtimeConfig{PingInterval: 100 * time.Millisecond, PongTimeout: 100 * time.Millisecond, QueueSize: 64, MessageBytes: 65536, Burst: 1000, MessagesPerSecond: 1000}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&ws.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				client := newClient(conn, &Handler{cfg: cfg}, domain.Session{ConnectionID: uuid.NewString()}, time.Now().Add(time.Hour))
				close(started)
				client.read()
				result <- client.reason
			}))
			defer server.Close()
			conn, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			<-started
			begin := time.Now()
			pulse := time.NewTicker(10 * time.Millisecond)
			defer pulse.Stop()
			limit := time.NewTimer(time.Second)
			defer limit.Stop()
			for {
				select {
				case reason := <-result:
					elapsed := time.Since(begin)
					if reason != "presence_timeout" || elapsed < 150*time.Millisecond || elapsed > 750*time.Millisecond {
						t.Fatalf("wrong timeout: reason=%s elapsed=%s", reason, elapsed)
					}
					return
				case <-limit.C:
					t.Fatal("missing confirmed pong left session alive")
				case <-pulse.C:
					switch kind {
					case "wrong pong":
						_ = conn.WriteControl(ws.PongMessage, []byte("another-connection"), time.Now().Add(50*time.Millisecond))
					case "application messages":
						_ = conn.WriteMessage(ws.TextMessage, []byte(`{}`))
					}
				}
			}
		})
	}
}
