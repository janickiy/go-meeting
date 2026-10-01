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

type Service interface {
	Create(context.Context, string, conferences.CreateRequest) (conferences.View, error)
	List(context.Context, string, int, int) ([]conferences.View, error)
	Read(context.Context, string, string) (conferences.View, error)
	Transition(context.Context, string, string, conferences.Status) (conferences.View, error)
	Participants(context.Context, string, string, int, int) ([]conferences.ParticipantView, error)
	Join(context.Context, string, string, conferences.JoinRequest) (conferences.ParticipantView, error)
	Leave(context.Context, string, string) (conferences.ParticipantView, error)
	LookupInvite(context.Context, string) (conferences.InviteView, error)
	JoinInvite(context.Context, string, string) (conferences.ParticipantView, error)
	Self(context.Context, string, string) (conferences.ParticipantView, error)
	Admission(context.Context, string, string, string, conferences.AdmissionRequest) (conferences.ParticipantView, error)
	Schedule(context.Context, string, string, conferences.ScheduleRequest) (conferences.View, error)
	Timeline(context.Context, string, conferences.TimelineQuery) (conferences.TimelinePage, error)
	History(context.Context, string, string) (conferences.HistoryView, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c *gin.Context) {
	var request conferences.CreateRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), httpmiddleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "item": item})
}

func (h *Handler) List(c *gin.Context) {
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

func (h *Handler) Read(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

func (h *Handler) Transition(target conferences.Status) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := conferenceID(c)
		if !ok {
			return
		}
		if !httpresponse.BindJSON(c, &struct{}{}, true) {
			return
		}
		item, err := h.service.Transition(c.Request.Context(), httpmiddleware.UserID(c), id, target)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
	}
}

func (h *Handler) Participants(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.Participants(c.Request.Context(), httpmiddleware.UserID(c), id, limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

func (h *Handler) Join(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.JoinRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.Join(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

func (h *Handler) Leave(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Leave(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

func (h *Handler) LookupInvite(c *gin.Context) {
	item, err := h.service.LookupInvite(c.Request.Context(), c.Param("code"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

func (h *Handler) JoinInvite(c *gin.Context) {
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.JoinInvite(c.Request.Context(), httpmiddleware.UserID(c), c.Param("code"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

func conferenceID(c *gin.Context) (string, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, "conference id must be a non-zero UUID"))
		return "", false
	}
	return id.String(), true
}
