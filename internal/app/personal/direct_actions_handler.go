package personalapp

import (
	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	middleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// directActionID проверяет идентификатор из маршрута до обращения в хранилище.
// @args: c — текущий запрос.
// @return: нормализованный UUID и признак успешной проверки.
func directActionID(c *gin.Context) (string, bool) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		httpresponse.Fail(c, err)
		return "", false
	}
	return id, true
}

// publishDirectAction синхронизирует настройку между вкладками только её владельца.
// @args: c — запрос; kind — тип события; id — переписка; item — персональная проекция;
// cutoff — подтверждённая граница истории для минимального события скрытия без проекции.
func (h *Handler) publishDirectAction(c *gin.Context, kind, id string, item *personal.Conversation, cutoff int64) {
	if h.Events == nil {
		return
	}
	data := gin.H{"conversationId": id, "type": "direct", "userId": middleware.UserID(c)}
	if item != nil {
		data["item"] = *item
		data["historyClearedThrough"] = item.HistoryClearedThrough
	} else if kind == "conversation.hidden" {
		data["historyClearedThrough"] = cutoff
		data["hidden"] = true
	}
	_ = h.Events.PublishUser(c.Request.Context(), middleware.UserID(c), kind, data)
}

// SetDirectPreferences принимает явное разрешение уведомлений для личного чата.
// @args: c — авторизованный запрос с notificationsEnabled.
func (h *Handler) SetDirectPreferences(c *gin.Context) {
	id, ok := directActionID(c)
	if !ok {
		return
	}
	var body struct {
		NotificationsEnabled *bool `json:"notificationsEnabled"`
	}
	if !httpresponse.BindJSON(c, &body, false) {
		return
	}
	if body.NotificationsEnabled == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	item, err := h.Repo.SetDirectPreferences(c.Request.Context(), middleware.UserID(c), id, *body.NotificationsEnabled)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishDirectAction(c, "conversation.preferences.updated", id, &item, item.HistoryClearedThrough)
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// ClearDirectHistory удаляет доступ к прежней истории только у текущего пользователя.
// @args: c — авторизованный запрос с идентификатором личного чата.
func (h *Handler) ClearDirectHistory(c *gin.Context) {
	id, ok := directActionID(c)
	if !ok {
		return
	}
	item, err := h.Repo.ClearDirectHistory(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishDirectAction(c, "conversation.history.cleared", id, &item, item.HistoryClearedThrough)
	c.JSON(200, gin.H{"status": "success", "item": item})
}

// HideDirectConversation скрывает чат у текущего пользователя, не изменяя данные собеседника.
// @args: c — авторизованный запрос с идентификатором личного чата.
func (h *Handler) HideDirectConversation(c *gin.Context) {
	id, ok := directActionID(c)
	if !ok {
		return
	}
	cutoff, err := h.Repo.HideDirectConversation(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.publishDirectAction(c, "conversation.hidden", id, nil, cutoff)
	c.JSON(200, gin.H{"status": "success", "hidden": true, "historyClearedThrough": cutoff})
}
