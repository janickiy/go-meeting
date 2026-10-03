package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// TestRateLimitDisabledSkipsLimiter проверяет пропуск ограничителя при выключенном лимите.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRateLimitDisabledSkipsLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &fakeLimiter{}
	router := gin.New()
	router.Use(httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{
		Enabled: false,
		Rules: []httpmiddleware.Rule{
			{
				Method: "GET",
				Path:   "/api/v1/records",
				Scope:  "ip",
				Limit:  1,
				Window: time.Minute,
				Key:    httpmiddleware.ClientIPKey,
			},
		},
	}))
	router.GET("/api/v1/records", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/records", nil))

	assertStatus(t, response.Code, http.StatusOK)
	if limiter.calls != 0 {
		t.Fatalf("limiter calls = %d, want 0", limiter.calls)
	}
}

// TestRateLimitExceededReturnsFailedJSON проверяет JSON-ошибку при превышении лимита.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRateLimitExceededReturnsFailedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetAt := time.Unix(1_765_000_000, 0).UTC()
	limiter := &fakeLimiter{
		result: ratelimit.Result{
			Allowed:    false,
			Limit:      12,
			Remaining:  0,
			RetryAfter: 30 * time.Second,
			ResetAt:    resetAt,
		},
	}
	router := gin.New()
	router.Use(httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{
		Enabled: true,
		Rules: []httpmiddleware.Rule{
			{
				Method: "POST",
				Path:   "/api/v1/records/start",
				Scope:  "conference",
				Limit:  12,
				Window: time.Minute,
				Key:    httpmiddleware.JSONFieldKey("conferenceId"),
			},
		},
	}))
	router.POST("/api/v1/records/start", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			t.Fatal("handler must not be called after rate limit exceeded")
		})

	response := performJSON(router, http.MethodPost, "/api/v1/records/start", `{
		"conferenceId":"11111111-1111-4111-8111-111111111111"
	}`)

	assertStatus(t, response.Code, http.StatusTooManyRequests)
	assertJSONField(t, response.Body.String(), "status", "failed")
	assertJSONField(t, response.Body.String(), "message", "rate limit exceeded")
	if response.Header().Get("Retry-After") != "30" {
		t.Fatalf("Retry-After = %q, want 30", response.Header().Get("Retry-After"))
	}
	if response.Header().Get("X-RateLimit-Limit") != "12" {
		t.Fatalf("X-RateLimit-Limit = %q, want 12", response.Header().Get("X-RateLimit-Limit"))
	}
	if response.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 0", response.Header().Get("X-RateLimit-Remaining"))
	}
	if response.Header().Get("X-RateLimit-Reset") != "1765000000" {
		t.Fatalf("X-RateLimit-Reset = %q, want 1765000000", response.Header().Get("X-RateLimit-Reset"))
	}
	if len(limiter.keys) != 1 || !strings.Contains(limiter.keys[0], "11111111-1111-4111-8111-111111111111") {
		t.Fatalf("rate limit key = %v, want conferenceId inside", limiter.keys)
	}
}

// TestRateLimitJSONFieldRestoresBodyForHandler проверяет восстановление тела JSON для обработчика.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRateLimitJSONFieldRestoresBodyForHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &fakeLimiter{
		result: ratelimit.Result{
			Allowed:   true,
			Limit:     12,
			Remaining: 11,
			ResetAt:   time.Unix(1_765_000_060, 0).UTC(),
		},
	}
	router := gin.New()
	router.Use(httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{
		Enabled: true,
		Rules: []httpmiddleware.Rule{
			{
				Method: "POST",
				Path:   "/api/v1/records/start",
				Scope:  "conference",
				Limit:  12,
				Window: time.Minute,
				Key:    httpmiddleware.JSONFieldKey("conferenceId"),
			},
		},
	}))
	router.POST("/api/v1/records/start", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			var request struct {
				ConferenceID string `json:"conferenceId"`
			}
			if err := c.ShouldBindJSON(&request); err != nil {
				t.Fatalf("ShouldBindJSON() error = %v", err)
			}
			c.JSON(http.StatusAccepted, request)
		})

	response := performJSON(router, http.MethodPost, "/api/v1/records/start", `{
		"conferenceId":"11111111-1111-4111-8111-111111111111"
	}`)

	assertStatus(t, response.Code, http.StatusAccepted)
	assertJSONField(t, response.Body.String(), "conferenceId", "11111111-1111-4111-8111-111111111111")
	if limiter.calls != 1 {
		t.Fatalf("limiter calls = %d, want 1", limiter.calls)
	}
	if limiter.limits[0] != 12 {
		t.Fatalf("limit = %d, want 12", limiter.limits[0])
	}
}

// TestRateLimitPathParamKey проверяет сценарий «Rate лимит путь Param ключ», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRateLimitPathParamKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := &fakeLimiter{
		result: ratelimit.Result{Allowed: true, Limit: 40, Remaining: 39},
	}
	router := gin.New()
	router.Use(httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{
		Enabled: true,
		Rules: []httpmiddleware.Rule{
			{
				Method: "POST",
				Path:   "/api/v1/records/:id/webrtc/offer",
				Scope:  "record",
				Limit:  40,
				Window: time.Minute,
				Key:    httpmiddleware.PathParamKey("id"),
			},
		},
	}))
	router.POST("/api/v1/records/:id/webrtc/offer", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})

	response := performJSON(router, http.MethodPost, "/api/v1/records/22222222-2222-4222-8222-222222222222/webrtc/offer", `{"type":"offer","sdp":"sdp"}`)

	assertStatus(t, response.Code, http.StatusOK)
	if len(limiter.keys) != 1 || !strings.Contains(limiter.keys[0], "22222222-2222-4222-8222-222222222222") {
		t.Fatalf("rate limit key = %v, want record id inside", limiter.keys)
	}
}

// fakeLimiter хранит изолированное состояние тестового ограничителя.
// @params:
//   - calls: значение calls типа int, используемое согласно назначению этой операции.
//   - result: результат проверки или обработки, передаваемый следующему шагу.
//   - err: сохранённая причина ошибочного завершения.
//   - keys: набор значений keys для последовательной или пакетной обработки.
//   - limits: настройки ограничений размера, частоты и количества ресурсов.
type fakeLimiter struct {
	calls  int
	result ratelimit.Result
	err    error
	keys   []string
	limits []int
}

// Allow проверяет ограничение частоты и возвращает решение, остаток и время сброса.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - limit (int): предел количества обрабатываемых элементов.
//   - _ (time.Duration): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (ratelimit.Result): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (l *fakeLimiter) Allow(_ context.Context, key string, limit int, _ time.Duration) (ratelimit.Result, error) {
	l.calls++
	l.keys = append(l.keys, key)
	l.limits = append(l.limits, limit)

	return l.result, l.err
}
