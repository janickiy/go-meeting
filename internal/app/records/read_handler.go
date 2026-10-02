package recordsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Read возвращает одну запись по UUID.
// @args
// - c: Gin context HTTP-запроса.
// @return JSON response.
func (h *Handler) Read(c *gin.Context) {
	item, err := h.service.Read(c.Request.Context(), c.Param("id"))
	if err != nil {
		failed(c, http.StatusNotFound, "record not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
