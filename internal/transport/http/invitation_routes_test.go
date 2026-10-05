package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	app "github.com/janickiy/go-recorder/internal/app/conferences"
	d "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type invitationRouteService struct{ called bool }

func (s *invitationRouteService) Search(context.Context, string, string, string) ([]d.InvitationUser, error) {
	s.called = true
	return []d.InvitationUser{}, nil
}
func (s *invitationRouteService) Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error) {
	s.called = true
	return []d.InvitationResult{}, nil
}

func TestInvitationRoutesAuthenticationLimitsAndStrictJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, _ := security.NewTokenService(strings.Repeat("invitation-test-secret-", 3))
	token, _ := tokens.Issue("d781f8eb-dff7-4a51-b018-7c0b1eeb3b62")
	limiter := &integrationLimiter{}
	service := &invitationRouteService{}
	router := gin.New()
	RegisterInvitationRoutes(router, &app.InvitationHandler{Service: service}, middleware.Authenticate(tokens), limiter)
	base := APIV1Prefix + "/conferences/cc78300e-15d8-429b-a51d-ecac7a3c7d6e"
	request := func(method, path, body string, bearer bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if bearer {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Header().Get("Cache-Control") != "private, no-store" {
			t.Error("invitation result can be cached")
		}
		return response
	}
	if got := request("POST", base+"/invitations", `{"emails":["test@example.org"]}`, false); got.Code != 401 || service.called {
		t.Fatal("unauthenticated invitations")
	}
	for _, test := range []struct{ method, path, scope string }{{"POST", base + "/invitations", "meeting_invitation_send"}, {"GET", base + "/invitation-users?query=member", "meeting_invitation_search"}} {
		if got := request(test.method, test.path, `{}`, true); got.Code != 429 || !strings.Contains(limiter.keys[len(limiter.keys)-1], test.scope) || service.called {
			t.Fatal("invitation endpoint not limited", got.Code, limiter.keys)
		}
	}
	limiter.allowed = true
	for _, body := range []string{`{"emails":["test@example.org"],"smtpPassword":"secret"}`, `{"emails":["test@example.org"]}{}`} {
		if got := request("POST", base+"/invitations", body, true); got.Code != http.StatusBadRequest || service.called {
			t.Fatal("untrusted JSON reached invitations", got.Code)
		}
	}
	if got := request("POST", base+"/invitations", `{"emails":["test@example.org"]}`, true); got.Code != http.StatusAccepted || !service.called {
		t.Fatal("invitation acceptance contract", got.Code)
	}
}
