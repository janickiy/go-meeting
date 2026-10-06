package httpmiddleware

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

type unavailableSessionVerifier struct{ err error }

func (v unavailableSessionVerifier) Verify(string) (string, error) {
	panic("context-aware verifier must be used")
}
func (v unavailableSessionVerifier) VerifyAuthorization(context.Context, string) (string, string, string, time.Time, error) {
	return "", "", "", time.Time{}, v.err
}

func TestSessionStoreFailureIsNotAnAuthenticationRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"temporarily unavailable", apperrors.ErrUnavailable, 503},
		{"invalid or revoked session", apperrors.ErrUnauthorized, 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/api/v1/auth/me", Authenticate(unavailableSessionVerifier{test.err}), func(c *gin.Context) { c.Status(200) })
			request := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
			request.Header.Set("Authorization", "Bearer token")
			result := httptest.NewRecorder()
			router.ServeHTTP(result, request)
			if result.Code != test.status {
				t.Fatalf("session store failure was misclassified: %d", result.Code)
			}
		})
	}
}
