package httpmiddleware_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

func TestGuestSessionScope(t *testing.T) {
	tokens, err := security.NewTokenService(strings.Repeat("guest-test-secret", 3))
	if err != nil {
		t.Fatal(err)
	}
	userID, conferenceID := uuid.NewString(), uuid.NewString()
	token, err := tokens.IssueGuest(userID, conferenceID)
	if err != nil {
		t.Fatal(err)
	}
	id, scope, expiry, err := tokens.VerifySession(token)
	if err != nil || id != userID || scope != conferenceID || expiry.IsZero() {
		t.Fatal("guest token lost its scope")
	}
	account, _ := tokens.Issue(userID)
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/v1/auth/me", 200}, {"POST", "/api/v1/auth/logout", 200},
		{"GET", "/api/v1/capabilities", 200}, {"GET", "/api/v1/webrtc/config", 200},
		{"GET", "/api/v1/conferences/" + conferenceID, 200},
		{"POST", "/api/v1/conferences/" + conferenceID + "/messages", 200},
		{"POST", "/api/v1/conferences/" + conferenceID + "/ws-ticket", 200},
		{"GET", "/api/v1/conferences/" + uuid.NewString(), 403},
		{"POST", "/api/v1/conferences", 403}, {"GET", "/api/v1/admin/summary", 403},
		{"GET", "/api/v1/me/conferences", 403}, {"PATCH", "/api/v1/auth/me", 403},
		{"GET", "/api/v1/notifications", 403}, {"GET", "/api/v1/search", 403},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			router := gin.New()
			router.Handle(tc.method, tc.path, middleware.Authenticate(tokens), func(c *gin.Context) { c.Status(200) })
			for _, identity := range []struct {
				raw      string
				expected int
			}{{token, tc.status}, {account, 200}, {token + "bad", 401}, {"", 401}} {
				req := httptest.NewRequest(tc.method, tc.path, nil)
				if identity.raw != "" {
					req.Header.Set("Authorization", "Bearer "+identity.raw)
				}
				out := httptest.NewRecorder()
				router.ServeHTTP(out, req)
				if out.Code != identity.expected {
					t.Fatalf("status=%d expected=%d", out.Code, identity.expected)
				}
			}
		})
	}
}
