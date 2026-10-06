package websocket

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
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

func (h *UserHandler) deliver(ctx context.Context, user string, data []byte, write func() error) error {
	var event domain.Envelope
	if json.Unmarshal(data, &event) != nil {
		return nil
	}
	if !strings.HasPrefix(event.Type, "message.") && !strings.HasPrefix(event.Type, "conversation.") {
		return write()
	}
	var payload struct {
		ConversationID string `json:"conversationId"`
		Type           string `json:"type"`
		UserID         string `json:"userId"`
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
		return write()
	}
	if guard, ok := h.accounts.(conversationDeliveryGuard); ok {
		_, err := guard.WithConversationDelivery(ctx, payload.ConversationID, user, write)
		return err
	}
	if checker, ok := h.accounts.(conversationAccessChecker); ok {
		allowed, err := checker.HasActiveMembership(ctx, payload.ConversationID, user)
		if err != nil || !allowed {
			return err
		}
		return write()
	}
	// Compatibility for direct-only test/adapters; group delivery fails closed.
	if payload.Type == "direct" {
		return write()
	}
	return nil
}
