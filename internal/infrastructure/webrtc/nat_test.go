package webrtc

import (
	"context"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"
)

// TestNATAddressRewrite проверяет публикацию одного и нескольких внешних
// host-адресов после перехода на актуальный API Pion. t управляет изолированным
// manager и собирает SDP без подключения к внешней сети. IPv4 host-кандидаты
// заменяются; IPv6 без заданного IPv6 NAT остаётся неизменным, как в прежнем API.
func TestNATAddressRewrite(t *testing.T) {
	for _, external := range [][]string{{"203.0.113.10"}, {"203.0.113.10", "203.0.113.11"}, {"203.0.113.10/192.0.2.10"}} {
		t.Run(strings.Join(external, ","), func(t *testing.T) {
			manager, err := NewManager(Options{StoragePath: t.TempDir(), NATIPs: external, Logger: log.New(io.Discard, "", 0)})
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
				public, _, _ := strings.Cut(address, "/")
				seen[public] = false
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
				parsed := net.ParseIP(address)
				if parsed == nil {
					t.Fatalf("invalid host IP %q", address)
				}
				if parsed.To4() == nil {
					continue
				}
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
