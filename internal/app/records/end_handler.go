package recordsapp

import (
	"net/http"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/gin-gonic/gin"
)

// End завершает запись и отправляет worker-у команду финализации.
// Параметры:
// - c: Gin context HTTP-запроса.
// Возвращает: JSON response.
func (h *Handler) End(c *gin.Context) {
	var request records.EndRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		failed(c, http.StatusBadRequest, "invalid json body")
		return
	}
	if message := records.ValidateEndRequest(request); message != "" {
		failed(c, http.StatusBadRequest, message)
		return
	}
	if err := h.service.Stop(c.Request.Context(), request); err != nil {
		failed(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "success", "message": "Record stop command accepted"})
}
