package integration_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestRetiredRecordingAPILocal проверяет закрытие прежнего браузерного recorder
// API на явно выбранном локальном стенде. В отличие от прежнего smoke-теста,
// проверка не создаёт пользователей, записей, объектов хранилища или media tracks.
//
// @args
//   - t: контекст проверки, пропускаемой без RECORDER_TEST_URL.
func TestRetiredRecordingAPILocal(t *testing.T) {
	base := os.Getenv("RECORDER_TEST_URL")
	if base == "" {
		t.Skip("set RECORDER_TEST_URL to verify retired endpoints on a local API")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") || u.User != nil {
		t.Fatal("integration test only supports a local HTTP API without URL credentials")
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		// Не следуем перенаправлению с локального стенда на внешний адрес.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	paths := []string{
		"/api/v1/records", "/api/v1/records/start", "/api/v1/records/end",
		"/api/v1/records/count-by-conference", "/api/v1/records/unknown/webrtc/offer",
		"/api/records", "/api/records/unknown", "/debug/records/completed", "/debug/webrtc-smoke",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, strings.TrimRight(base, "/")+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusGone || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("retired endpoint returned status=%d cache-control=%q", response.StatusCode, response.Header.Get("Cache-Control"))
			}
			var payload map[string]string
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil || len(payload) != 2 || payload["status"] != "failed" || payload["message"] == "" {
				t.Fatal("retired endpoint returned unexpected data")
			}
		})
	}
}
