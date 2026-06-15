package recordsapp

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// List возвращает список записей.
// Параметры:
// - c: Gin context HTTP-запроса.
// Возвращает: JSON response.
func (h *Handler) List(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	items, err := h.service.List(c.Request.Context(), limit, offset)
	if err != nil {
		failed(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
