// Пакет captionsapp связывает настройки живого распознавания с авторизованным HTTP.
package captionsapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/config"
	analytics "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	live "github.com/janickiy/meet-space/internal/usecase/captions"
	"net/http"
	"strconv"
	"time"
)

// Handler возвращает сохранённые финальные реплики и настройки, не открывая провайдер в API.
type Handler struct {
	Repo      live.Repository
	Enabled   bool
	Analytics interface {
		Read(context.Context, string, string) (analytics.Conference, error)
	}
	Search interface {
		Reindex(context.Context, string, string, string, config.StageEightConfig, time.Duration) error
	}
	Config config.StageEightConfig
}

// ReadAnalytics возвращает объективные агрегаты только допущенным участникам встречи.
// @args c — авторизованный запрос с идентификатором конференции.
func (h *Handler) ReadAnalytics(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	value, err := h.Analytics.Read(c.Request.Context(), middleware.UserID(c), cid)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}

// Reindex ставит ограниченную по частоте переиндексацию для организатора или соведущего.
// @args c — авторизованный запрос с идентификаторами встречи и записи.
func (h *Handler) Reindex(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	rid, err := uuid.Parse(c.Param("recordingId"))
	if err != nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	err = h.Search.Reindex(c.Request.Context(), middleware.UserID(c), cid, rid.String(), h.Config, 5*time.Minute)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success"})
}

// id проверяет UUID конференции до выполнения SQL.
// @args c — авторизованный HTTP запрос.
// @return нормализованный UUID и признак успешной проверки.
func id(c *gin.Context) (string, bool) {
	v, e := uuid.Parse(c.Param("id"))
	if e != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// Read показывает согласие и состояние всем допущенным участникам и право изменения организатору.
// @args c — authenticated request.
func (h *Handler) Read(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	s, e := h.Repo.Read(c.Request.Context(), middleware.UserID(c), cid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	s.CanManage = s.CanManage && h.Enabled
	if !s.Enabled {
		s.Status = "off"
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Set принимает только enabled/language, без произвольных параметров провайдера и URI.
// @args c — запрос организатора с JSON.
func (h *Handler) Set(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Enabled  *bool  `json:"enabled"`
		Language string `json:"language"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.Enabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !h.Enabled && *request.Enabled {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	if request.Language == "" {
		request.Language = "auto"
	}
	s, e := h.Repo.Set(c.Request.Context(), middleware.UserID(c), cid, *request.Enabled, request.Language)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	s.Available = h.Enabled
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": s})
}

// Finals восстанавливает пропущенные финальные события WebSocket через курсорную пагинацию.
// @args c — authenticated request с afterCursor/limit.
func (h *Handler) Finals(c *gin.Context) {
	cid, ok := id(c)
	if !ok {
		return
	}
	after, e := strconv.ParseInt(c.DefaultQuery("afterCursor", "0"), 10, 64)
	limit, e2 := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if e != nil || e2 != nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	items, e := h.Repo.Finals(c.Request.Context(), middleware.UserID(c), cid, after, limit)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	cursor := after
	for _, v := range items {
		cursor = max(cursor, v.Cursor)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items, "nextCursor": cursor, "hasMore": len(items) == limit})
}
