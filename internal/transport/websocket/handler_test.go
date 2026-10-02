package websocket

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
)

// TestBoundedQueueDisconnectsSlowClient проверяет сценарий «ограниченный очередь Disconnects Slow клиент», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestBoundedQueueDisconnectsSlowClient(t *testing.T) {
	c := newClient(nil, &Handler{cfg: config.RealtimeConfig{QueueSize: 2}}, domain.Session{}, time.Now().Add(time.Hour))
	if !c.Offer(domain.Event("one", "", nil)) || !c.Offer(domain.Event("two", "", nil)) {
		t.Fatal("queue not filled")
	}
	if c.Offer(domain.Event("three", "", nil)) {
		t.Fatal("overflow accepted")
	}
	select {
	case <-c.done:
	default:
		t.Fatal("slow client not stopped")
	}
	if c.reason != "slow_client" || len(c.out) != 2 {
		t.Fatal("unbounded queue/wrong close reason")
	}
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			c.Stop("duplicate")
			if c.Offer(domain.Event("later", "", nil)) {
				t.Error("closed socket accepted event")
			}
		}()
	}
	wg.Wait()
}

// TestOriginAndSafeAuthentication проверяет сценарий «Origin и безопасный Authentication», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestOriginAndSafeAuthentication(t *testing.T) {
	tokens, _ := security.NewTokenService(strings.Repeat("s", 32))
	h := NewHandler(nil, tokens, nil, nil, config.RealtimeConfig{})
	for _, origin := range []string{"https://evil.example", "null", "https://localhost:5173/path", "https://user@localhost:5173", "https://localhost:5173?ticket=bad"} {
		r := httptest.NewRequest("GET", "http://localhost:5173/", nil)
		r.Header.Set("Origin", origin)
		if h.origin(r) {
			t.Errorf("origin accepted: %s", origin)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost:5173/", nil)
	r.Header.Set("Origin", "https://localhost:5173")
	if !h.origin(r) {
		t.Fatal("same host HTTPS proxy rejected")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h.RegisterRoutes(router)
	for _, path := range []string{"/api/v1/webrtc/config", "/api/v1/conferences/3c13da06-9cd3-45c8-85ea-2c6ddcb18611/ws?token=long-jwt", "/api/v1/conferences/3c13da06-9cd3-45c8-85ea-2c6ddcb18611/ws"} {
		r := httptest.NewRequest("GET", path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		if response.Code != 401 {
			t.Errorf("missing auth not rejected: %d", response.Code)
		}
	}
}

// TestStrictJSONAndBucket проверяет сценарий «строгий JSON и Bucket», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStrictJSONAndBucket(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{"unknown":true}`, `null true`} {
		var e domain.Envelope
		if strictJSON([]byte(raw), &e) == nil {
			t.Error("invalid JSON accepted")
		}
	}
	var e domain.Envelope
	if strictJSON(bytes.TrimSpace([]byte(`{"version":1}`)), &e) != nil {
		t.Fatal("valid JSON rejected")
	}
	b := bucket{tokens: 2, burst: 2, rate: 1, updated: time.Now()}
	first, second, third := b.allow(), b.allow(), b.allow()
	if !first || !second || third {
		t.Fatal("burst limit not enforced")
	}
}
