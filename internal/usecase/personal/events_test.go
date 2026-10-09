package personal

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type eventMembers struct {
	ids  []string
	kind string
	err  error
}

func (m eventMembers) Members(context.Context, string) ([]string, error)        { return m.ids, m.err }
func (m eventMembers) ConversationType(context.Context, string) (string, error) { return m.kind, m.err }

type eventBus struct {
	events map[string][]realtime.Envelope
}

func (b *eventBus) Publish(_ context.Context, user string, e realtime.Envelope) error {
	b.events[user] = append(b.events[user], e)
	return nil
}

func TestGroupMessageUsesTypeAndActiveRecipients(t *testing.T) {
	bus := &eventBus{events: map[string][]realtime.Envelope{}}
	events := &Events{Members: eventMembers{ids: []string{"owner", "member", "member"}, kind: "group"}, Bus: bus}
	err := events.Broadcast(context.Background(), realtime.Event("chat.message.created", "", chat.Message{ID: "message", ConversationID: "group", ClientRequestID: "retry"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(bus.events) != 2 || len(bus.events["member"]) != 1 {
		t.Fatal("duplicate or inactive recipients", bus.events)
	}
	var data struct {
		Type    string       `json:"type"`
		Message chat.Message `json:"message"`
	}
	if err := json.Unmarshal(bus.events["member"][0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Type != "group" || data.Message.ClientRequestID != "retry" || bus.events["member"][0].Type != "message.created" {
		t.Fatal("invalid group event", data)
	}
}

func TestRemovedUserReceivesOnlyExplicitLifecycleEvent(t *testing.T) {
	bus := &eventBus{events: map[string][]realtime.Envelope{}}
	events := &Events{Members: eventMembers{ids: []string{"owner"}, kind: "group"}, Bus: bus}
	data := map[string]string{"conversationId": "group", "type": "group", "userId": "removed"}
	if err := events.PublishConversationTo(context.Background(), "group", "conversation.member.removed", data, "removed"); err != nil {
		t.Fatal(err)
	}
	if len(bus.events["removed"]) != 1 || len(bus.events["owner"]) != 1 {
		t.Fatal("revoke delivery missing")
	}
	if err := events.PublishConversationTo(context.Background(), "group", "message.created", data, "removed"); err == nil {
		t.Fatal("private payload accepted extra recipient")
	}
	if len(bus.events["removed"]) != 1 {
		t.Fatal("removed user received private event")
	}
}

func TestGroupReadTypeAndLookupFailure(t *testing.T) {
	bus := &eventBus{events: map[string][]realtime.Envelope{}}
	events := &Events{Members: eventMembers{kind: "group"}, Bus: bus}
	if err := events.SendToParticipant(context.Background(), "group", "owner", realtime.Event("chat.read.updated", "", chat.ReadState{})); err != nil {
		t.Fatal(err)
	}
	var data struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(bus.events["owner"][0].Data, &data)
	if data.Type != "group" {
		t.Fatal("read event lost type")
	}
	events.Members = eventMembers{err: errors.New("database unavailable")}
	if err := events.Broadcast(context.Background(), realtime.Event("chat.message.created", "", chat.Message{})); err == nil {
		t.Fatal("lookup failure should not publish")
	}
	if len(bus.events["owner"]) != 1 {
		t.Fatal("published on failed lookup")
	}
}

// TestPrivateConversationActionsReachOnlyActor проверяет адресность личных
// настроек: собеседник не получает сведения об очистке, скрытии или mute.
func TestPrivateConversationActionsReachOnlyActor(t *testing.T) {
	bus := &eventBus{events: map[string][]realtime.Envelope{}}
	events := &Events{Members: eventMembers{ids: []string{"actor", "peer"}, kind: "direct"}, Bus: bus}
	id := uuid.NewString()
	for _, kind := range []string{"conversation.preferences.updated", "conversation.history.cleared", "conversation.hidden"} {
		data := map[string]any{"conversationId": id, "type": "direct", "userId": "actor", "historyClearedThrough": int64(10)}
		if err := events.PublishUser(context.Background(), "actor", kind, data); err != nil {
			t.Fatal(err)
		}
	}
	if len(bus.events["actor"]) != 3 || len(bus.events["peer"]) != 0 {
		t.Fatal("personal actions escaped actor channel", bus.events)
	}
	for _, data := range []any{
		map[string]string{"conversationId": id, "type": "direct", "userId": "peer"},
		map[string]string{"conversationId": id, "type": "group", "userId": "actor"},
		map[string]string{"conversationId": "invalid", "type": "direct", "userId": "actor"},
		map[string]string{"conversationId": id, "type": "direct"},
	} {
		if err := events.PublishUser(context.Background(), "actor", "conversation.hidden", data); err == nil {
			t.Fatal("invalid actor-only payload accepted", data)
		}
	}
	if err := events.PublishUser(context.Background(), "actor", "message.created", map[string]string{"conversationId": id, "type": "direct", "userId": "actor"}); err == nil {
		t.Fatal("private event helper accepted a message")
	}
	if len(bus.events["actor"]) != 3 {
		t.Fatal("invalid action published")
	}
}
