package recordsapp

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// Start запускает запись.
// @args
// - c: контекст HTTP-запроса Gin.
// @return JSON response.
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
