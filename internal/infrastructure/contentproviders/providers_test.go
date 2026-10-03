package contentproviders

import (
	"context"
	"encoding/json"
	"errors"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestHTTPTranscription проверяет потоковый multipart и стабильный заголовок идемпотентности.
// @args t — test runner; server работает только на loopback.
func TestHTTPTranscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Idempotency-Key") != "same-job" {
			t.Error("missing bounded provider auth/idempotency")
		}
		if e := r.ParseMultipartForm(1024); e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, e := r.FormFile("audio")
		if e != nil {
			t.Error(e)
			return
		}
		defer file.Close()
		data, _ := io.ReadAll(file)
		if string(data) != "wav-data" || r.FormValue("language") != "auto" {
			t.Error("wrong streaming payload")
		}
		_ = json.NewEncoder(w).Encode(domain.TranscriptionResult{Language: "ru", Segments: []domain.Segment{{Text: "Тест", StartMS: 0, EndMS: 100}}})
	}))
	defer server.Close()
	p, e := NewTranscriptionProvider("http", server.URL, "secret", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	out, e := p.Transcribe(context.Background(), domain.TranscriptionRequest{Audio: strings.NewReader("wav-data"), Size: 8, Language: "auto", IdempotencyKey: "same-job"})
	if e != nil || out.Language != "ru" || len(out.Segments) != 1 {
		t.Fatal(out, e)
	}
}

// TestProviderFailureClassification проверяет ограниченный Retry-After и безопасные ошибки без тела ответа сервера.
// @args t — test runner.
func TestProviderFailureClassification(t *testing.T) {
	for _, status := range []int{400, 401, 429, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "999999")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "private transcript token-secret")
			}))
			defer server.Close()
			p, _ := NewAIProvider("http", server.URL, "secret", "model", time.Second)
			_, e := p.Summarize(context.Background(), domain.AIRequest{InputJSON: `{}`, IdempotencyKey: "same"})
			var classified *jobs.Error
			if !errors.As(e, &classified) || classified.Retryable != (status == 429 || status >= 500) || classified.RetryAfter != time.Hour || strings.Contains(e.Error(), "secret") {
				t.Fatal(e)
			}
		})
	}
}

// TestNoopMockAndRedirectPrivacy проверяет достоверность режимов и запрет передачи Bearer на адрес перенаправления.
// @args t — test runner.
func TestNoopMockAndRedirectPrivacy(t *testing.T) {
	noop, _ := NewTranscriptionProvider("noop", "", "", time.Second)
	if _, e := noop.Transcribe(context.Background(), domain.TranscriptionRequest{}); !errors.Is(e, jobs.ErrSkip) {
		t.Fatal(e)
	}
	mock, _ := NewTranscriptionProvider("mock", "", "", time.Second)
	out, e := mock.Transcribe(context.Background(), domain.TranscriptionRequest{DurationSec: 10})
	if e != nil || !strings.Contains(out.Segments[0].Text, "демонстрационный") {
		t.Fatal(e)
	}
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	ai, _ := NewAIProvider("http", redirect.URL, "secret", "model", time.Second)
	_, e = ai.Summarize(context.Background(), domain.AIRequest{InputJSON: `{}`})
	if e == nil || called {
		t.Fatal("redirect exposed credential")
	}
}
