package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	d "github.com/janickiy/go-recorder/internal/domain/conferences"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type InvitationService interface {
	Search(context.Context, string, string, string) ([]d.InvitationUser, error)
	Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error)
}
type InvitationHandler struct{ Service InvitationService }

func (h *InvitationHandler) Search(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	items, err := h.Service.Search(c.Request.Context(), middleware.UserID(c), id, c.Query("query"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

func (h *InvitationHandler) Invite(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request d.InvitationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	items, err := h.Service.Invite(c.Request.Context(), middleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "items": items})
}
