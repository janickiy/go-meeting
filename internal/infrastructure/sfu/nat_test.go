package sfu

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
)

// TestNATAddressRewrite проверяет внешние host-кандидаты SFU для одного и
// нескольких NAT-адресов. t собирает реальный SDP через manager.api без STUN
// и сетевого peer; проверка исключает публикацию локальных адресов или смену
// типа кандидата при замене устаревшего API.
func TestNATAddressRewrite(t *testing.T) {
	for _, external := range [][]string{{"203.0.113.10"}, {"203.0.113.10", "203.0.113.11"}} {
		t.Run(strings.Join(external, ","), func(t *testing.T) {
			manager, err := NewManager(Options{NATIPs: external, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := manager.Shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			pc, err := manager.api.NewPeerConnection(pion.Configuration{})
			if err != nil {
				t.Fatal(err)
			}
			defer pc.Close()
			if _, err := pc.AddTransceiverFromKind(pion.RTPCodecTypeAudio); err != nil {
				t.Fatal(err)
			}
			offer, err := pc.CreateOffer(nil)
			if err != nil {
				t.Fatal(err)
			}
			gathered := pion.GatheringCompletePromise(pc)
			if err := pc.SetLocalDescription(offer); err != nil {
				t.Fatal(err)
			}
			select {
			case <-gathered:
			case <-time.After(3 * time.Second):
				t.Fatal("ICE gathering timed out")
			}
			seen := map[string]bool{}
			for _, address := range external {
				seen[address] = false
			}
			for _, line := range strings.Split(pc.LocalDescription().SDP, "\n") {
				if !strings.HasPrefix(line, "a=candidate:") {
					continue
				}
				candidate := strings.Fields(line)
				if len(candidate) < 8 || candidate[6] != "typ" || candidate[7] != "host" {
					t.Fatalf("unexpected ICE candidate %q", line)
				}
				address := candidate[4]
				if _, expected := seen[address]; !expected {
					t.Fatalf("private/unexpected host address %q", address)
				}
				seen[address] = true
			}
			for address, found := range seen {
				if !found {
					t.Fatalf("external host address %q missing", address)
				}
			}
		})
	}
}
