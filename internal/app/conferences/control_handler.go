package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type ControlService interface {
	Moderate(context.Context, string, string, string, conferences.ModerationRequest) (conferences.ParticipantView, error)
	UpdateMediaState(context.Context, string, string, conferences.MediaState) (conferences.ParticipantView, error)
}
type ControlHandler struct{ service ControlService }

func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}
func (h *ControlHandler) Moderate(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	participant, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participant == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request conferences.ModerationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Moderate(c.Request.Context(), httpmiddleware.UserID(c), id, participant.String(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
func (h *ControlHandler) Media(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.MediaState
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.UpdateMediaState(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
