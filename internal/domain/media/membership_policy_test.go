package media

import (
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"testing"
)

func TestMembershipPolicyProjection(t *testing.T) {
	p := conferences.Participant{Status: conferences.Joined, AdmissionState: conferences.AdmissionAdmitted, MediaPolicyVersion: 42, MicrophoneBlocked: true, ScreenBlocked: true}
	got := PolicyForParticipant(p)
	if got.Version != 42 || got.Kicked || !got.MicrophoneBlocked || got.CameraBlocked || !got.ScreenBlocked {
		t.Fatalf("projection lost policy: %+v", got)
	}
	for _, status := range []conferences.ParticipantStatus{conferences.Left, conferences.Kicked, conferences.Waiting, conferences.Rejected} {
		p.Status = status
		if got := PolicyForParticipant(p); !got.Kicked || got.Allows(SourceCamera) {
			t.Fatalf("inactive participant still publishes: %+v", got)
		}
	}
	p.Status = conferences.Joined
	p.AdmissionState = conferences.AdmissionWaiting
	if !PolicyForParticipant(p).Kicked {
		t.Fatal("waiting admission may publish")
	}
}
