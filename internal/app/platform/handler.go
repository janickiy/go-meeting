// Пакет platformapp предоставляет безопасный просмотр возможностей и операционных данных администратора.
package platformapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	domain "github.com/janickiy/meet-space/internal/domain/platform"
)

type Service interface {
	Capabilities(context.Context) domain.Capabilities
	Summary(context.Context) (domain.Summary, error)
}

type Handler struct {
	Service      Service
	BuildVersion string
}

// Capabilities возвращает только действующие переключатели функций и безопасный идентификатор сборки.
func (h *Handler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "capabilities": h.Service.Capabilities(c.Request.Context()), "buildVersion": h.BuildVersion})
}

// Summary возвращает операционные агрегаты после проверки сохранённых прав администратора.
func (h *Handler) Summary(c *gin.Context) {
	value, err := h.Service.Summary(c.Request.Context())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}
