package records

import "strings"

const (
	DefaultVideoQuality       = "720p"
	DefaultVideoWidth         = 1280
	DefaultVideoHeight        = 720
	DefaultVideoFrameRate     = 30
	DefaultVideoMaxBitrateBPS = 6_000_000
)

// VideoSettings задаёт разрешение, частоту кадров и битрейт браузерного видеозахвата.
// @params
//   - Quality: поддерживаемый профиль качества видео.
//   - Width: ширина видеокадра или области в пикселях.
//   - Height: высота видеокадра или области в пикселях.
//   - FrameRate: значение FrameRate типа int, используемое согласно назначению этой операции.
//   - MaxBitrateBPS: значение MaxBitrateBPS типа int, используемое согласно назначению этой операции.
type VideoSettings struct {
	Quality       string `json:"quality"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	FrameRate     int    `json:"frameRate"`
	MaxBitrateBPS int    `json:"maxBitrateBps"`
}

// NormalizeStartRequest заполняет безопасные значения по умолчанию для старта записи.
// @args
// - request: исходный DTO старта записи.
// @return DTO с дефолтным качеством видео.
func NormalizeStartRequest(request StartRequest) StartRequest {
	request.Quality = strings.TrimSpace(request.Quality)
	if request.Quality == "" {
		request.Quality = DefaultVideoQuality
	}

	return request
}

// VideoSettingsForQuality возвращает профиль видео по строковому quality.
// @args
// - quality: качество из API, например 720p.
// @return настройки видео; для неизвестного качества возвращается 720p.
func VideoSettingsForQuality(quality string) VideoSettings {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "1080p":
		return VideoSettings{
			Quality:       "1080p",
			Width:         1920,
			Height:        1080,
			FrameRate:     30,
			MaxBitrateBPS: 10_000_000,
		}
	case "480p":
		return VideoSettings{
			Quality:       "480p",
			Width:         854,
			Height:        480,
			FrameRate:     30,
			MaxBitrateBPS: 1_500_000,
		}
	case "360p":
		return VideoSettings{
			Quality:       "360p",
			Width:         640,
			Height:        360,
			FrameRate:     30,
			MaxBitrateBPS: 800_000,
		}
	default:
		return VideoSettings{
			Quality:       DefaultVideoQuality,
			Width:         DefaultVideoWidth,
			Height:        DefaultVideoHeight,
			FrameRate:     DefaultVideoFrameRate,
			MaxBitrateBPS: DefaultVideoMaxBitrateBPS,
		}
	}
}

// IsSupportedVideoQuality проверяет, поддерживается ли quality.
// @args
// - quality: качество из API.
// @return true, если quality поддерживается серверной частью.
func IsSupportedVideoQuality(quality string) bool {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "360p", "480p", "720p", "1080p":
		return true
	default:
		return false
	}
}
