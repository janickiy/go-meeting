package operations

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestReadinessCacheDrainAndSecret проверяет отсутствие всплеска нагрузки от проверок готовности,
// отказ готовности, завершение работы и защиту метрик токеном Bearer. t фиксирует регрессии.
func TestReadinessCacheDrainAndSecret(t *testing.T) {
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.OperationsConfig{ProbeTimeout: 50 * time.Millisecond, ProbeInterval: time.Hour, HTTPBodyBytes: 65536, MetricsSecret: strings.Repeat("m", 32)}
	r := New("test", "test", cfg, map[string]Check{"database": func(ctx context.Context) error { calls.Add(1); return ctx.Err() }})
	done := r.Run(ctx)
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r.Readiness(w, httptest.NewRequest("GET", "/health/ready", nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("health requests trigger probes")
	}
	for _, secret := range []string{"", "wrong", cfg.MetricsSecret} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/metrics", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		r.Metrics(w, req)
		expected := 404
		if secret == cfg.MetricsSecret {
			expected = 200
		}
		if w.Code != expected {
			t.Fatal(w.Code)
		}
		if strings.Contains(w.Body.String(), cfg.MetricsSecret) {
			t.Fatal("metrics leak credentials")
		}
	}
	r.Checks["database"] = func(ctx context.Context) error { return errors.New("private-error") }
	r.probe(ctx)
	w := httptest.NewRecorder()
	r.Readiness(w, httptest.NewRequest("GET", "/health/ready", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "private-error") {
		t.Fatal(w.Body.String())
	}
	states := r.DependencyStatuses()
	if states["database"] {
		t.Fatal("failed dependency reported ready")
	}
	states["database"] = true
	if r.DependencyStatuses()["database"] {
		t.Fatal("caller modified cached dependency state")
	}
	r.Drain()
	r.Checks["database"] = func(context.Context) error { return nil }
	r.probe(ctx)
	if r.Ready() {
		t.Fatal("drain reverted")
	}
	w = httptest.NewRecorder()
	r.Live(w, httptest.NewRequest("GET", "/health/live", nil))
	if w.Code != 200 {
		t.Fatal("dependency failure broke liveness")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("probe loop leaked")
	}
}

// TestMiddlewareCardinalityAndBody проверяет корреляцию UUID, ограниченный набор меток,
// размер JSON, учёт отказов и отклонение новых запросов во время завершения работы.
func TestMiddlewareCardinalityAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := New("test", "test", config.OperationsConfig{HTTPBodyBytes: 65536}, nil)
	router := gin.New()
	router.Use(r.Middleware())
	r.RegisterGin(router)
	router.POST("/items/:id", func(c *gin.Context) {
		if ID(c.Request.Context()) == "" {
			t.Error("missing correlation")
		}
		c.Status(204)
	})
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/items/"+uuid.NewString(), nil))
		if w.Code != 204 {
			t.Fatal(w.Code)
		}
		if _, err := uuid.Parse(w.Header().Get("X-Request-ID")); err != nil {
			t.Fatal(err)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/items/any", strings.NewReader(strings.Repeat("x", 65537))))
	if w.Code != 413 {
		t.Fatal(w.Code)
	}
	for i := 0; i < 100; i++ {
		State(fmt.Sprintf("technical_%d", i), 1)
		WSMessage(uuid.NewString())
	}
	if len(r.customNames) != 64 {
		t.Fatal("unbounded state cardinality")
	}
	metrics, err := r.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metrics {
		if m.GetName() == "recorder_http_requests_total" && len(m.Metric) != 2 {
			t.Fatal("URL IDs in labels")
		}
	}
	r.Drain()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/items/any", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

// TestMiddlewareAttachmentBodies проверяет реальные чтения тела через Gin:
// только PUT content обоих чатов проходит JSON-лимит, в том числе без Content-Length.
// JSON, другие методы и похожий шаблон пути остаются ограничены тем же пределом.
func TestMiddlewareAttachmentBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const limit int64 = 1 << 20
	payload := `{"text":"` + strings.Repeat("x", int(limit)+1024) + `"}`
	r := New("test", "test", config.OperationsConfig{HTTPBodyBytes: limit}, nil)
	for _, scope := range []string{"conferences", "conversations"} {
		for _, method := range []string{http.MethodPut, http.MethodPost} {
			for _, knownLength := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/%s/known-length-%v", scope, method, knownLength), func(t *testing.T) {
					testMiddlewareBody(t, r, method,
						"/api/v1/"+scope+"/:id/attachments/:attachmentId/content",
						"/api/v1/"+scope+"/conversation/attachments/attachment/content",
						payload, knownLength, method == http.MethodPut, limit)
				})
			}
		}
	}
	for _, endpoint := range []struct {
		name, method, route, path string
	}{
		{"json", http.MethodPost, "/api/v1/conversations/direct", "/api/v1/conversations/direct"},
		{"similar-content-route", http.MethodPut, "/api/v1/other/:id/attachments/:attachmentId/content", "/api/v1/other/conversation/attachments/attachment/content"},
	} {
		for _, knownLength := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/known-length-%v", endpoint.name, knownLength), func(t *testing.T) {
				testMiddlewareBody(t, r, endpoint.method, endpoint.route, endpoint.path, payload, knownLength, false, limit)
			})
		}
	}
}

func testMiddlewareBody(t *testing.T, r *Runtime, method, route, path, payload string, knownLength, allowed bool, limit int64) {
	t.Helper()
	router := gin.New()
	router.Use(r.Middleware())
	called := false
	var readBytes int64
	router.Handle(method, route, func(c *gin.Context) {
		called = true
		var err error
		readBytes, err = io.Copy(io.Discard, c.Request.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if !errors.As(err, &tooLarge) {
				t.Errorf("unexpected body error: %v", err)
			}
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(method, path, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if !knownLength {
		req.ContentLength = -1
		req.TransferEncoding = []string{"chunked"}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if allowed {
		if w.Code != http.StatusNoContent || !called || readBytes != int64(len(payload)) {
			t.Fatalf("upload body truncated or rejected: status=%d called=%v bytes=%d want=%d", w.Code, called, readBytes, len(payload))
		}
		return
	}
	if w.Code != http.StatusRequestEntityTooLarge || readBytes > limit || called == knownLength {
		t.Fatalf("body limit bypassed: status=%d called=%v bytes=%d limit=%d knownLength=%v", w.Code, called, readBytes, limit, knownLength)
	}
}

// TestProfilingDurationBound проверяет отказ до запуска долгой диагностики.
// t перебирает неверные значения, NaN, Inf и допустимые параметры без запуска измерений CPU.
func TestProfilingDurationBound(t *testing.T) {
	for _, value := range []string{"61", "0", "-1", "NaN", "Inf", "text", "0.5", "60", ""} {
		called := false
		h := boundedProfile(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { called = true; w.WriteHeader(204) }))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/debug/pprof/profile?seconds="+value, nil))
		allowed := value == "0.5" || value == "60" || value == ""
		if called != allowed || (!allowed && w.Code != 400) {
			t.Fatalf("duration %q: called=%v code=%d", value, called, w.Code)
		}
	}
}
