package telemetry

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// TestReceiveRejectsSensitiveData проверяет, что приватные данные не попадают в журнал даже при ошибке разбора.
// @args t — контекст теста проверки границы доверия телеметрии.
func TestReceiveRejectsSensitiveData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"version":"v1.0.0","route":"/history/:id","code":"uncaught_error","browser":"firefox","stack":[{"file":"index-AbCdEfGh.js","line":12,"column":34}]}`
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, http.StatusAccepted},
		{"unknown field", strings.Replace(valid, `"version":`, `"token":"private-sentinel","version":`, 1), http.StatusBadRequest},
		{"resource route", strings.Replace(valid, "/history/:id", "/history/private-sentinel", 1), http.StatusBadRequest},
		{"query", strings.Replace(valid, "/history/:id", "/history?token=private-sentinel", 1), http.StatusBadRequest},
		{"raw message", strings.Replace(valid, "uncaught_error", "private-sentinel", 1), http.StatusBadRequest},
		{"raw browser", strings.Replace(valid, "firefox", "private-sentinel", 1), http.StatusBadRequest},
		{"signed URL", strings.Replace(valid, "index-AbCdEfGh.js", "https://private-sentinel/file.js", 1), http.StatusBadRequest},
		{"stack text", strings.Replace(valid, `"line":12`, `"message":"private-sentinel","line":12`, 1), http.StatusBadRequest},
		{"invalid position", strings.Replace(valid, `"line":12`, `"line":-1`, 1), http.StatusBadRequest},
		{"extra JSON", valid + `{"token":"private-sentinel"}`, http.StatusBadRequest},
		{"oversize", `{"version":"` + strings.Repeat("private-sentinel", 800) + `"}`, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			registry := prometheus.NewRegistry()
			handler := NewHandler(true, slog.New(slog.NewJSONHandler(&logs, nil)), registry)
			router := gin.New()
			router.POST("/client-errors", handler.Receive)
			request := httptest.NewRequest(http.MethodPost, "/client-errors", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if strings.Contains(logs.String(), "private-sentinel") || (test.status != http.StatusAccepted && logs.Len() != 0) {
				t.Fatal("untrusted report leaked into logs")
			}
			if test.status == http.StatusAccepted {
				var logged map[string]any
				if json.Unmarshal(logs.Bytes(), &logged) != nil || logged["route"] != "/history/:id" {
					t.Fatal("safe report was not recorded")
				}
				metrics, err := registry.Gather()
				if err != nil || len(metrics) != 1 || metrics[0].GetName() != "recorder_client_errors_total" {
					t.Fatal("client counter is missing")
				}
			}
		})
	}
}

// TestReceiveDisabledAndContentType проверяет отключённый приём и запрет простых межсайтовых POST.
// @args t — контекст теста выключенной функции и неподдерживаемого формата.
func TestReceiveDisabledAndContentType(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		handler := NewHandler(enabled, slog.New(slog.NewTextHandler(io.Discard, nil)), prometheus.NewRegistry())
		router := gin.New()
		router.POST("/client-errors", handler.Receive)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/client-errors", strings.NewReader(`{}`)))
		want := http.StatusNotFound
		if enabled {
			want = http.StatusUnsupportedMediaType
		}
		if response.Code != want {
			t.Fatalf("status = %d, want %d", response.Code, want)
		}
	}
}
