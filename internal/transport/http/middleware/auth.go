package httpmiddleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

const authenticatedUserIDKey = "authenticated_user_id"

type TokenVerifier interface{ Verify(string) (string, error) }

func Authenticate(tokens TokenVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") || tokens == nil {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		id, err := tokens.Verify(header[1])
		if err != nil || id == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		c.Set(authenticatedUserIDKey, id)
		c.Next()
	}
}

func UserID(c *gin.Context) string { return c.GetString(authenticatedUserIDKey) }
