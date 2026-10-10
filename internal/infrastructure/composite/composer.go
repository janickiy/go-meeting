package composite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/janickiy/meet-space/internal/operations"

	"github.com/janickiy/meet-space/internal/domain/media"
)

// Source описывает один устойчивый медиа-источник для общей композиции.
// @params
//   - Track: медиа-дорожка, которую обрабатывает или подписывает компонент.
//   - File: значение File типа string, используемое согласно назначению этой операции.
//   - Offset: число элементов, пропускаемых перед началом страницы.
//   - End: значение End типа float64, используемое согласно назначению этой операции.
type Source struct {
	Track  media.Track `json:"track"`
	File   string      `json:"file"`
	Offset float64     `json:"offset"`
	End    float64     `json:"end"`
}

// Chunk считает фрагмент устойчивым только после закрытия всех потоков и атомарной публикации манифеста.
// @params
//   - Index: значение Index типа int, используемое согласно назначению этой операции.
//   - Duration: плановая длительность или интервал в единицах, заданных типом.
//   - Sources: набор источников медиа для публикации или композиции.
//   - Layout: расположение источников в итоговом видеокадре.
type Chunk struct {
	Mode        string   `json:"mode,omitempty"`
	StartedAtNS int64    `json:"startedAtNs,omitempty"`
	Index       int      `json:"index"`
	Duration    float64  `json:"duration"`
	Sources     []Source `json:"sources"`
	Layout      Layout   `json:"layout"`
}

// Composer строит общую запись из независимых источников через FFmpeg вне цикла пересылки SFU.
// @params
//   - FFmpegPath: значение FFmpegPath типа string, используемое согласно назначению этой операции.
//   - Width: ширина видеокадра или области в пикселях.
//   - Height: высота видеокадра или области в пикселях.
//   - FPS: значение FPS типа int, используемое согласно назначению этой операции.
//   - Timeout: максимальное время ожидания операции.
//   - slots: канал «slots» для передачи данных или завершения ожидания.
type Composer struct {
	FFmpegPath string
	Width      int
	Height     int
	FPS        int
	Timeout    time.Duration
	slots      chan struct{}
}

