package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// OutputValidation собирает результаты проверки итогового медиафайла через ffprobe.
//   - Duration: плановая длительность или интервал в единицах, заданных типом.
//   - VideoDuration: значение VideoDuration типа float64, используемое согласно назначению этой операции.
//   - AudioDuration: значение AudioDuration типа float64, используемое согласно назначению этой операции.
//   - VideoCodec: значение VideoCodec типа string, используемое согласно назначению этой операции.
//   - AudioCodec: значение AudioCodec типа string, используемое согласно назначению этой операции.
//   - Width: ширина видеокадра или области в пикселях.
//   - Height: высота видеокадра или области в пикселях.
//   - Size: размер содержимого в байтах.
type OutputValidation struct {
	Duration      float64 `json:"duration"`
	VideoDuration float64 `json:"videoDuration"`
	AudioDuration float64 `json:"audioDuration"`
	VideoCodec    string  `json:"videoCodec"`
	AudioCodec    string  `json:"audioCodec"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	Size          int64   `json:"size"`
}

// concatComposite объединяет готовые фрагменты общей записи в итоговый файл.
// Внешняя команда или запрос использует контекст операции.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - listPath (string): значение listPath типа string, используемое согласно назначению этой операции.
//   - finalPath (string): значение finalPath типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) concatComposite(ctx context.Context, listPath, finalPath string) error {
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	output, err := runBounded(runCtx, p.ffmpegPath, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-movflags", "+faststart", finalPath)
	if err != nil {
		return fmt.Errorf("concat composite: %w: %s", err, output)
	}
	return nil
}

// ValidateOutput проверяет итоговый медиафайл через ffprobe: дорожки, кодеки, длительность и размеры.
// Внешняя команда или запрос использует контекст операции.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - path (string): путь к локальному файлу или каталогу операции.
//   - requireAudio (bool): логический признак requireAudio, управляющий соответствующей веткой обработки.
//
// @return:
//   - результат 1 (OutputValidation): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) ValidateOutput(ctx context.Context, path string, requireAudio bool) (OutputValidation, error) {
	result := OutputValidation{}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	result.Size = info.Size()
	if info.Size() < 1024 {
		return result, fmt.Errorf("recording output is too small")
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	data, err := runBounded(runCtx, p.ffprobePath, "-v", "error", "-show_entries", "format=duration,format_name:stream=codec_type,codec_name,duration,width,height", "-of", "json", path)
	if err != nil {
		return result, fmt.Errorf("probe recording output: %w", err)
	}
	var probe struct {
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
		Streams []struct {
			Type     string `json:"codec_type"`
			Codec    string `json:"codec_name"`
			Duration string `json:"duration"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return result, err
	}
	result.Duration, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	if !strings.Contains(probe.Format.Name, "mp4") || result.Duration <= 0 || math.IsInf(result.Duration, 0) || math.IsNaN(result.Duration) {
		return result, fmt.Errorf("invalid MP4 duration or container")
	}
	for _, stream := range probe.Streams {
		duration, _ := strconv.ParseFloat(stream.Duration, 64)
		switch stream.Type {
		case "video":
			result.VideoCodec = stream.Codec
			result.VideoDuration = duration
			result.Width = stream.Width
			result.Height = stream.Height
		case "audio":
			result.AudioCodec = stream.Codec
			result.AudioDuration = duration
		}
	}
	if result.VideoCodec != "h264" || result.Width <= 0 || result.Height <= 0 || result.VideoDuration <= 0 {
		return result, fmt.Errorf("composite output lacks valid H.264 video")
	}
	if requireAudio && (result.AudioCodec != "aac" || result.AudioDuration <= 0) {
		return result, fmt.Errorf("composite output lacks valid AAC audio")
	}
	if result.AudioDuration > 0 && math.Abs(result.VideoDuration-result.AudioDuration) > 0.35 {
		return result, fmt.Errorf("composite A/V duration mismatch: video %.3fs audio %.3fs", result.VideoDuration, result.AudioDuration)
	}
	return result, nil
}
