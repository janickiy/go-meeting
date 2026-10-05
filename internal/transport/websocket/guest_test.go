package websocket

import (
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuestWebsocketScope(t *testing.T) {
	tokens, _ := security.NewTokenService(strings.Repeat("guest-ws-secret-", 3))
	id, room := uuid.NewString(), uuid.NewString()
	token, _ := tokens.IssueGuest(id, room)
	handler := &Handler{verifier: tokens}
	for _, path := range []string{"/api/v1/conferences/" + room + "/ws", "/api/v1/conferences/" + room + "/ws-ticket", "/api/v1/webrtc/config"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		identity, err := handler.identity(req)
		if err != nil || identity.UserID != id {
			t.Fatalf("allowed path %s rejected: %v", path, err)
		}
	}
	req := httptest.NewRequest("GET", "/api/v1/conferences/"+uuid.NewString()+"/ws", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, err := handler.identity(req); err == nil {
		t.Fatal("guest allowed into unrelated websocket")
	}
}
