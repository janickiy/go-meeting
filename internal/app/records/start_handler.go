package recordsapp

import (
	"errors"
	"net/http"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/gin-gonic/gin"
)

// Start запускает запись.
// Параметры:
// - c: Gin context HTTP-запроса.
// Возвращает: JSON response.
func (h *Handler) Start(c *gin.Context) {
	var request records.StartRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		failed(c, http.StatusBadRequest, "invalid json body")
		return
	}
	request = records.NormalizeStartRequest(request)
	if message := records.ValidateStartRequest(request); message != "" {
		failed(c, http.StatusBadRequest, message)
		return
	}
	response, err := h.service.Start(c.Request.Context(), request)
	if err != nil {
		if errors.Is(err, records.ErrConferenceAlreadyRecording) {
			failed(c, http.StatusConflict, err.Error())
			return
		}
		failed(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusAccepted, response)
}
