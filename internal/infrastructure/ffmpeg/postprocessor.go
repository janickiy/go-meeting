package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PostProcessor объединяет сегменты, проверяет выход и создаёт итоговые артефакты записи.
//   - ffmpegPath: значение ffmpegPath типа string, используемое согласно назначению этой операции.
//   - ffprobePath: значение ffprobePath типа string, используемое согласно назначению этой операции.
type PostProcessor struct {
	ffmpegPath  string
	ffprobePath string
}

// Result передаёт результат операции и связанные метаданные компонента.
//   - FinalPath: значение FinalPath типа string, используемое согласно назначению этой операции.
//   - PreviewPath: значение PreviewPath типа string, используемое согласно назначению этой операции.
//   - Segments: доступные сегменты записи для итоговой сборки.
//   - FinalSizeBytes: значение FinalSizeBytes типа int64, используемое согласно назначению этой операции.
//   - PreviewSizeBytes: значение PreviewSizeBytes типа int64, используемое согласно назначению этой операции.
//   - DurationSec: длительность в секундах.
//   - FinalChecksum: значение FinalChecksum типа string, используемое согласно назначению этой операции.
//   - PreviewChecksum: значение PreviewChecksum типа string, используемое согласно назначению этой операции.
type Result struct {
	FinalPath        string
	PreviewPath      string
	Segments         []Segment
	FinalSizeBytes   int64
	PreviewSizeBytes int64
	DurationSec      int
	FinalChecksum    string
	PreviewChecksum  string
}

// Segment передаёт проверенные метаданные локального сегмента записи.
//   - SeqNo: значение SeqNo типа int, используемое согласно назначению этой операции.
//   - Path: путь к локальному файлу или каталогу операции.
//   - FileName: значение FileName типа string, используемое согласно назначению этой операции.
//   - SizeBytes: фактический размер объекта в байтах.
//   - ChecksumSHA256: SHA-256 содержимого артефакта.
type Segment struct {
	SeqNo          int
	Path           string
	FileName       string
	SizeBytes      int64
	ChecksumSHA256 string
}

// NewPostProcessor создает FFmpeg post-processor.
// @parameters:
// - ffmpegPath: путь к ffmpeg, если пусто используется ffmpeg из PATH.
// @return готовый PostProcessor.
func NewPostProcessor(ffmpegPath string) *PostProcessor {
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	return &PostProcessor{
		ffmpegPath:  ffmpegPath,
		ffprobePath: ffprobePath(ffmpegPath),
	}
}

// Finalize склеивает segment_* в final.mp4 и создает preview.jpg.
// @parameters:
// - ctx: контекст операции.
// - recordDir: директория записи в локальном volume.
// @return Result с путями/размерами/checksum или ошибку FFmpeg.
func (p *PostProcessor) Finalize(ctx context.Context, recordDir string) (Result, error) {
	return p.finalize(ctx, recordDir, false)
}

// FinalizeComposite завершает сборку общей записи, проверяет файл и формирует превью.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordDir (string): каталог локальных артефактов конкретной записи.
//
// @return:
//   - результат 1 (Result): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) FinalizeComposite(ctx context.Context, recordDir string) (Result, error) {
	return p.finalize(ctx, recordDir, true)
}

