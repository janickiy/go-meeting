package personal

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type Members interface {
	Members(context.Context, string) ([]string, error)
}
type conversationTypes interface {
	ConversationType(context.Context, string) (string, error)
}
type Bus interface {
	Publish(context.Context, string, realtime.Envelope) error
}

// Events adapts the shared chat application to per-account delivery after persistence.
type Events struct {
	Members Members
	Bus     Bus
}

func (e *Events) Broadcast(ctx context.Context, event realtime.Envelope) error {
	var m chat.Message
	if err := json.Unmarshal(event.Data, &m); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, m.ConversationID)
	if err != nil {
		return err
	}
	return e.PublishConversation(ctx, m.ConversationID, strings.TrimPrefix(event.Type, "chat."), map[string]any{"conversationId": m.ConversationID, "type": kind, "message": m})
}
func (e *Events) PublishConversation(ctx context.Context, id, kind string, data any) error {
	return e.PublishConversationTo(ctx, id, kind, data)
}

// PublishConversationTo also delivers a minimal lifecycle event to a removed user.
// Callers persist first; extra recipients must never receive private messages.
func (e *Events) PublishConversationTo(ctx context.Context, id, kind string, data any, extraUsers ...string) error {
	ids, err := e.Members.Members(ctx, id)
	if err != nil {
		return err
	}
	if len(extraUsers) > 0 && kind != "conversation.member.removed" {
		return errors.New("extra recipients require a membership removal event")
	}
	ids = append(ids, extraUsers...)
	seen := make(map[string]struct{}, len(ids))
	event := realtime.Event(kind, "", data)
	for _, user := range ids {
		if _, exists := seen[user]; exists {
			continue
		}
		seen[user] = struct{}{}
		err = errors.Join(err, e.Bus.Publish(ctx, user, event))
	}
	return err
}
func (e *Events) SendToParticipant(ctx context.Context, id, user string, event realtime.Envelope) error {
	var state chat.ReadState
	if err := json.Unmarshal(event.Data, &state); err != nil {
		return err
	}
	kind, err := e.conversationType(ctx, id)
	if err != nil {
		return err
	}
	return e.Bus.Publish(ctx, user, realtime.Event("conversation.read.updated", "", map[string]any{"conversationId": id, "type": kind, "state": state}))
}

func (e *Events) conversationType(ctx context.Context, id string) (string, error) {
	if types, ok := e.Members.(conversationTypes); ok {
		return types.ConversationType(ctx, id)
	}
	// Compatibility for existing direct-only adapters.
	return "direct", nil
}
