// Пакет platform собирает доступные только для чтения возможности продукта и операционные данные.
package platform

import (
	"context"
	"github.com/janickiy/go-recorder/internal/buildinfo"
	"runtime/debug"

	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/platform"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// ReleaseVersion задаётся при сборке релиза через -ldflags и не содержит секретов.
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

// Service исключает учётные данные и внутренние адреса из публичных ответов.
type Service struct {
	Repo               Repository
	Vector             VectorAvailability
	MediaWorker        ReadyProbe
	APIReady           func() bool
	DependencyStatuses func() map[string]bool
	StageSeven         config.StageSevenConfig
	StageEight         config.StageEightConfig
}

// Capabilities возвращает фактически включённые оператором функции; векторный поиск также требует pgvector.
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

// Summary дополняет снимок БД ограниченным набором признаков работоспособности.
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

// BuildVersion возвращает только версию или короткий хеш коммита, без путей и флагов сборки.
func BuildVersion() string {
	if safeBuildVersion(ReleaseVersion) {
		return ReleaseVersion
	}
	if buildinfo.ValidVersion(buildinfo.Version) {
		return buildinfo.Version
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

// safeBuildVersion ограничивает публичные сведения о сборке одним коротким непрозрачным идентификатором.
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
