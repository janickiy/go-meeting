package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	domain "github.com/janickiy/go-recorder/internal/domain/media"
)

// TestHTTPMediaClientIdentityCorrelationAndSafeErrors проверяет связь ответов с доверенной идентичностью и безопасные ошибки.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestHTTPMediaClientIdentityCorrelationAndSafeErrors(t *testing.T) {
	id := uuid.NewString()
	secret := strings.Repeat("i", 32)
	server := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("X-Request-ID") != id || r.URL.Path != "/internal/media/offer" {
				t.Error("internal auth/correlation missing")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"code":"invalid_media_signal","detail":"sensitive SDP that must not escape"}`))
		}))
	defer server.Close()
	client := NewHTTPClient(secret, time.Second)
	defer client.Close()
	_, err := client.Call(context.Background(), "offer", domain.Command{RequestID: id, Route: domain.Route{Endpoint: server.URL}})
	if !errors.Is(err, domain.ErrInvalid) || strings.Contains(err.Error(), "sensitive") {
		t.Fatal("unsafe error mapping", err)
	}
}

// TestHTTPMediaClientRedirectTimeoutAndBoundedResponse проверяет перенаправления, таймаут и ограничение ответа.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestHTTPMediaClientRedirectTimeoutAndBoundedResponse(t *testing.T) {
	var redirected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer other.Close()
	redirect := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
		}))
	defer redirect.Close()
	client := NewHTTPClient(strings.Repeat("i", 32), 50*time.Millisecond)
	defer client.Close()
	if _, err := client.Call(context.Background(), "join", domain.Command{RequestID: uuid.NewString(), Route: domain.Route{Endpoint: redirect.URL}}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("redirect should be rejected")
	}
	if redirected.Load() != 0 {
		t.Fatal("credential-bearing redirect followed")
	}
	slow := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(120 * time.Millisecond)
			_, _ = w.Write([]byte(`{}`))
		}))
	defer slow.Close()
	if _, err := client.Call(context.Background(), "join", domain.Command{Route: domain.Route{Endpoint: slow.URL}}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("signaling timeout not enforced")
	}
	huge := httptest.NewServer(http.HandlerFunc( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 262145))) }))
	defer huge.Close()
	if _, err := client.Call(context.Background(), "join", domain.Command{Route: domain.Route{Endpoint: huge.URL}}); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("oversized response accepted")
	}
}

func TestHTTPMediaClientPreservesCauseAndLegacyCode(t *testing.T) {
	client := NewHTTPClient(strings.Repeat("i", 32), time.Second)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Call(ctx, "join", domain.Command{Route: domain.Route{Endpoint: "http://127.0.0.1:12345"}})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, domain.ErrUnavailable) || err.Error() != "media_unavailable" {
		t.Fatalf("cause or safe code lost: %v", err)
	}
	for _, body := range []string{`{"error":"screen_sharing_conflict"}`, `{"code":"screen_sharing_conflict","error":"SQL private"}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(409); _, _ = w.Write([]byte(body)) }))
		_, err := client.Call(context.Background(), "offer", domain.Command{Route: domain.Route{Endpoint: server.URL}})
		server.Close()
		if !errors.Is(err, domain.ErrScreenConflict) {
			t.Fatalf("legacy/new wire code: %v", err)
		}
	}
}
