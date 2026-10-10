package ffmpeg

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// RecordingObjectReader читает только приватный объект и не принимает внешние URL.
type RecordingObjectReader interface {
	// OpenRecording открывает принадлежащий серверу ключ с ограничением числа байтов.
	// @args ctx — срок выполнения; key — проверенный ключ хранилища; maximum — предел байтов.
	// @return поток, фактический размер и ошибку хранилища.
	OpenRecording(context.Context, string, int64) (io.ReadCloser, int64, error)
}

// TranscriptionAudio извлекает WAV отдельно от финализации записи и передачи медиа.
type TranscriptionAudio struct {
	objects                      RecordingObjectReader
	binary, tempRoot             string
	maxVideoBytes, maxAudioBytes int64
	maxDuration                  int
}

// NewTranscriptionAudio создаёт независимый обработчик извлечения с конечными пределами объёма и длительности.
// @args objects — private MinIO reader; binary — доверенный FFmpeg path;
// tempRoot — приватный каталог временных файлов; maxVideoBytes/maxAudioBytes — пределы входа и выхода;
// maxDuration — максимально допустимая длительность в секундах.
// @return адаптер AudioSource; actual выделение ресурсов происходит только в Open.
func NewTranscriptionAudio(objects RecordingObjectReader, binary, tempRoot string, maxVideoBytes, maxAudioBytes int64, maxDuration int) *TranscriptionAudio {
	return &TranscriptionAudio{objects: objects, binary: binary, tempRoot: tempRoot, maxVideoBytes: maxVideoBytes, maxAudioBytes: maxAudioBytes, maxDuration: maxDuration}
}

// Open скачивает MP4 ограниченного размера и извлекает монофонический WAV с частотой 16 кГц в приватном временном каталоге.
// @args ctx — конечный срок распознавания речи; source — серверные метаданные готовой записи.
// @return поток чтения и идемпотентную очистку; ошибки не меняют готовую запись.
func (a *TranscriptionAudio) Open(ctx context.Context, source domain.RecordingSource) (domain.Audio, error) {
	if a.objects == nil || a.binary == "" || a.maxVideoBytes < 1 || a.maxAudioBytes < 44 || a.maxDuration < 1 || source.DurationSec < 1 || source.DurationSec > a.maxDuration || source.SizeBytes > a.maxVideoBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_input_limit", Retryable: false}
	}
	root := a.tempRoot
	if root == "" {
		root = os.TempDir()
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	dir, err := os.MkdirTemp(root, "go-recorder-stt-")
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	var once sync.Once
	cleanup := func() { once.Do(func() { _ = os.RemoveAll(dir) }) }
	keep := false
	defer func() {
		if !keep {
			cleanup()
		}
	}()
	input, actual, err := a.objects.OpenRecording(ctx, source.ObjectKey, a.maxVideoBytes)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_storage_unavailable", Retryable: true}
	}
	defer input.Close()
	path := filepath.Join(dir, "input.mp4")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	n, err := io.Copy(file, io.LimitReader(input, a.maxVideoBytes+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil || n != actual || n > a.maxVideoBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_download_failed", Retryable: true}
	}
	output := filepath.Join(dir, "audio.wav")
	_, err = runBounded(ctx, a.binary, "-nostdin", "-hide_banner", "-loglevel", "error", "-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-t", strconv.Itoa(a.maxDuration), "-fs", strconv.FormatInt(a.maxAudioBytes, 10), output)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_extract_failed", Retryable: ctx.Err() != nil}
	}
	stat, err := os.Stat(output)
	if err != nil || stat.Size() <= 44 || stat.Size() >= a.maxAudioBytes {
		return domain.Audio{}, &jobs.Error{Code: "audio_output_limit", Retryable: false}
	}
	reader, err := os.Open(output)
	if err != nil {
		return domain.Audio{}, &jobs.Error{Code: "audio_temp_unavailable", Retryable: true}
	}
	keep = true
	return domain.Audio{Reader: reader, Size: stat.Size(), ContentType: "audio/wav", Cleanup: func() { _ = reader.Close(); cleanup() }}, nil
}
