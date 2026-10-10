package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/personal"
	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

type conversationAccessChecker interface {
	HasActiveMembership(context.Context, string, string) (bool, error)
}

// A delivery guard serializes the bounded socket write with member revocation.
// Concurrent deliveries take a shared lock; membership changes take an exclusive
// lock. A completed removal therefore cannot be followed by a queued message.
type conversationDeliveryGuard interface {
	WithConversationDelivery(context.Context, string, string, func() error) (bool, error)
}

// conversationMessageDeliveryGuard защищает отправку сообщения от запоздалой
// очереди после очистки истории и передаёт текущие личные настройки получателя.
type conversationMessageDeliveryGuard interface {
	WithConversationMessageDelivery(context.Context, string, string, string, func(bool, int64) error) (bool, error)
}

// directActionDeliveryGuard заменяет запоздалую личную нагрузку текущей
// проекцией владельца и отклоняет скрытие уже восстановленной беседы.
type directActionDeliveryGuard interface {
	WithDirectActionDelivery(context.Context, string, string, string, func(*personal.Conversation) error) (bool, error)
}

// deliver проверяет право на событие непосредственно перед записью в сокет.
// Под блокировкой беседы она дополняет сообщение актуальным флагом уведомлений
// и удаляет цитату из очищенной истории, не подавляя сами новые сообщения.
//
// @args
//   - ctx — контекст проверки и ограниченной записи.
//   - user — авторизованный получатель сокета.
//   - data — сериализованный конверт из персонального серверного канала.
//   - write — запись проверенного и при необходимости очищенного конверта.
//
// @return ошибка проверки или записи; недоступное событие молча пропускается.
func (h *UserHandler) deliver(ctx context.Context, user string, data []byte, write func([]byte) error) error {
	var event domain.Envelope
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	if !strings.HasPrefix(event.Type, "message.") && !strings.HasPrefix(event.Type, "conversation.") {
		return write(data)
	}
	var payload struct {
		ConversationID string        `json:"conversationId"`
		Type           string        `json:"type"`
		UserID         string        `json:"userId"`
		Message        *chat.Message `json:"message"`
	}
	if json.Unmarshal(event.Data, &payload) != nil || (payload.Type != "direct" && payload.Type != "group") {
		return nil
	}
	conversation, err := uuid.Parse(payload.ConversationID)
	if err != nil || conversation == uuid.Nil || conversation.String() != payload.ConversationID {
		return nil
	}
	// A revoked user needs only this lifecycle event to discard local caches.
	// Reject a purported revoke carrying metadata/messages before bypassing auth.
	if event.Type == "conversation.member.removed" && payload.UserID == user {
		var fields map[string]json.RawMessage
		if json.Unmarshal(event.Data, &fields) != nil {
			return nil
		}
		for key := range fields {
			if key != "conversationId" && key != "type" && key != "userId" && key != "role" {
				return nil
			}
		}
		return write(data)
	}
	if event.Type == "conversation.preferences.updated" || event.Type == "conversation.history.cleared" || event.Type == "conversation.hidden" {
		// Персональные настройки и очистка не публикуются собеседнику.
		if payload.Type != "direct" || payload.UserID != user {
			return nil
		}
		if guard, ok := h.accounts.(directActionDeliveryGuard); ok {
			_, err := guard.WithDirectActionDelivery(ctx, payload.ConversationID, user, event.Type, func(item *personal.Conversation) error {
				if item == nil {
					return errors.New("missing authoritative conversation state")
				}
				fields := map[string]any{"conversationId": payload.ConversationID, "type": "direct", "userId": user}
				fields["historyClearedThrough"] = item.HistoryClearedThrough
				if event.Type == "conversation.hidden" {
					fields["hidden"] = true
				} else {
					fields["item"] = *item
				}
				var err error
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if strings.HasPrefix(event.Type, "message.") {
		if guard, ok := h.accounts.(conversationMessageDeliveryGuard); ok {
			if payload.Message == nil || payload.Message.ConversationID != payload.ConversationID {
				return nil
			}
			messageID, err := uuid.Parse(payload.Message.ID)
			if err != nil || messageID == uuid.Nil || messageID.String() != payload.Message.ID {
				return nil
			}
			_, err = guard.WithConversationMessageDelivery(ctx, payload.ConversationID, user, payload.Message.ID, func(notificationsEnabled bool, cutoff int64) error {
				message := *payload.Message
				if cutoff > 0 && message.ReplyPreview != nil && message.ReplyPreview.Sequence <= cutoff {
					message.ReplyPreview = nil
					message.ReplyTo = nil
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(event.Data, &fields); err != nil {
					return err
				}
				fields["message"], err = json.Marshal(message)
				if err != nil {
					return err
				}
				fields["notificationsEnabled"], err = json.Marshal(notificationsEnabled)
				if err != nil {
					return err
				}
				fields["historyClearedThrough"], err = json.Marshal(cutoff)
				if err != nil {
					return err
				}
				event.Data, err = json.Marshal(fields)
				if err != nil {
					return err
				}
				outbound, err := json.Marshal(event)
				if err != nil {
					return err
				}
				return write(outbound)
			})
			return err
		}
	}
	if guard, ok := h.accounts.(conversationDeliveryGuard); ok {
		_, err := guard.WithConversationDelivery(ctx, payload.ConversationID, user, func() error { return write(data) })
		return err
	}
	if checker, ok := h.accounts.(conversationAccessChecker); ok {
		allowed, err := checker.HasActiveMembership(ctx, payload.ConversationID, user)
		if err != nil || !allowed {
			return err
		}
		return write(data)
	}
	// Compatibility for direct-only test/adapters; group delivery fails closed.
	if payload.Type == "direct" {
		return write(data)
	}
	return nil
}
