package ffmpeg_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ffmpeginfra "github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
)

// TestPostProcessorFinalizeIgnoresEmptySegmentsAndNonSegmentFiles проверяет игнорирование пустых сегментов и посторонних файлов при финализации.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPostProcessorFinalizeIgnoresEmptySegmentsAndNonSegmentFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "segment_000000.mkv"), nil, 0o644); err != nil {
		t.Fatalf("write empty segment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "final.mp4"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write final: %v", err)
	}

	processor := ffmpeginfra.NewPostProcessor("ffmpeg")
	_, err := processor.Finalize(context.Background(), dir)
	if err == nil {
		t.Fatal("Finalize() error = nil, want no non-empty segments error")
	}
	if !strings.Contains(err.Error(), "no non-empty record segments found") {
		t.Fatalf("Finalize() error = %q, want no non-empty segments error", err.Error())
	}
}
