package personalapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
	"strconv"
)

type Repository interface {
	GroupRepository
	Account(context.Context, string) error
	GetOrCreate(context.Context, string, string) (personal.Conversation, bool, error)
	Get(context.Context, string, string) (personal.Conversation, error)
	List(context.Context, string, string, int) (personal.Page, error)
	ListFiltered(context.Context, string, string, int, personal.ListFilter) (personal.Page, error)
	Search(context.Context, string, string) ([]personal.Peer, error)
	SetDirectPreferences(context.Context, string, string, bool) (personal.Conversation, error)
	ClearDirectHistory(context.Context, string, string) (personal.Conversation, error)
	HideDirectConversation(context.Context, string, string) (int64, error)
}
type Handler struct {
	Repo     Repository
	Events   *personalusecase.Events
	Presence GroupPresence
}

func (h *Handler) AccountOnly(c *gin.Context) {
	if err := h.Repo.Account(c.Request.Context(), middleware.UserID(c)); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Next()
}
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		UserID string `json:"userId"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	id, err := chat.UUID(body.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, created, err := h.Repo.GetOrCreate(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if created && h.Events != nil {
		_ = h.Events.PublishConversation(c.Request.Context(), item.ID, "conversation.updated", gin.H{"conversationId": item.ID, "type": "direct"})
	}
	status := 200
	if created {
		status = 201
	}
	c.JSON(status, gin.H{"status": "success", "item": item})
}
func (h *Handler) List(c *gin.Context) {
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = n
	}
	filter := personal.ListFilter{Type: c.Query("type"), Search: c.Query("search")}
	if raw, present := c.GetQuery("unreadOnly"); present {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		filter.UnreadOnly = value
	}
	page, err := h.Repo.ListFiltered(c.Request.Context(), middleware.UserID(c), c.Query("before"), limit, filter)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (h *Handler) Get(c *gin.Context) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	item, err := h.Repo.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "item": item})
}
func (h *Handler) Search(c *gin.Context) {
	items, err := h.Repo.Search(c.Request.Context(), middleware.UserID(c), c.Query("search"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}
