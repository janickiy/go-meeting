package recordsapp

import (
	"net/http"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/gin-gonic/gin"
)

// Offer проксирует SDP offer браузера во внутренний worker.
// Параметры:
// - c: Gin context HTTP-запроса.
// Возвращает: SDP answer worker-а.
func (h *Handler) Offer(c *gin.Context) {
	var request records.WebRTCOfferRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		failed(c, http.StatusBadRequest, "invalid json body")
		return
	}
	if h.workerSignaler == nil {
		failed(c, http.StatusBadGateway, "worker signaling is not configured")
		return
	}
	response, err := h.workerSignaler.Offer(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		failed(c, http.StatusBadGateway, err.Error())
		return
	}

	c.JSON(http.StatusOK, response)
}
