package recordingsapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type Service interface {
	Start(context.Context, string, string, records.ConferenceStartRequest) (records.RecordCard, error)
	Stop(context.Context, string, string, string) (records.RecordCard, error)
	Read(context.Context, string, string, string) (records.RecordCard, error)
	List(context.Context, string, string, int, int) ([]records.RecordCard, error)
}
type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }
func parameter(c *gin.Context, key string) (string, bool) {
	id, err := uuid.Parse(c.Param(key))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, key+" must be a non-zero UUID"))
		return "", false
	}
	return id.String(), true
}
func (h *Handler) Start(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request records.ConferenceStartRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.Start(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": item})
}
func (h *Handler) Stop(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	recordID, ok := parameter(c, "recordingId")
	if !ok {
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Stop(c.Request.Context(), httpmiddleware.UserID(c), id, recordID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": item})
}
func (h *Handler) Read(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	recordID, ok := parameter(c, "recordingId")
	if !ok {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id, recordID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
func (h *Handler) List(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, offset, ok := httpresponse.Pagination(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), id, limit, offset)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}
