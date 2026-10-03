package mediaworker

import (
	"github.com/google/uuid"
	"testing"
)

// TestMediaDrainPreservesExistingSignaling запрещает новые подключения при сохранении SDP и выхода.
// @args t — контекст проверки действующей медиа-комнаты.
func TestMediaDrainPreservesExistingSignaling(t *testing.T) {
	f := newFixture(t)
	peer := f.join(t)
	f.h.BeginDrain()
	if f.h.Ready() || f.h.ActiveRooms() != 1 {
		t.Fatal("drain did not preserve active room")
	}
	if status, _ := f.request("join", f.cmd, f.cfg.InternalSecret); status != 503 {
		t.Fatalf("new join admitted: %d", status)
	}
	cmd := f.cmd
	cmd.MediaPeerID = peer
	cmd.NegotiationID = uuid.NewString()
	cmd.SDP = "offer"
	if status, _ := f.request("offer", cmd, f.cfg.InternalSecret); status != 200 {
		t.Fatalf("existing signaling interrupted: %d", status)
	}
	if status, _ := f.request("leave", cmd, f.cfg.InternalSecret); status != 200 {
		t.Fatalf("leave interrupted: %d", status)
	}
	if f.h.ActiveRooms() != 0 {
		t.Fatal("drained room retained active peer")
	}
}
