package platform

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/config"
)

func TestBuildVersionAllowsOnlySafeReleaseMetadata(t *testing.T) {
	previous := ReleaseVersion
	t.Cleanup(func() { ReleaseVersion = previous })
	for _, version := range []string{"9f27c2e4a14b", "v1.2.3+stage9"} {
		ReleaseVersion = version
		if got := BuildVersion(); got != version {
			t.Fatalf("version %q became %q", version, got)
		}
	}
	for _, value := range []string{"", "../private", "v1;token", "a b", "v1\nsecret", string(make([]byte, 65))} {
		if safeBuildVersion(value) {
			t.Fatalf("unsafe build version was accepted: %q", value)
		}
	}
	ReleaseVersion = "v1\nsecret"
	if got := BuildVersion(); got == ReleaseVersion || got == "" {
		t.Fatal("unsafe metadata escaped into API response")
	}
}

type vectorAvailabilityStub struct {
	available bool
	err       error
}

func (s vectorAvailabilityStub) Available(context.Context) (bool, error) { return s.available, s.err }

// TestCapabilitiesReflectEffectiveAvailability guards against advertising unavailable paid features.
func TestCapabilitiesReflectEffectiveAvailability(t *testing.T) {
	service := Service{StageSeven: config.StageSevenConfig{STTEnabled: true, AIEnabled: false},
		StageEight: config.StageEightConfig{LiveEnabled: true, EmbeddingsEnabled: true, AnalyticsEnabled: true}}
	for _, example := range []struct {
		name     string
		vector   vectorAvailabilityStub
		semantic bool
	}{
		{"available", vectorAvailabilityStub{available: true}, true},
		{"extension absent", vectorAvailabilityStub{}, false},
		{"database error", vectorAvailabilityStub{err: errors.New("private address")}, false},
	} {
		t.Run(example.name, func(t *testing.T) {
			service.Vector = example.vector
			value := service.Capabilities(context.Background())
			if value.SemanticSearch != example.semantic || !value.LiveCaptions || !value.Transcription || value.AISummary || !value.MeetingAnalytics || len(value.RecordingModes) != 4 {
				t.Fatalf("incorrect effective capabilities: %+v", value)
			}
		})
	}
}
