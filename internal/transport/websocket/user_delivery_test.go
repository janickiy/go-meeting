package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type deliveryAccount struct {
	allowed bool
	err     error
	checked int
}

func (*deliveryAccount) Account(context.Context, string) error { return nil }
func (a *deliveryAccount) WithConversationDelivery(_ context.Context, _, _ string, write func() error) (bool, error) {
	a.checked++
	if a.err != nil || !a.allowed {
		return false, a.err
	}
	return true, write()
}
func TestUserDeliveryRechecksRevokedMembershipAndKeepsSafeRevoke(t *testing.T) {
	ctx := context.Background()
	a := &deliveryAccount{}
	h := &UserHandler{accounts: a}
	conversation := uuid.NewString()
	writes := 0
	write := func([]byte) error { writes++; return nil }
	data, _ := json.Marshal(realtime.Event("message.created", "", map[string]any{"conversationId": conversation, "type": "group", "message": map[string]string{"text": "private"}}))
	if err := h.deliver(ctx, "removed", data, write); err != nil {
		t.Fatal(err)
	}
	if writes != 0 || a.checked != 1 {
		t.Fatal("queued private message escaped revoked membership")
	}
	a.allowed = true
	if err := h.deliver(ctx, "owner", data, write); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Fatal("active user did not receive message")
	}
	a.allowed = false
	revoke, _ := json.Marshal(realtime.Event("conversation.member.removed", "", map[string]string{"conversationId": conversation, "type": "group", "userId": "removed"}))
	if err := h.deliver(ctx, "removed", revoke, write); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatal("safe revoke was denied")
	}
	unsafe, _ := json.Marshal(realtime.Event("conversation.member.removed", "", map[string]string{"conversationId": conversation, "type": "group", "userId": "removed", "name": "private metadata"}))
	if err := h.deliver(ctx, "removed", unsafe, write); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatal("revoke bypass leaked metadata")
	}
	a.err = errors.New("database unavailable")
	if err := h.deliver(ctx, "owner", data, write); err == nil {
		t.Fatal("availability failure must close socket")
	}
	if writes != 2 {
		t.Fatal("membership failure leaked payload")
	}
}

// messageDeliveryAccount имитирует актуальную проверку БД, а не флаг из очереди.
type messageDeliveryAccount struct {
	deliveryAccount
	enabled bool
	cutoff  int64
	message string
}

func (a *messageDeliveryAccount) WithConversationMessageDelivery(_ context.Context, _, _, message string, write func(bool, int64) error) (bool, error) {
	a.checked++
	a.message = message
	if a.err != nil || !a.allowed {
		return false, a.err
	}
	return true, write(a.enabled, a.cutoff)
}

// TestUserMessageDeliveryUsesCurrentMuteAndHistory проверяет, что mute не
// скрывает новое сообщение, а очередь после очистки не возвращает старую цитату.
func TestUserMessageDeliveryUsesCurrentMuteAndHistory(t *testing.T) {
	ctx := context.Background()
	a := &messageDeliveryAccount{deliveryAccount: deliveryAccount{allowed: true}, cutoff: 10}
	h := &UserHandler{accounts: a}
	id, messageID, replyID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	message := chat.Message{ID: messageID, ConversationID: id, Sequence: 20, Text: "new", ReplyTo: &replyID, ReplyPreview: &chat.ReplyPreview{ID: replyID, Sequence: 8, Text: "cleared"}}
	event := realtime.Event("message.created", "", map[string]any{"conversationId": id, "type": "direct", "message": message, "notificationsEnabled": true})
	data, _ := json.Marshal(event)
	var received struct {
		NotificationsEnabled  bool         `json:"notificationsEnabled"`
		HistoryClearedThrough int64        `json:"historyClearedThrough"`
		Message               chat.Message `json:"message"`
	}
	writes := 0
	write := func(raw []byte) error {
		writes++
		var envelope realtime.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		return json.Unmarshal(envelope.Data, &received)
	}
	if err := h.deliver(ctx, "actor", data, write); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || received.NotificationsEnabled || received.HistoryClearedThrough != 10 || received.Message.Text != "new" || a.message != messageID {
		t.Fatal("mute suppressed message or stale flag survived", writes, received)
	}
	if received.Message.ReplyPreview != nil || received.Message.ReplyTo != nil {
		t.Fatal("cleared reply escaped socket guard", received.Message)
	}
	message.ReplyPreview.Sequence = 12
	a.enabled = true
	event = realtime.Event("message.created", "", map[string]any{"conversationId": id, "type": "direct", "message": message, "notificationsEnabled": false})
	data, _ = json.Marshal(event)
	if err := h.deliver(ctx, "actor", data, write); err != nil {
		t.Fatal(err)
	}
	if writes != 2 || !received.NotificationsEnabled || received.Message.ReplyPreview == nil {
		t.Fatal("visible new quote or current unmute missing", received)
	}
	a.allowed = false
	if err := h.deliver(ctx, "actor", data, write); err != nil || writes != 2 {
		t.Fatal("cleared/hidden queued message escaped", err, writes)
	}
	a.err = errors.New("database unavailable")
	if err := h.deliver(ctx, "actor", data, write); err == nil || writes != 2 {
		t.Fatal("message guard did not fail closed", err, writes)
	}
}

