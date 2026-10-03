package liveproviders

import (
	"context"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/janickiy/go-recorder/internal/domain/captions"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestWebsocketGatewayProtocol проверяет handshake, бинарный PCM, конец потока и получение финала до Close.
// @args t — исполнитель; gateway локальный, без передачи звука внешнему сервису.
func TestWebsocketGatewayProtocol(t *testing.T) {
	seen := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var cfg captions.SessionConfig
		if conn.ReadJSON(&cfg) != nil {
			return
		}
		valid := r.Header.Get("Authorization") == "Bearer test-only" && r.Header.Get("Idempotency-Key") == cfg.SessionID && cfg.SampleRate == 16000
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		valid = valid && kind == websocket.BinaryMessage && len(data) == 640
		_ = conn.WriteJSON(captions.Event{UtteranceID: "a", Text: "draft", Sequence: 1, Revision: 1, Language: "en", EndMS: 20})
		_, data, err = conn.ReadMessage()
		var end map[string]string
		valid = valid && err == nil && json.Unmarshal(data, &end) == nil && end["type"] == "end"
		_ = conn.WriteJSON(captions.Event{UtteranceID: "a", Text: "final", Sequence: 2, Revision: 2, Language: "en", EndMS: 20, Final: true})
		seen <- valid
	}))
	defer server.Close()
	p := Provider{Mode: "websocket", Endpoint: "ws" + strings.TrimPrefix(server.URL, "http"), Token: "test-only", Timeout: time.Second}
	s, err := p.StartSession(context.Background(), captions.SessionConfig{SessionID: "session", TrackInstanceID: "track", Language: "en", SampleRate: 16000, Channels: 1, Format: "pcm_s16le"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAudio(context.Background(), make([]byte, 640)); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if !<-seen {
		t.Fatal("protocol changed")
	}
	var events []captions.Event
	for e := range s.Events() {
		events = append(events, e)
	}
	if len(events) != 2 || !events[1].Final {
		t.Fatal("final lost", events)
	}
	if err = s.Close(); err != nil {
		t.Fatal("repeat Close", err)
	}
}

// TestGatewayOutageCancellation проверяет ограниченное по времени отключение молчащего или сломавшегося внешнего сервиса.
// @args t — исполнитель проверки очистки ресурсов.
func TestGatewayOutageCancellation(t *testing.T) {
	p := Provider{Mode: "mock", Timeout: time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := p.StartSession(ctx, captions.SessionConfig{SampleRate: 16000, Channels: 1, Format: "pcm_s16le", Language: "ru"})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if s.WriteAudio(ctx, make([]byte, 640)) == nil {
		t.Fatal("cancel ignored")
	}
	if s.Close() != nil {
		t.Fatal("close failed")
	}
	if _, ok := <-s.Events(); ok {
		t.Fatal("events not closed")
	}
	if _, err = p.StartSession(context.Background(), captions.SessionConfig{SampleRate: 48000, Channels: 2}); err == nil {
		t.Fatal("invalid PCM accepted")
	}
}
