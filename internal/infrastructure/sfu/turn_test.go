package sfu

import (
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	pion "github.com/pion/webrtc/v4"
	"os"
	"testing"
	"time"
)

// TestStageSixForcedTURN проверяет реальный двусторонний RTP через Coturn с
// временными REST-credentials по UDP и TCP-control. t фиксирует выбранную relay
// пару, доставку аудио/видео, повторное подключение и очистку SFU.
func TestStageSixForcedTURN(t *testing.T) {
	host, secret := os.Getenv("RECORDER_TEST_TURN_HOST"), os.Getenv("RECORDER_TEST_TURN_SECRET")
	if host == "" || secret == "" {
		t.Skip("set isolated TURN host and shared secret")
	}
	for _, transport := range []string{"udp", "tcp"} {
		t.Run(transport, func(t *testing.T) {
			h := harness(t, 3, Options{MaxRooms: 2, NegotiationTimeout: 15 * time.Second})
			turn := config.TURNConfig{URLs: []string{"turn:" + host + "?transport=" + transport}, Secret: secret, TTL: 5 * time.Minute, ForceRelay: true}
			ice := turn.ClientICE(realtime.ICEConfig{}, time.Now())
			server := ice.ICEServers[0]
			h.clientConfig = pion.Configuration{ICETransportPolicy: pion.ICETransportPolicyRelay, ICEServers: []pion.ICEServer{{URLs: server.URLs, Username: server.Username, Credential: server.Credential}}}
			room := uuid.NewString()
			a, b := h.join(t, room, ""), h.join(t, room, "")
			assertTURNMedia(t, h, a, b)
			b.close()
			b = h.join(t, room, "")
			assertTURNMedia(t, h, a, b)
			a.close()
			b.close()
			eventually(t, h, "TURN cleanup", func() bool { return h.manager.Snapshot().Rooms == 0 })
			t.Logf("TURN transport=%s relay-only RTP+reconnect PASS", transport)
		})
	}
}

// assertTURNMedia требует обе дорожки у peers и выбранный local relay candidate.
// t сообщает ошибку, h обеспечивает bounded ожидание, peers — живые Pion-клиенты.
func assertTURNMedia(t *testing.T, h *pionHarness, peers ...*testPeer) {
	t.Helper()
	eventually(t, h, "relay RTP", func() bool {
		for _, p := range peers {
			p.mu.Lock()
			count := len(p.received)
			p.mu.Unlock()
			if count < 2 {
				return false
			}
		}
		return true
	})
	for _, p := range peers {
		pair, err := p.audioSender.Transport().ICETransport().GetSelectedCandidatePair()
		if err != nil || pair == nil || pair.Local.Typ != pion.ICECandidateTypeRelay {
			t.Fatalf("expected selected relay pair, got %v (%v)", pair, err)
		}
	}
	eventually(t, h, "relay classified", func() bool { return h.manager.Snapshot().RelayConnections >= uint64(len(peers)) })
}

// TestStageSixTURNUnavailable ожидает контролируемую внешнюю остановку тестового
// Coturn после маркера INJECT_TURN_OUTAGE_NOW. t подтверждает failed/cleanup, не
// считает пропажу RTP успешным сценарием. Никогда не останавливает чужие сервисы.
func TestStageSixTURNUnavailable(t *testing.T) {
	if os.Getenv("RECORDER_TEST_TURN_OUTAGE") != "true" {
		t.Skip("manual TURN outage injection")
	}
	host, secret := os.Getenv("RECORDER_TEST_TURN_HOST"), os.Getenv("RECORDER_TEST_TURN_SECRET")
	if host == "" || secret == "" {
		t.Fatal("isolated TURN configuration required")
	}
	h := harness(t, 3, Options{MaxRooms: 1, ICEDisconnectedTimeout: time.Second, ICEFailedTimeout: 3 * time.Second, ICEKeepaliveInterval: 500 * time.Millisecond, NegotiationTimeout: 15 * time.Second})
	cfg := config.TURNConfig{URLs: []string{"turn:" + host + "?transport=tcp"}, Secret: secret, TTL: 5 * time.Minute}
	server := cfg.ClientICE(realtime.ICEConfig{}, time.Now()).ICEServers[0]
	h.clientConfig = pion.Configuration{ICETransportPolicy: pion.ICETransportPolicyRelay, ICEServers: []pion.ICEServer{{URLs: server.URLs, Username: server.Username, Credential: server.Credential}}}
	room := uuid.NewString()
	a, b := h.join(t, room, ""), h.join(t, room, "")
	assertTURNMedia(t, h, a, b)
	t.Log("INJECT_TURN_OUTAGE_NOW")
	deadline := time.Now().Add(30 * time.Second)
	for {
		s := h.manager.Snapshot()
		if s.Failures >= 2 && s.Peers == 0 && s.Rooms == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("TURN outage did not fail transports: %+v", s)
		}
		time.Sleep(50 * time.Millisecond)
	}
	a.close()
	b.close()
	t.Log("TURN outage: both media transports failed and resources released")
}
