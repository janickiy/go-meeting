package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	app "github.com/janickiy/go-recorder/internal/app/integrations"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	u "github.com/janickiy/go-recorder/internal/usecase/integrations"
)

// integrationLimiter фиксирует безопасные ключи областей Redis и управляемое решение по запросу.
type integrationLimiter struct {
	allowed bool
	keys    []string
}

// Allow моделирует общую распределённую квоту без внешнего Redis в модульном тесте маршрутов.
// @args ctx — срок запроса; key — пользовательская область; limit/window — фиксированная policy.
// @return: controlled результат и nil.
func (l *integrationLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error) {
	l.keys = append(l.keys, key)
	return ratelimit.Result{Allowed: l.allowed, Limit: limit, RetryAfter: time.Minute, ResetAt: time.Now().Add(time.Minute)}, nil
}

// TestIntegrationRoutesRateLimitAndStrictJSON проверяет квоту до обработчика и запрет токенов провайдера, неизвестных полей и данных после JSON.
// @args t — контекст unit теста HTTP.
func TestIntegrationRoutesRateLimitAndStrictJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := security.NewTokenService(strings.Repeat("integration-test-secret-", 3))
	token, _ := tokens.Issue("d781f8eb-dff7-4a51-b018-7c0b1eeb3b62")
	service, err := u.NewService(nil, d.Providers{Capabilities: d.Capabilities{Email: "noop", Push: "noop", Calendar: "noop"}}, nil, u.Options{})
	if err != nil {
		t.Fatal(err)
	}
	limiter := &integrationLimiter{}
	router := gin.New()
	RegisterIntegrationRoutes(router, app.NewHandler(service), middleware.Authenticate(tokens), limiter)
	request := func(method, path, body string, bearer bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, APIV1Prefix+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if bearer {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Error("private response can be cached")
		}
		return response
	}
	if got := request("GET", "/integrations/calendars/generic/connect", "", true); got.Code != 429 || len(limiter.keys) != 1 || !strings.Contains(limiter.keys[0], "calendar_oauth_connect") {
		t.Fatal("OAuth not rate limited", got.Code, limiter.keys)
	}
	if got := request("POST", "/notifications/devices", `{"platform":"web","token":"sensitive-token"}`, true); got.Code != 429 || !strings.Contains(limiter.keys[len(limiter.keys)-1], "push_device_register") {
		t.Fatal("device registration quota missing")
	}
	limiter.allowed = true
	for _, test := range []struct{ method, path, body string }{{"POST", "/integrations/calendars/generic/callback", `{"code":"code","state":"state","accessToken":"forbidden"}`}, {"POST", "/notifications/devices", `{"platform":"web","token":"token","endpoint":"https://attacker"}`}, {"PUT", "/notifications/preferences", `{"email":true}{}`}, {"POST", "/integrations/calendars/mock", `{"accessToken":"manual-token"}`}} {
		if got := request(test.method, test.path, test.body, true); got.Code != http.StatusBadRequest {
			t.Fatal("non-strict JSON accepted", test.path, got.Code)
		}
	}
	if got := request("GET", "/integrations/capabilities", "", false); got.Code != 401 {
		t.Fatal("unauthenticated integration read")
	}
}
