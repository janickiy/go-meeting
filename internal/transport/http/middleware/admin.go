package httpmiddleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// AdminChecker reads the current persisted global capability for an authenticated user.
type AdminChecker interface {
	IsAdmin(context.Context, string) (bool, error)
}

// RequireAdmin checks the database on every request; conference roles and stale JWTs cannot grant access.
func RequireAdmin(checker AdminChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || UserID(c) == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		allowed, err := checker.IsAdmin(c.Request.Context(), UserID(c))
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !allowed {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Next()
	}
}
