package sfu

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The browser can suspend incoming video with an SDP preference while keeping its own camera
// published. The SFU must stop forwarding those subscriptions and restore them.
func TestReceiveVideoPreferencePreservesAudio(t *testing.T) {
	h := harness(t, 3)
	a := h.join(t, uuid.NewString(), "")
	b := h.join(t, a.binding.ConferenceID, "")
	defer a.close()
	defer b.close()
	server, _ := h.manager.get(b.id)
	eventually(t, h, "both subscriptions ready", func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		if len(server.subscriptions) != 2 {
			return false
		}
		for _, sub := range server.subscriptions {
			if !sub.ready.Load() {
				return false
			}
		}
		return true
	})
	b.neg.Lock()
	offer := b.pc.LocalDescription().SDP
	offer = strings.Replace(offer, "m=", "a=x-meet-receive-video:0\r\nm=", 1)
	negotiation := uuid.NewString()
	_, err := h.manager.Offer(context.Background(), b.id, negotiation, offer)
	if err == nil {
		err = h.manager.Ready(context.Background(), b.id, negotiation)
	}
	if err != nil {
		b.neg.Unlock()
		t.Fatal(err)
	}
	server.mu.Lock()
	audio, video := 0, 0
	for _, sub := range server.subscriptions {
		if sub.ready.Load() {
			if sub.source.metadata.Kind == "audio" {
				audio++
			} else {
				video++
			}
		}
	}
	server.mu.Unlock()
	b.neg.Unlock()
	if audio != 1 || video != 0 {
		t.Fatalf("expected audio only, got audio=%d video=%d", audio, video)
	}
	if err := b.negotiate(); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "video reception restored", func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		if len(server.subscriptions) != 2 {
			return false
		}
		for _, sub := range server.subscriptions {
			if !sub.ready.Load() {
				return false
			}
		}
		return true
	})
}

func TestReceiveVideoPreferenceValidation(t *testing.T) {
	for _, tc := range []struct {
		attrs          string
		enabled, valid bool
	}{
		{"", true, true},
		{"a=x-meet-receive-video:0\r\n", false, true},
		{"a=x-meet-receive-video:1\r\n", true, true},
		{"a=x-meet-receive-video:no\r\n", false, false},
		{"a=x-meet-receive-video:0\r\na=x-meet-receive-video:1\r\n", false, false},
	} {
		raw := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" + tc.attrs + "m=audio 9 UDP/TLS/RTP/SAVPF 111\r\n"
		enabled, err := receiveVideoPreference(raw)
		if (err == nil) != tc.valid || (err == nil && enabled != tc.enabled) {
			t.Errorf("%q: enabled=%t err=%v", tc.attrs, enabled, err)
		}
	}
}