// finalize выполняет общую последовательность завершающей обработки локальных артефактов.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordDir (string): каталог локальных артефактов конкретной записи.
//   - composite (bool): логический признак composite, управляющий соответствующей веткой обработки.
//
// @return:
//   - результат 1 (Result): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) finalize(ctx context.Context, recordDir string, composite bool) (Result, error) {
	segments, err := findSegments(recordDir)
	if err != nil {
		return Result{}, err
	}
	segments = p.usableSegments(ctx, segments)
	if len(segments) == 0 {
		return Result{}, fmt.Errorf("no non-empty record segments found in %s", recordDir)
	}

	listPath := filepath.Join(recordDir, "concat.txt")
	if err := writeConcatList(listPath, segments); err != nil {
		return Result{}, err
	}

	finalPath := filepath.Join(recordDir, "final.mp4")
	previewPath := filepath.Join(recordDir, "preview.jpg")
	var concatErr error
	if composite {
		concatErr = p.concatComposite(ctx, listPath, finalPath)
	} else {
		concatErr = p.concat(ctx, listPath, finalPath)
	}
	if concatErr != nil {
		return Result{}, concatErr
	}
	if composite {
		if _, err := p.ValidateOutput(ctx, finalPath, true); err != nil {
			return Result{}, err
		}
	}
	if err := p.preview(ctx, finalPath, previewPath); err != nil {
		return Result{}, err
	}

	finalInfo, err := os.Stat(finalPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat final video: %w", err)
	}
	previewInfo, err := os.Stat(previewPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat preview: %w", err)
	}
	finalChecksum, err := checksumSHA256(finalPath)
	if err != nil {
		return Result{}, err
	}
	previewChecksum, err := checksumSHA256(previewPath)
	if err != nil {
		return Result{}, err
	}
	segmentInfos, err := segmentInfos(segments)
	if err != nil {
		return Result{}, err
	}
	durationSec, _ := p.durationSec(ctx, finalPath)

	return Result{
		FinalPath:        finalPath,
		PreviewPath:      previewPath,
		Segments:         segmentInfos,
		FinalSizeBytes:   finalInfo.Size(),
		PreviewSizeBytes: previewInfo.Size(),
		DurationSec:      durationSec,
		FinalChecksum:    finalChecksum,
		PreviewChecksum:  previewChecksum,
	}, nil
}

// concat запускает FFmpeg concat demuxer и транскодирует результат в MP4/H.264/AAC.
// @parameters:
// - ctx: контекст операции.
// - listPath: concat.txt со списком сегментов.
// - finalPath: путь итогового MP4.
// @return ошибку FFmpeg.
func (p *PostProcessor) concat(ctx context.Context, listPath string, finalPath string) error {
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-b:v", "6M",
		"-maxrate", "6M",
		"-bufsize", "12M",
		"-r", "30",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-movflags", "+faststart",
		finalPath,
	}
	output, err := runBounded(runCtx, p.ffmpegPath, args...)
	if err != nil {
		return fmt.Errorf("concat final video: %w; ffmpeg=%s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// preview извлекает preview.jpg из итогового MP4.
// @parameters:
// - ctx: контекст операции.
// - finalPath: путь итогового видео.
// - previewPath: путь preview.jpg.
// @return ошибку FFmpeg.
func (p *PostProcessor) preview(ctx context.Context, finalPath string, previewPath string) error {
	if err := p.previewAt(ctx, finalPath, previewPath, "1"); err == nil {
		return nil
	}

	return p.previewAt(ctx, finalPath, previewPath, "0")
}

// previewAt извлекает кадр предварительного просмотра в заданный момент видео.
// Внешняя команда или запрос использует контекст операции.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - finalPath (string): значение finalPath типа string, используемое согласно назначению этой операции.
//   - previewPath (string): значение previewPath типа string, используемое согласно назначению этой операции.
//   - second (string): значение second типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) previewAt(ctx context.Context, finalPath string, previewPath string, second string) error {
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{
		"-hide_banner",
		"-y",
		"-ss", second,
		"-i", finalPath,
		"-frames:v", "1",
		"-q:v", "2",
		previewPath,
	}
	output, err := runBounded(runCtx, p.ffmpegPath, args...)
	if err != nil {
		return fmt.Errorf("create preview: %w; ffmpeg=%s", err, strings.TrimSpace(string(output)))
	}
	if info, err := os.Stat(previewPath); err != nil || info.Size() == 0 {
		return fmt.Errorf("preview frame was not produced at %s seconds", second)
	}

	return nil
}

// durationSec получает длительность файла в целых секундах через ffprobe.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - path (string): путь к локальному файлу или каталогу операции.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) durationSec(ctx context.Context, path string) (int, error) {
	value, err := p.durationFloat(ctx, path)
	if err != nil {
		return 0, err
	}

	return int(value + 0.5), nil
}

