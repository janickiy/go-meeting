package media

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	domain "github.com/janickiy/go-recorder/internal/domain/media"
)

type testPolicyProvider struct {
	policy domain.ParticipantPolicy
	calls  int
}

func (p *testPolicyProvider) MediaPolicy(context.Context, string, string) (domain.ParticipantPolicy, error) {
	p.calls++
	return p.policy, nil
}

func TestControllerRefreshesPolicyAndCarriesTypedSources(t *testing.T) {
	f := newControllerFixture()
	provider := &testPolicyProvider{policy: domain.ParticipantPolicy{Version: 1}}
	f.controller.SetPolicyProvider(provider)
	f.join(t)
	if f.transport.commands[0].Policy == nil || f.transport.commands[0].Policy.Version != 1 {
		t.Fatal("join lost authoritative policy")
	}
	provider.policy = domain.ParticipantPolicy{Version: 2, MicrophoneBlocked: true}
	signal := domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString(), SDP: "offer", Publications: []domain.Publication{{MID: "1", Source: domain.SourceCamera, TrackID: "new-camera"}, {MID: "3", Source: domain.SourceVideoScreen, TrackID: "new-display"}}}
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.offer", signal)); err != nil {
		t.Fatal(err)
	}
	last := f.transport.commands[len(f.transport.commands)-1]
	if provider.calls != 2 || last.Policy.Version != 2 || len(last.Publications) != 2 || last.Publications[1].TrackID != "new-display" {
		t.Fatal("offer did not refresh policy/source metadata")
	}
	if err := f.controller.SetParticipantPolicy(context.Background(), f.session.ConferenceID, f.session.ParticipantID, provider.policy); err != nil {
		t.Fatal(err)
	}
	if f.transport.actions[len(f.transport.actions)-1] != "policy" {
		t.Fatal("policy not pushed")
	}
	signal.Publications = append(signal.Publications, domain.Publication{MID: "4", Source: domain.SourceVideoScreen})
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.offer", signal)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("duplicate semantic source accepted", err)
	}
}
