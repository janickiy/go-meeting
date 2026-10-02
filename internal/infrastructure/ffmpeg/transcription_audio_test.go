package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// audioObjectFake изолирует private-object чтение от реального bucket.
type audioObjectFake struct {
	data    []byte
	readErr error
}

// OpenRecording выдаёт точный fixture stream с тем же byte budget контрактом.
// @parameters: ctx/key/maximum — trusted request; @return bounded fixture reader/size/error.
func (o audioObjectFake) OpenRecording(context.Context, string, int64) (io.ReadCloser, int64, error) {
	if o.readErr != nil {
		return nil, 0, o.readErr
	}
	return io.NopCloser(bytes.NewReader(o.data)), int64(len(o.data)), nil
}

// TestTranscriptionAudioRealExtractionAndCleanup проверяет реальный mono16k WAV и все cleanup paths.
// @parameters: t — test runner; отсутствие FFmpeg явно skip, без притворного PASS live extraction.
func TestTranscriptionAudioRealExtractionAndCleanup(t *testing.T) {
	binary, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("FFmpeg required for real extraction test")
	}
	input := filepath.Join(t.TempDir(), "source.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, e := exec.CommandContext(ctx, binary, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=1", "-c:a", "aac", input).CombinedOutput(); e != nil {
		t.Fatalf("audio fixture: %s %v", out, e)
	}
	data, e := os.ReadFile(input)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	source := domain.RecordingSource{DurationSec: 1, ObjectKey: "server-key", SizeBytes: int64(len(data))}
	adapter := NewTranscriptionAudio(audioObjectFake{data: data}, binary, root, 1<<20, 1<<20, 10)
	audio, e := adapter.Open(ctx, source)
	if e != nil {
		t.Fatal(e)
	}
	wav, e := io.ReadAll(audio.Reader)
	if e != nil || len(wav) < 32000 || string(wav[:4]) != "RIFF" || audio.ContentType != "audio/wav" {
		t.Fatal("not bounded WAV", len(wav), e)
	}
	audio.Cleanup()
	audio.Cleanup()
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 0 {
		t.Fatal("successful audio temp leaked", entries, e)
	}
	// -fs прекращает запись, но усечённый WAV нельзя считать полным аудио встречи.
	limited := NewTranscriptionAudio(audioObjectFake{data: data}, binary, root, 1<<20, 100, 10)
	if _, e = limited.Open(ctx, source); e == nil {
		t.Fatal("truncated WAV was accepted")
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("output-limit temp leaked")
	}
	invalid := NewTranscriptionAudio(audioObjectFake{data: []byte("invalid MP4")}, binary, root, 1<<20, 1<<20, 10)
	if _, e = invalid.Open(ctx, source); e == nil {
		t.Fatal("invalid input accepted")
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("FFmpeg failure temp leaked")
	}
}

// TestTranscriptionAudioDownloadAndAdmissionFailures проверяет пределы до extract и cleanup при storage outage.
// @parameters: t — test runner.
func TestTranscriptionAudioDownloadAndAdmissionFailures(t *testing.T) {
	root := t.TempDir()
	adapter := NewTranscriptionAudio(audioObjectFake{readErr: errors.New("private storage token should not leak")}, "ffmpeg", root, 100, 1000, 10)
	if _, e := adapter.Open(context.Background(), domain.RecordingSource{DurationSec: 11}); e == nil {
		t.Fatal("duration admission ignored")
	}
	_, e := adapter.Open(context.Background(), domain.RecordingSource{DurationSec: 1})
	var failure *jobs.Error
	if !errors.As(e, &failure) || failure.Code != "audio_storage_unavailable" || !failure.Retryable {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("storage-error temp leaked")
	}
}
