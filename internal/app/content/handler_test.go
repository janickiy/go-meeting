package contentapp_test

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	contentapp "github.com/janickiy/go-recorder/internal/app/content"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const actor = "1e5806a1-7e74-45f6-8bbf-d034f9dd7c19"
const cid = "03f62a99-bf60-48aa-8bd8-c19e76c5bc22"
const rid = "ac174d19-b4ef-45a4-a85a-3989a1798c22"

// verifier принимает только test bearer и выдаёт server identity.
type verifier struct{}

// Verify не получает userID из запроса и исключает ID spoofing.
// @args token — тестовая строка Bearer.
// @return authenticated actor или ошибка token.
func (verifier) Verify(token string) (string, error) {
	if token != "test" {
		return "", errors.New("invalid token")
	}
	return actor, nil
}

// fakeService реализует ограниченную HTTP fixture без provider calls.
type fakeService struct {
	contentapp.Service
	called bool
	denied bool
}

// Transcript выдаёт nullable state и authoritative capability.
// @args ctx/user/cid/rid — область авторизованного request.
// @return controlled state, без nested envelope.
func (s *fakeService) Transcript(_ context.Context, user, conference, recording string) (domain.TranscriptState, error) {
	s.called = true
	if user != actor || conference != cid || recording != rid {
		return domain.TranscriptState{}, apperrors.ErrInvalidInput
	}
	if s.denied {
		return domain.TranscriptState{}, apperrors.ErrForbidden
	}
	return domain.TranscriptState{Enabled: false, ProviderMode: "noop"}, nil
}

// Search выдаёт plain-text fixture с timestamp и фильтром server actor.
// @args ctx/user/query — проверяемый contract HTTP parsing.
// @return bounded search page.
func (s *fakeService) Search(_ context.Context, user string, q domain.SearchQuery) (domain.SearchPage, error) {
	s.called = true
	return domain.SearchPage{Items: []domain.SearchResult{}, Limit: q.Limit, Offset: q.Offset}, nil
}

// RetryTranscript только ставит fixture generation в очередь.
// @args ctx/user/cid/rid — auth/resource identity.
// @return metadata queued без внешнего вызова.
func (s *fakeService) RetryTranscript(context.Context, string, string, string) (domain.TranscriptState, error) {
	s.called = true
	return domain.TranscriptState{Enabled: true, ProviderMode: "mock", Item: &domain.Transcript{Status: domain.Queued}}, nil
}

// TestContentHTTPPrivacyAndValidation проверяет cache/auth, UUID/JSON/date errors и nullable response.
// @args t — test runner.
func TestContentHTTPPrivacyAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	router := gin.New()
	httptransport.RegisterContentRoutes(router, contentapp.NewHandler(service), httpmiddleware.Authenticate(verifier{}))
	base := "/api/v1/conferences/" + cid + "/recordings/" + rid
	for _, tc := range []struct {
		method, path, body, token string
		want                      int
		called                    bool
	}{
		{"GET", base + "/transcript", "", "", 401, false},
		{"GET", base + "/transcript", "", "test", 200, true},
		{"GET", "/api/v1/conferences/invalid/recordings/" + rid + "/transcript", "", "test", 422, false},
		{"POST", base + "/transcript/retry", `{"providerToken":"spoof"}`, "test", 400, false},
		{"POST", base + "/transcript/retry", `{}`, "test", 202, true},
		{"GET", "/api/v1/search?q=test&from=tomorrow", "", "test", 422, false},
		{"GET", "/api/v1/search?q=test&limit=wrong", "", "test", 422, false},
		{"GET", "/api/v1/search?q=test&from=2026-10-01T00%3A00%3A00Z", "", "test", 200, true},
	} {
		service.called = false
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		if tc.token != "" {
			request.Header.Set("Authorization", "Bearer "+tc.token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != tc.want || service.called != tc.called {
			t.Fatalf("%s %s: %d %s called=%t", tc.method, tc.path, response.Code, response.Body.String(), service.called)
		}
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("private content cache allowed")
		}
		if strings.HasSuffix(tc.path, "/transcript") && tc.want == http.StatusOK && !strings.Contains(response.Body.String(), `"item":null`) {
			t.Fatal("missing transcript fabricated lifecycle state", response.Body.String())
		}
	}
	service.denied = true
	request := httptest.NewRequest("GET", base+"/transcript", nil)
	request.Header.Set("Authorization", "Bearer test")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal(response.Code)
	}
}