// durationFloat получает точную длительность файла через ffprobe.
// Внешняя команда или запрос использует контекст операции.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - path (string): путь к локальному файлу или каталогу операции.
//
// @return:
//   - результат 1 (float64): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *PostProcessor) durationFloat(ctx context.Context, path string) (float64, error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		path,
	}
	output, err := runBounded(runCtx, p.ffprobePath, args...)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil {
		return 0, err
	}

	return value, nil
}

// usableSegments отбирает сегменты, пригодные для объединения в итоговую запись.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - segments ([]string): доступные сегменты записи для итоговой сборки.
//
// @return:
//   - результат 1 ([]string): собранные элементы результата; состав ограничивается параметрами операции.
func (p *PostProcessor) usableSegments(ctx context.Context, segments []string) []string {
	result := make([]string, 0, len(segments))
	for _, segment := range segments {
		duration, err := p.durationFloat(ctx, segment)
		if err != nil || duration <= 0 {
			continue
		}
		result = append(result, segment)
	}

	return result
}

// findSegments находит файлы сегментов в каталоге записи.
//
// @parameters:
//   - recordDir (string): каталог локальных артефактов конкретной записи.
//
// @return:
//   - результат 1 ([]string): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func findSegments(recordDir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(recordDir, "segment_*.*"))
	if err != nil {
		return nil, fmt.Errorf("glob segments: %w", err)
	}
	sort.Strings(matches)

	segments := make([]string, 0, len(matches))
	for _, path := range matches {
		if strings.Contains(filepath.Base(path), ".partial") {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat segment %s: %w", path, err)
		}
		if info.IsDir() || info.Size() == 0 {
			continue
		}
		segments = append(segments, path)
	}

	return segments, nil
}

// segmentInfos собирает размеры, длительности и контрольные суммы сегментов.
//
// @parameters:
//   - paths ([]string): набор значений paths для последовательной или пакетной обработки.
//
// @return:
//   - результат 1 ([]Segment): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func segmentInfos(paths []string) ([]Segment, error) {
	result := make([]Segment, 0, len(paths))
	for index, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat segment %s: %w", path, err)
		}
		checksum, err := checksumSHA256(path)
		if err != nil {
			return nil, err
		}
		seqNo := index
		if parsed, err := segmentSeqNo(info.Name()); err == nil {
			seqNo = parsed
		}
		result = append(result, Segment{
			SeqNo:          seqNo,
			Path:           path,
			FileName:       info.Name(),
			SizeBytes:      info.Size(),
			ChecksumSHA256: checksum,
		})
	}

	return result, nil
}

// segmentSeqNo извлекает порядковый номер сегмента из имени файла.
//
// @parameters:
//   - fileName (string): значение fileName типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func segmentSeqNo(fileName string) (int, error) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(fileName, "segment_"), filepath.Ext(fileName))
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("parse segment seq no %q: %w", fileName, err)
	}

	return value, nil
}

// writeConcatList создаёт входной список файлов для режима concat FFmpeg с безопасным экранированием путей.
//
// @parameters:
//   - path (string): путь к локальному файлу или каталогу операции.
//   - segments ([]string): доступные сегменты записи для итоговой сборки.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func writeConcatList(path string, segments []string) error {
	var b strings.Builder
	for _, segment := range segments {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(segment, "'", "'\\''"))
		b.WriteString("'\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// checksumSHA256 вычисляет SHA-256 содержимого локального файла потоковым чтением.
//
// @parameters:
//   - path (string): путь к локальному файлу или каталогу операции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func checksumSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s for checksum: %w", path, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ffprobePath определяет путь к ffprobe рядом с выбранным FFmpeg.
//
// @parameters:
//   - ffmpegPath (string): значение ffmpegPath типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func ffprobePath(ffmpegPath string) string {
	if ffmpegPath == "ffmpeg" {
		return "ffprobe"
	}
	return filepath.Join(filepath.Dir(ffmpegPath), "ffprobe")
}
