package personal

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"strings"
)

type Members interface {
	Members(context.Context, string) ([]string, error)
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
	return e.PublishConversation(ctx, m.ConversationID, strings.TrimPrefix(event.Type, "chat."), map[string]any{"conversationId": m.ConversationID, "type": "direct", "message": m})
}
func (e *Events) PublishConversation(ctx context.Context, id, kind string, data any) error {
	ids, err := e.Members.Members(ctx, id)
	if err != nil {
		return err
	}
	event := realtime.Event(kind, "", data)
	for _, user := range ids {
		err = errors.Join(err, e.Bus.Publish(ctx, user, event))
	}
	return err
}
func (e *Events) SendToParticipant(ctx context.Context, id, user string, event realtime.Envelope) error {
	var state chat.ReadState
	if err := json.Unmarshal(event.Data, &state); err != nil {
		return err
	}
	return e.Bus.Publish(ctx, user, realtime.Event("conversation.read.updated", "", map[string]any{"conversationId": id, "type": "direct", "state": state}))
}
