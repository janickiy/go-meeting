// Package platform assembles read-only product capabilities and operations data.
package platform

import (
	"context"
	"runtime/debug"

	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/platform"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// ReleaseVersion is set by the release build through -ldflags; it contains no secrets.
var ReleaseVersion string

type Repository interface {
	Summary(context.Context) (domain.Summary, error)
}

type VectorAvailability interface {
	Available(context.Context) (bool, error)
}

type ReadyProbe interface {
	Ready(context.Context) bool
}

// Service keeps credentials and internal addresses out of public responses.
type Service struct {
	Repo               Repository
	Vector             VectorAvailability
	MediaWorker        ReadyProbe
	APIReady           func() bool
	DependencyStatuses func() map[string]bool
	StageSeven         config.StageSevenConfig
	StageEight         config.StageEightConfig
}

// Capabilities returns effective operator-enabled flags; vector search also needs pgvector.
func (s *Service) Capabilities(ctx context.Context) domain.Capabilities {
	semantic := false
	if s.StageEight.EmbeddingsEnabled && s.Vector != nil {
		available, err := s.Vector.Available(ctx)
		semantic = err == nil && available
	}
	return domain.Capabilities{
		LiveCaptions:     s.StageEight.LiveEnabled,
		Transcription:    s.StageSeven.STTEnabled,
		AISummary:        s.StageSeven.AIEnabled,
		SemanticSearch:   semantic,
		MeetingAnalytics: s.StageEight.AnalyticsEnabled,
		RecordingModes:   []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus},
	}
}

// Summary enriches the database snapshot with bounded health signals.
func (s *Service) Summary(ctx context.Context) (domain.Summary, error) {
	value, err := s.Repo.Summary(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	if s.APIReady != nil {
		value.APIReady = s.APIReady()
	}
	if s.DependencyStatuses != nil {
		value.Dependencies = s.DependencyStatuses()
	}
	if value.Dependencies == nil {
		value.Dependencies = map[string]bool{}
	}
	if s.MediaWorker != nil {
		value.MediaWorkerReady = s.MediaWorker.Ready(ctx)
	}
	return value, nil
}

// BuildVersion returns only a version or short commit hash, never build paths or flags.
func BuildVersion() string {
	if safeBuildVersion(ReleaseVersion) {
		return ReleaseVersion
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if safeBuildVersion(info.Main.Version) {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && len(setting.Value) >= 12 && safeBuildVersion(setting.Value[:12]) {
			return setting.Value[:12]
		}
	}
	return "dev"
}

// safeBuildVersion limits public build metadata to one short opaque token.
func safeBuildVersion(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for index, char := range value {
		letter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
		digit := char >= '0' && char <= '9'
		if letter || digit || index > 0 && (char == '.' || char == '_' || char == '-' || char == '+') {
			continue
		}
		return false
	}
	return true
}
