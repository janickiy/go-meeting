package foldersapp

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/folders"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

type Repository interface {
	Account(context.Context, string) error
	List(context.Context, string, string, string) ([]folders.Folder, error)
	Create(context.Context, string, string) (folders.Folder, error)
	Get(context.Context, string, string) (folders.Folder, error)
	Rename(context.Context, string, string, string) (folders.Folder, error)
	Delete(context.Context, string, string) error
	Order(context.Context, string, []string) ([]folders.Folder, error)
	SetItem(context.Context, string, string, string, string, bool) (folders.Folder, error)
	Items(context.Context, string, string, string, int, folders.Filter) (folders.Page, error)
	Candidates(context.Context, string, string, string, int, folders.Filter) (folders.Page, error)
}
type Emitter interface {
	PublishFolder(context.Context, string, string, string) error
}
type Handler struct {
	Repo   Repository
	Events Emitter
}

func NewHandler(repo Repository, events Emitter) *Handler {
	return &Handler{Repo: repo, Events: events}
}
func (h *Handler) emit(c *gin.Context, kind, id string) {
	if h.Events != nil {
		_ = h.Events.PublishFolder(c.Request.Context(), middleware.UserID(c), kind, id)
	}
}
func (h *Handler) AccountOnly(c *gin.Context) {
	if err := h.Repo.Account(c.Request.Context(), middleware.UserID(c)); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Next()
}
func parameter(c *gin.Context, name string) (string, bool) {
	id, err := chat.UUID(c.Param(name))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}
func result(c *gin.Context, status int, item folders.Folder) {
	c.JSON(status, gin.H{"status": "success", "item": item})
}
func (h *Handler) List(c *gin.Context) {
	items, err := h.Repo.List(c.Request.Context(), middleware.UserID(c), c.Query("itemKind"), c.Query("itemId"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success", "items": items})
}
func (h *Handler) Create(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.Create(c.Request.Context(), middleware.UserID(c), body.Name)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.created", item.ID)
	result(c, 201, item)
}
func (h *Handler) Get(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	item, err := h.Repo.Get(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	result(c, 200, item)
}
func (h *Handler) Rename(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	item, err := h.Repo.Rename(c.Request.Context(), middleware.UserID(c), id, body.Name)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.updated", id)
	result(c, 200, item)
}
func (h *Handler) Delete(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	if err := h.Repo.Delete(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.deleted", id)
	c.JSON(200, gin.H{"status": "success"})
}
func (h *Handler) Order(c *gin.Context) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	if body.IDs == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	items, err := h.Repo.Order(c.Request.Context(), middleware.UserID(c), body.IDs)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.reordered", "")
	c.JSON(200, gin.H{"status": "success", "items": items})
}
func (h *Handler) AddItem(c *gin.Context)    { h.setItem(c, true) }
func (h *Handler) RemoveItem(c *gin.Context) { h.setItem(c, false) }
func (h *Handler) setItem(c *gin.Context, add bool) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	itemID, ok := parameter(c, "itemId")
	if !ok {
		return
	}
	kind := c.Param("kind")
	if !folders.ValidKind(kind) {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Repo.SetItem(c.Request.Context(), middleware.UserID(c), id, kind, itemID, add)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.emit(c, "folder.items.updated", id)
	result(c, 200, item)
}
func pageLimit(c *gin.Context) (int, bool) {
	limit := 50
	if raw, exists := c.GetQuery("limit"); exists {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return 0, false
		}
		limit = n
	}
	return limit, true
}
func (h *Handler) Items(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	limit, ok := pageLimit(c)
	if !ok {
		return
	}
	page, err := h.Repo.Items(c.Request.Context(), middleware.UserID(c), id, c.Query("before"), limit, folders.Filter{Type: c.Query("type"), Search: c.Query("search")})
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}
func (h *Handler) Candidates(c *gin.Context) {
	limit, ok := pageLimit(c)
	if !ok {
		return
	}
	page, err := h.Repo.Candidates(c.Request.Context(), middleware.UserID(c), c.Query("folderId"), c.Query("before"), limit, folders.Filter{Type: c.Query("type"), Search: c.Query("search")})
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, page)
}
