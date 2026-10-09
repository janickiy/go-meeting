package personalapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// PeerPresence читает присутствие только собеседника доступного личного чата.
// Сначала проверяет членство через репозиторий, затем читает серверный идентификатор
// собеседника из хранилища присутствия без продления его сессии. Произвольные
// идентификаторы пользователя из query-параметров не используются.
// При сбое, отмене или неполном ответе хранилища возвращает недоступность, а не офлайн.
// @args c — запрос Gin с UUID переписки и подтверждённой middleware идентичностью аккаунта.
// @return JSON с PeerPresence либо безопасная ошибка без статуса присутствия.
func (h *Handler) PeerPresence(c *gin.Context) {
	id, err := chat.UUID(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "failed", "message": "invalid conversationId"})
		return
	}
	actor, err := chat.UUID(middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	request := c.Request.Context()
	if request.Err() != nil {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	item, err := h.Repo.Get(request, actor, id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if item.ID != id || item.Type != "direct" || item.Peer == nil {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	peer, err := chat.UUID(item.Peer.ID)
	if err != nil || peer == actor {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	if request.Err() != nil || h.Presence == nil {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	lookup, cancel := context.WithTimeout(request, 2*time.Second)
	defer cancel()
	statuses, err := h.Presence.Online(lookup, []string{peer})
	if err != nil || lookup.Err() != nil || request.Err() != nil {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	online, known := statuses[peer]
	if !known {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": personal.PeerPresence{
		ConversationID: id, PeerID: peer, Online: online,
	}})
}
