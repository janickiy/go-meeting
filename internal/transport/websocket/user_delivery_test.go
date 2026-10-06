package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
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
	write := func() error { writes++; return nil }
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
