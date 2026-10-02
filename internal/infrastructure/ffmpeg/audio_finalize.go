package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FinalizeAudio объединяет аудиосегменты без видеопроцесса и без искусственного превью.
// @args ctx — deadline; dir — приватный каталог закрытых сегментов.
// @return проверенный AAC/MP4, контрольная сумма и метаданные либо ошибка.
func (p *PostProcessor) FinalizeAudio(ctx context.Context, dir string) (Result, error) {
	paths, err := findSegments(dir)
	if err != nil {
		return Result{}, err
	}
	paths = p.usableSegments(ctx, paths)
	if len(paths) == 0 {
		return Result{}, fmt.Errorf("no audio segments")
	}
	list := filepath.Join(dir, "concat.txt")
	if err = writeConcatList(list, paths); err != nil {
		return Result{}, err
	}
	final := filepath.Join(dir, "final.mp4")
	op, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if _, err = runBounded(op, p.ffmpegPath, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "concat", "-safe", "0", "-i", list, "-map", "0:a:0", "-vn", "-c:a", "copy", "-movflags", "+faststart", final); err != nil {
		return Result{}, err
	}
	if _, err = p.validateOutput(ctx, final, true, false); err != nil {
		return Result{}, err
	}
	stat, err := os.Stat(final)
	if err != nil {
		return Result{}, err
	}
	sum, err := checksumSHA256(final)
	if err != nil {
		return Result{}, err
	}
	segments, err := segmentInfos(paths)
	if err != nil {
		return Result{}, err
	}
	duration, err := p.durationSec(ctx, final)
	if err != nil {
		return Result{}, err
	}
	return Result{FinalPath: final, FinalSizeBytes: stat.Size(), FinalChecksum: sum, DurationSec: duration, Segments: segments}, nil
}
