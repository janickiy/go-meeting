package recordsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// End завершает запись и отправляет worker-у команду финализации.
// @args
// - c: Gin context HTTP-запроса.
// @return JSON response.
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