// TestPrivateActionSocketDeliveryRequiresActor проверяет, что личное событие
// может очистить скрытый чат в своих вкладках, но не раскрывается собеседнику.
func TestPrivateActionSocketDeliveryRequiresActor(t *testing.T) {
	a := &deliveryAccount{allowed: true}
	h := &UserHandler{accounts: a}
	data, _ := json.Marshal(realtime.Event("conversation.hidden", "", map[string]string{"conversationId": uuid.NewString(), "type": "direct", "userId": "actor"}))
	writes := 0
	write := func([]byte) error { writes++; return nil }
	if err := h.deliver(context.Background(), "peer", data, write); err != nil || writes != 0 || a.checked != 0 {
		t.Fatal("peer received a private action", err, writes)
	}
	if err := h.deliver(context.Background(), "actor", data, write); err != nil || writes != 1 || a.checked != 1 {
		t.Fatal("actor lifecycle was denied", err, writes)
	}
	a.allowed = false
	if err := h.deliver(context.Background(), "actor", data, write); err != nil || writes != 1 {
		t.Fatal("private action bypassed regular membership", err, writes)
	}
}

// actionDeliveryAccount подставляет текущую проекцию вместо данных старого события.
type actionDeliveryAccount struct {
	deliveryAccount
	item *personal.Conversation
}

func (a *actionDeliveryAccount) WithDirectActionDelivery(_ context.Context, _, _, _ string, write func(*personal.Conversation) error) (bool, error) {
	a.checked++
	if a.err != nil || !a.allowed {
		return false, a.err
	}
	return true, write(a.item)
}

// TestDirectActionDeliveryReprojectsQueuedMetadata проверяет замену старых
// настроек и превью текущим состоянием, а также минимальную нагрузку скрытия.
func TestDirectActionDeliveryReprojectsQueuedMetadata(t *testing.T) {
	id := uuid.NewString()
	a := &actionDeliveryAccount{deliveryAccount: deliveryAccount{allowed: true}, item: &personal.Conversation{ID: id, Type: "direct", NotificationsEnabled: true, HistoryClearedThrough: 10, Preview: "fresh message"}}
	h := &UserHandler{accounts: a}
	event := realtime.Event("conversation.preferences.updated", "", map[string]any{"conversationId": id, "type": "direct", "userId": "actor", "item": personal.Conversation{ID: id, NotificationsEnabled: false, Preview: "stale"}, "privateExtra": "discard"})
	data, _ := json.Marshal(event)
	writes := 0
	var fields map[string]json.RawMessage
	write := func(raw []byte) error {
		writes++
		var envelope realtime.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		fields = nil
		return json.Unmarshal(envelope.Data, &fields)
	}
	if err := h.deliver(context.Background(), "actor", data, write); err != nil {
		t.Fatal(err)
	}
	var item personal.Conversation
	if err := json.Unmarshal(fields["item"], &item); err != nil || !item.NotificationsEnabled || item.Preview != "fresh message" || item.HistoryClearedThrough != 10 {
		t.Fatal("queued action was not reprojected", item, err)
	}
	if fields["privateExtra"] != nil {
		t.Fatal("non-contract metadata escaped")
	}
	a.item = &personal.Conversation{ID: id, Type: "direct", HistoryClearedThrough: 12}
	event.Type = "conversation.hidden"
	data, _ = json.Marshal(event)
	if err := h.deliver(context.Background(), "actor", data, write); err != nil || writes != 2 || len(fields) != 5 {
		t.Fatal("hidden event was not minimal", writes, fields, err)
	}
	var cutoff int64
	var hidden bool
	if err := json.Unmarshal(fields["historyClearedThrough"], &cutoff); err != nil || cutoff != 12 {
		t.Fatal("hidden event lost authoritative cutoff", cutoff, err)
	}
	if err := json.Unmarshal(fields["hidden"], &hidden); err != nil || !hidden || fields["item"] != nil {
		t.Fatal("hidden event leaked a projection or missing hidden flag", fields, err)
	}
	a.allowed = false
	if err := h.deliver(context.Background(), "actor", data, write); err != nil || writes != 2 {
		t.Fatal("stale hidden action escaped", writes, err)
	}
}
