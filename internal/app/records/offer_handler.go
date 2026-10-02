package recordsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// Offer проксирует SDP offer браузера во внутренний worker.
// @args
// - c: Gin context HTTP-запроса.
// @return SDP answer worker-а.
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
