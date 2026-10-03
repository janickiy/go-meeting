package httptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	platformapp "github.com/janickiy/go-recorder/internal/app/platform"
	domain "github.com/janickiy/go-recorder/internal/domain/platform"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type statusServiceStub struct{ summaryCalls int }

func (s *statusServiceStub) Capabilities(context.Context) domain.Capabilities {
	return domain.Capabilities{LiveCaptions: true, RecordingModes: []string{"composite"}}
}

func (s *statusServiceStub) Summary(context.Context) (domain.Summary, error) {
	s.summaryCalls++
	return domain.Summary{AsOf: time.Now().UTC(), ActiveConferences: 2, RecentFailures: []domain.Failure{{Kind: "content.transcribe", Code: "timeout"}}}, nil
}

type adminCheckerStub struct {
	allowed bool
	err     error
}

func (s *adminCheckerStub) IsAdmin(context.Context, string) (bool, error) { return s.allowed, s.err }

// TestPlatformStatusAuthorization verifies the persisted admin gate, including revocation of an existing JWT.
func TestPlatformStatusAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := security.NewTokenService(strings.Repeat("stage-nine-test-secret-", 2))
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.Issue(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	service := &statusServiceStub{}
	checker := &adminCheckerStub{}
	router := gin.New()
	RegisterPlatformStatusRoutes(router, &platformapp.Handler{Service: service, BuildVersion: "test-build"}, middleware.Authenticate(tokens), checker)
	call := func(path, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	if got := call("/api/v1/capabilities", ""); got.Code != 401 {
		t.Fatalf("anonymous capabilities: %d", got.Code)
	}
	if got := call("/api/v1/capabilities", token); got.Code != 200 || !strings.Contains(got.Body.String(), `"buildVersion":"test-build"`) || !strings.Contains(got.Body.String(), `"liveCaptions":true`) {
		t.Fatalf("authenticated capabilities: %d %s", got.Code, got.Body.String())
	}
	if got := call("/api/v1/admin/summary", ""); got.Code != 401 {
		t.Fatalf("anonymous admin: %d", got.Code)
	}
	if got := call("/api/v1/admin/summary", token); got.Code != 403 || service.summaryCalls != 0 {
		t.Fatalf("ordinary user reached summary: %d calls=%d", got.Code, service.summaryCalls)
	}
	checker.allowed = true
	if got := call("/api/v1/admin/summary", token); got.Code != 200 || service.summaryCalls != 1 || !strings.Contains(got.Body.String(), `"activeConferences":2`) || got.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("admin summary: %d %s", got.Code, got.Body.String())
	}
	checker.allowed = false
	if got := call("/api/v1/admin/summary", token); got.Code != 403 || service.summaryCalls != 1 {
		t.Fatalf("demoted admin retained access: %d", got.Code)
	}
	checker.err = errors.New("internal database address and private token")
	if got := call("/api/v1/admin/summary", token); got.Code != 500 || strings.Contains(got.Body.String(), "private token") {
		t.Fatalf("database failure leaked details: %d %s", got.Code, got.Body.String())
	}
}