// NewComposer создаёт и связывает зависимости компонента Composer, используемого в сборке и проверке аудио- и видеозаписи.
//
// @args
//   - path (string): путь к локальному файлу или каталогу операции.
//   - width (int): ширина видеокадра или области в пикселях.
//   - height (int): высота видеокадра или области в пикселях.
//   - fps (int): частота видеокадров в секунду.
//   - concurrency (int): значение concurrency типа int, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Composer): созданный компонент с переданными зависимостями.
func NewComposer(path string, width, height, fps, concurrency int) *Composer {
	if path == "" {
		path = "ffmpeg"
	}
	if width == 0 {
		width = 1280
	}
	if height == 0 {
		height = 720
	}
	if fps == 0 {
		fps = 30
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &Composer{FFmpegPath: path, Width: width, Height: height, FPS: fps, Timeout: 2 * time.Minute, slots: make(chan struct{}, concurrency)}
}

// Compose собирает общую аудио- и видеозапись из устойчивых фрагментов источников через FFmpeg.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - dir (string): значение dir типа string, используемое согласно назначению этой операции.
//   - chunk (Chunk): устойчивый фрагмент захваченных источников записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Composer) Compose(ctx context.Context, dir string, chunk Chunk) error {
	select {
	case c.slots <- struct{}{}:
		defer /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и проверке аудио- и видеозаписи, используя состояние окружающей функции.

		 */func() { <-c.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	output := filepath.Join(dir, fmt.Sprintf("segment_%06d.mp4", chunk.Index))
	if info, err := os.Stat(output); err == nil && info.Size() > 1024 {
		return nil
	}
	args, err := c.Arguments(dir, chunk, output+".partial.mp4")
	if err != nil {
		return err
	}
	for _, source := range chunk.Sources {
		file, err := os.OpenFile(filepath.Join(dir, "sources", source.File), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		err = file.Sync()
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	workCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(workCtx, c.FFmpegPath, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	operations.FFmpegActive(1)
	defer operations.FFmpegActive(-1)
	var stderr boundedLog
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		operations.Event("ffmpeg_failed")
		return fmt.Errorf("compose segment %d: %w: %s", chunk.Index, err, stderr.String())
	}
	if err := os.Rename(output+".partial.mp4", output); err != nil {
		return err
	}
	return nil
}

// Arguments формирует аргументы FFmpeg для композиции источников и выбранного расположения плиток.
//
// @args
//   - dir (string): значение dir типа string, используемое согласно назначению этой операции.
//   - chunk (Chunk): устойчивый фрагмент захваченных источников записи.
//   - output (string): значение output типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 ([]string): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Composer) Arguments(dir string, chunk Chunk, output string) ([]string, error) {
	if chunk.Duration <= 0 || chunk.Duration > 31 {
		return nil, fmt.Errorf("invalid composite chunk duration")
	}
	if chunk.Mode == "audio_only" || chunk.Mode == "individual_tracks" {
		return c.audioArguments(dir, chunk, output)
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-filter_complex_threads", "1"}
	tracks := make([]media.Track, 0, len(chunk.Sources))
	index := make(map[string]int, len(chunk.Sources))
	for i, source := range chunk.Sources {
		if filepath.Base(source.File) != source.File || source.File == "." {
			return nil, fmt.Errorf("invalid source file")
		}
		// Браузеры меняют размеры VP8 в ходе передачи. Повторная инициализация всего графа
		// фильтров сбрасывает временные отметки и состояние наложения, особенно в FFmpeg 6,
		// из-за чего исправная камера может исчезнуть. Фильтр scale принимает новые размеры
		// входа и сохраняет фиксированные размеры выходного изображения композиции.
		args = append(args, "-threads", "1", "-reinit_filter", "0", "-i", filepath.Join(dir, "sources", source.File))
		tracks = append(tracks, source.Track)
		index[source.Track.ID] = i
	}
	layout := GridLayout(tracks, c.Width, c.Height)
	if chunk.Mode == "screen_focus" && layout.Name == "screen" {
		layout.Tiles = []Tile{{TrackID: layout.Tiles[0].TrackID, Width: layout.Width, Height: layout.Height}}
	}
	duration := strconv.FormatFloat(chunk.Duration, 'f', 6, 64)
	graph := []string{fmt.Sprintf("color=c=0x101827:s=%dx%d:r=%d:d=%s[base]", layout.Width, layout.Height, c.FPS, duration)}
	previous := "base"
	for n, tile := range layout.Tiles {
		i := index[tile.TrackID]
		source := chunk.Sources[i]
		graph = append(graph, fmt.Sprintf("[%d:v]setpts=PTS-STARTPTS+%.6f/TB,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=0x101827,setsar=1[v%d]", i, source.Offset, tile.Width, tile.Height, tile.Width, tile.Height, n))
		// Источники с переменной частотой кадров и статичные источники браузера сохраняют
		// последний кадр видимым, пока активны. Source.End ограничен явным событием track.end,
		// поэтому повтор кадра не может восстановить завершённую камеру или экран.
		graph = append(graph, fmt.Sprintf("[%s][v%d]overlay=%d:%d:eof_action=repeat:repeatlast=1:enable='between(t,%.6f,%.6f)'[mix%d]", previous, n, tile.X, tile.Y, source.Offset, source.End, n))
		previous = fmt.Sprintf("mix%d", n)
	}
	graph = append(graph, fmt.Sprintf("[%s]fps=%d,format=yuv420p,trim=duration=%s,setpts=PTS-STARTPTS[vout]", previous, c.FPS, duration))
	graph = append(graph, fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=duration=%s[silence]", duration))
	audioLabels := "[silence]"
	audioCount := 1
	for i, source := range chunk.Sources {
		if source.Track.Kind != media.KindAudio {
			continue
		}
		delay := int(max(0, source.Offset)*1000 + 0.5)
		graph = append(graph, fmt.Sprintf("[%d:a]aresample=48000:async=1:first_pts=0,aformat=sample_fmts=fltp:channel_layouts=stereo,atrim=start=%.6f,asetpts=PTS-STARTPTS,adelay=%d:all=1,apad,atrim=duration=%s[a%d]", i, max(0, -source.Offset), delay, duration, i))
		audioLabels += fmt.Sprintf("[a%d]", i)
		audioCount++
	}
	graph = append(graph, fmt.Sprintf("%samix=inputs=%d:duration=first:dropout_transition=0:normalize=1,alimiter=limit=0.95:latency=1[aout]", audioLabels, audioCount))
	args = append(args, "-filter_complex", strings.Join(graph, ";"), "-map", "[vout]", "-map", "[aout]", "-t", duration, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-threads", "2", "-r", strconv.Itoa(c.FPS), "-g", strconv.Itoa(c.FPS*2), "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-movflags", "+faststart", output)
	return args, nil
}

// Recover восстанавливает доступные устойчивые фрагменты записи после прерывания обработки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - dir (string): значение dir типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *Composer) Recover(ctx context.Context, dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "chunk_*.json"))
	if err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var chunk Chunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err
		}
		if err := c.Compose(ctx, dir, chunk); err != nil {
			return err
		}
	}
	return nil
}

// boundedLog сохраняет ограниченный диагностический вывод без бесконечного роста памяти.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - data: полезная нагрузка события или байты обрабатываемого содержимого.
type boundedLog struct {
	mu   sync.Mutex
	data []byte
}

// Write принимает байты вывода в ограниченный буфер и соблюдает контракт io.Writer.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p ([]byte): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 8192 {
		b.data = b.data[len(b.data)-8192:]
	}
	return len(p), nil
}

// String возвращает строковое представление накопленного значения или ограниченного диагностического вывода.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (b *boundedLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }
