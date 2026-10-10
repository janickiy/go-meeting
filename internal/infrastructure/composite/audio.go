package composite

import (
	"fmt"
	"github.com/janickiy/meet-space/internal/domain/media"
	"path/filepath"
	"strconv"
	"strings"
)

// audioArguments собирает только аудиофильтры: видеодекодеры и кодировщики не запускаются.
// @args dir — приватный каталог; chunk — закрытые источники; output — серверный путь результата.
// @return аргументы FFmpeg с фиксированными кодеком и лимитами либо ошибка пути.
func (c *Composer) audioArguments(dir string, chunk Chunk, output string) ([]string, error) {
	duration := strconv.FormatFloat(chunk.Duration, 'f', 6, 64)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_complex_threads", "1"}
	sources := []Source{}
	for _, source := range chunk.Sources {
		if source.Track.Kind != media.KindAudio {
			continue
		}
		if filepath.Base(source.File) != source.File || source.File == "." {
			return nil, fmt.Errorf("invalid audio source")
		}
		args = append(args, "-threads", "1", "-i", filepath.Join(dir, "sources", source.File))
		sources = append(sources, source)
	}
	graph := []string{fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=duration=%s[silence]", duration)}
	labels := "[silence]"
	for i, source := range sources {
		graph = append(graph, fmt.Sprintf("[%d:a]aresample=48000:async=1:first_pts=0,aformat=sample_fmts=fltp:channel_layouts=stereo,atrim=start=%.6f,asetpts=PTS-STARTPTS,adelay=%d:all=1,apad,atrim=duration=%s[a%d]", i, max(0, -source.Offset), int(max(0, source.Offset)*1000+.5), duration, i))
		labels += fmt.Sprintf("[a%d]", i)
	}
	graph = append(graph, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=0:normalize=1,alimiter=limit=0.95:latency=1[aout]", labels, len(sources)+1))
	return append(args, "-filter_complex", strings.Join(graph, ";"), "-map", "[aout]", "-vn", "-t", duration, "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-threads", "1", "-movflags", "+faststart", output), nil
}
