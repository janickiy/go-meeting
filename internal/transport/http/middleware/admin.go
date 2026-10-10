package httpmiddleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// AdminChecker читает текущие сохранённые глобальные полномочия авторизованного пользователя.
type AdminChecker interface {
	IsAdmin(context.Context, string) (bool, error)
}

// RequireAdmin проверяет БД при каждом запросе; роли конференции и устаревшие JWT не предоставляют доступ.
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
