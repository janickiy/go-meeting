package composite

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
)

// TestDeterministicGridAndScreenLayout проверяет сценарий «Deterministic сетка и экран Layout», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestDeterministicGridAndScreenLayout(t *testing.T) {
	for _, count := range []int{1, 2, 3, 4, 5, 10} {
		tracks := make([]media.Track, count)
		for i := range tracks {
			tracks[i] = media.Track{ID: uuid.NewString(), ParticipantID: uuid.NewString(), Kind: media.KindVideo, Source: media.SourceCamera}
		}
		layout := GridLayout(tracks, 1280, 720)
		if len(layout.Tiles) != count {
			t.Fatal(layout)
		}
		for _, tile := range layout.Tiles {
			if tile.Width%2 != 0 || tile.Height%2 != 0 || tile.X+tile.Width > 1280 || tile.Y+tile.Height > 720 {
				t.Fatal(tile)
			}
		}
		for i, j := 0, len(tracks)-1; i < j; i, j = i+1, j-1 {
			tracks[i], tracks[j] = tracks[j], tracks[i]
		}
		if !reflect.DeepEqual(layout, GridLayout(tracks, 1280, 720)) {
			t.Fatal("arrival order changed layout")
		}
	}
	tracks := []media.Track{{ID: "camera", Kind: media.KindVideo, Source: media.SourceCamera}, {ID: "screen", Kind: media.KindVideo, Source: media.SourceVideoScreen}, {ID: "mic", Kind: media.KindAudio, Source: media.SourceMicrophone}}
	result := GridLayout(tracks, 1280, 720)
	if result.Name != "screen" || len(result.Tiles) != 2 || result.Tiles[0].TrackID != "screen" || result.Tiles[0].Width != 960 {
		t.Fatal(result)
	}
}

// TestCompositionGraphMixesSourcesAndPreservesOffsets проверяет смешивание источников и сохранение смещений в графе композиции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCompositionGraphMixesSourcesAndPreservesOffsets(t *testing.T) {
	c := NewComposer("ffmpeg", 1280, 720, 30, 1)
	chunk := Chunk{Duration: 5, Sources: []Source{{Track: media.Track{ID: "v", Kind: media.KindVideo, Source: media.SourceCamera}, File: "v.ivf", Offset: 0.25, End: 3}, {Track: media.Track{ID: "a", Kind: media.KindAudio, Source: media.SourceMicrophone}, File: "a.ogg", Offset: 0.12, End: 4}}}
	args, err := c.Arguments(t.TempDir(), chunk, "output.mp4")
	if err != nil {
		t.Fatal(err)
	}
	graph := strings.Join(args, " ")
	for _, expected := range []string{"-reinit_filter 0", "PTS-STARTPTS+0.250000/TB", "eof_action=repeat:repeatlast=1", "between(t,0.250000,3.000000)", "adelay=120:all=1", "amix=inputs=2", "normalize=1", "alimiter=limit=0.95", "-c:v libx264", "-c:a aac"} {
		if !strings.Contains(graph, expected) {
			t.Fatalf("graph missing %s", expected)
		}
	}
	chunk.Sources[0].File = "../escape.ivf"
	if _, err := c.Arguments(t.TempDir(), chunk, "output.mp4"); err == nil {
		t.Fatal("accepted path traversal")
	}
}

// TestVP8CaptureWaitsForKeyframeAndOrdersRTP проверяет ожидание ключевого кадра VP8 и упорядочивание RTP.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestVP8CaptureWaitsForKeyframeAndOrdersRTP(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o750); err != nil {
		t.Fatal(err)
	}
	track := media.EgressTrack{Track: media.Track{ID: uuid.NewString(), Kind: media.KindVideo, Source: media.SourceCamera}, MimeType: "video/VP8", ClockRate: 90000}
	w, err := newSourceWriter(dir, 0, track)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte{0, 0, 0, 0x9d, 0x01, 0x2a, 0x80, 0x02, 0xe0, 0x01, 1, 2, 3}
	packets := []*rtp.Packet{
		{Header: rtp.Header{Version: 2, SequenceNumber: 1, Timestamp: 90000, Marker: true}, Payload: append([]byte{0x10}, []byte{1, 2, 3}...)},
		{Header: rtp.Header{Version: 2, SequenceNumber: 2, Timestamp: 93000, Marker: false}, Payload: append([]byte{0x10}, key[:7]...)},
		{Header: rtp.Header{Version: 2, SequenceNumber: 3, Timestamp: 93000, Marker: true}, Payload: append([]byte{0}, key[7:]...)},
		{Header: rtp.Header{Version: 2, SequenceNumber: 4, Timestamp: 96000, Marker: true}, Payload: append([]byte{0x10}, []byte{1, 2, 3}...)},
	}
	for _, i := range []int{0, 2, 1, 3} {
		if err := w.WriteRTP(packets[i], int64(time.Second)+int64(packets[i].Timestamp)*int64(time.Second)/90000); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(w.path)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(data[24:]) != 2 {
		t.Fatalf("expected keyframe+delta, got %d", binary.LittleEndian.Uint32(data[24:]))
	}
	if binary.LittleEndian.Uint16(data[12:]) != 640 || binary.LittleEndian.Uint16(data[14:]) != 480 {
		t.Fatal("keyframe dimensions lost")
	}
}

// TestCaptureRejectsMissingHelloAndInvalidCodec проверяет сценарий «захват Rejects отсутствующий Hello и некорректный Codec», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCaptureRejectsMissingHelloAndInvalidCodec(t *testing.T) {
	c := NewComposer("missing-ffmpeg", 640, 360, 25, 1)
	for _, input := range []string{`{"type":"rtp","sequence":1,"capturedAt":1}`, `{"type":"hello","sequence":1,"capturedAt":1}` + "\n" + `{"type":"track","sequence":2,"capturedAt":1,"track":{"id":"` + uuid.NewString() + `","kind":"video","mimeType":"video/H264","clockRate":90000}}`} {
		if err := c.Capture(context.Background(), context.Background(), strings.NewReader(input), t.TempDir(), CaptureOptions{SegmentDuration: time.Second}); err == nil {
			t.Fatal("invalid stream accepted")
		}
	}
}

// cancelAtEOF хранит состояние отмены тестового чтения при EOF.
//   - reader: источник содержимого либо читатель карточек записи согласно типу.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
type cancelAtEOF struct {
	reader io.Reader
	cancel context.CancelFunc
}

// Read читает состояние ресурсов компонента для дальнейшей обработки или ответа.
//
// @args
//   - p ([]byte): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r cancelAtEOF) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

// TestCapturePersistsShortFinalChunk проверяет сохранение короткого последнего сегмента захвата.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCapturePersistsShortFinalChunk(t *testing.T) {
	id := uuid.NewString()
	track := media.EgressTrack{Track: media.Track{ID: id, Kind: media.KindVideo, Source: media.SourceCamera}, MimeType: "video/VP8", ClockRate: 90000}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	sequence := uint64(1)
	epoch := int64(time.Second)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - frame (media.EgressFrame): значение frame типа media.EgressFrame, используемое согласно назначению этой операции.
	write := func(frame media.EgressFrame) {
		frame.Sequence = sequence
		sequence++
		if err := encoder.Encode(frame); err != nil {
			t.Fatal(err)
		}
	}
	write(media.EgressFrame{Type: "hello", CapturedAt: epoch})
	write(media.EgressFrame{Type: "track", CapturedAt: epoch, Track: &track})
	for i := 0; i < 24; i++ {
		payload := []byte{0x10, 0, 0, 0, 0x9d, 0x01, 0x2a, 0x80, 0x02, 0xe0, 0x01, 1, 2, 3}
		packet := rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(i), Timestamp: uint32(i * 3600), Marker: true}, Payload: payload}
		raw, err := packet.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		write(media.EgressFrame{Type: "rtp", TrackID: id, CapturedAt: epoch + int64(i)*int64(40*time.Millisecond), RTP: raw})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	// Подставной кадр намеренно не декодируется. Проверяем сохранность захваченных данных
	// при сбое до композиции, включая последний неполный сегмент.
	_ = NewComposer("/usr/bin/false", 640, 360, 25, 1).Capture(ctx, context.Background(), cancelAtEOF{&input, cancel}, dir, CaptureOptions{SegmentDuration: 2 * time.Second})
	data, err := os.ReadFile(filepath.Join(dir, "chunk_000000.json"))
	if err != nil {
		t.Fatal("tail manifest was not persisted", err)
	}
	var chunk Chunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		t.Fatal(err)
	}
	if chunk.Duration < 0.9 || chunk.Duration > 1 || len(chunk.Sources) != 1 {
		t.Fatalf("invalid final partial chunk: %+v", chunk)
	}
}

// TestSegmentBoundaryReusesVP8GOPAndOpusPreroll проверяет переиспользование группы кадров VP8 и начальных данных Opus на границе сегмента.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestSegmentBoundaryReusesVP8GOPAndOpusPreroll(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []media.Kind{media.KindVideo, media.KindAudio} {
		track := media.EgressTrack{Track: media.Track{ID: uuid.NewString(), Kind: kind, Source: media.SourceCamera}, ClockRate: 90000, MimeType: "video/VP8"}
		step := uint32(3600)
		duration := 40 * time.Millisecond
		if kind == media.KindAudio {
			track.ClockRate = 48000
			track.MimeType = "audio/opus"
			track.Source = media.SourceMicrophone
			track.Channels = 2
			step = 960
			duration = 20 * time.Millisecond
		}
		first, err := newSourceWriter(dir, 0, track)
		if err != nil {
			t.Fatal(err)
		}
		// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
		//
		// @args
		//   - i (int): значение i типа int, используемое согласно назначению этой операции.
		//
		// @return:
		//   - результат 1 (*rtp.Packet): значение, подготовленное операцией для вызывающей стороны.
		packet := func(i int) *rtp.Packet {
			payload := []byte{0xf8, 0xff, 0xfe}
			if kind == media.KindVideo {
				payload = []byte{0x10, 1, 2, 3}
				if i == 0 {
					payload = []byte{0x10, 0, 0, 0, 0x9d, 0x01, 0x2a, 0x80, 0x02, 0xe0, 0x01, 1, 2, 3}
				}
			}
			return &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(i), Timestamp: uint32(i) * step, Marker: true}, Payload: payload}
		}
		for i := 0; i < 12; i++ {
			if err := first.WriteRTP(packet(i), int64(time.Second)+int64(i)*int64(duration)); err != nil {
				t.Fatal(err)
			}
		}
		if err := first.closeChunk(false); err != nil {
			t.Fatal(err)
		}
		second, err := newSourceWriterState(dir, 1, track, first.state)
		if err != nil {
			t.Fatal(err)
		}
		if second.samples < 6 {
			t.Fatalf("%s pre-roll missing: %d samples", kind, second.samples)
		}
		for i := 12; i < 24; i++ {
			if err := second.WriteRTP(packet(i), int64(time.Second)+int64(i)*int64(duration)); err != nil {
				t.Fatal(err)
			}
		}
		if err := second.Close(); err != nil {
			t.Fatal(err)
		}
		if second.firstAt >= int64(time.Second)+12*int64(duration) {
			t.Fatalf("%s segment waits for new source frame", kind)
		}
		if kind == media.KindVideo {
			data, err := os.ReadFile(second.path)
			if err != nil {
				t.Fatal(err)
			}
			if binary.LittleEndian.Uint32(data[24:]) < 20 || data[44]&1 != 0 {
				t.Fatal("new segment lacks original keyframe and dependent GOP")
			}
		}
	}
}

// TestCaptureBoundsEndedTrackBuffersDuringRapidReplacement проверяет ограничение буферов завершённых дорожек при быстрой смене устройств.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCaptureBoundsEndedTrackBuffersDuringRapidReplacement(t *testing.T) {
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	sequence := uint64(1)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - frame (media.EgressFrame): значение frame типа media.EgressFrame, используемое согласно назначению этой операции.
	write := func(frame media.EgressFrame) {
		frame.Sequence = sequence
		sequence++
		frame.CapturedAt = int64(time.Second) + int64(sequence)*int64(time.Millisecond)
		if err := encoder.Encode(frame); err != nil {
			t.Fatal(err)
		}
	}
	write(media.EgressFrame{Type: "hello"})
	for i := 0; i < 2; i++ {
		id := uuid.NewString()
		track := media.EgressTrack{Track: media.Track{ID: id, Kind: media.KindVideo, Source: media.SourceCamera}, MimeType: "video/VP8", ClockRate: 90000}
		write(media.EgressFrame{Type: "track", Track: &track})
		packet := rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 1, Timestamp: 90000, Marker: true}, Payload: []byte{0x10, 0, 0, 0, 0x9d, 0x01, 0x2a, 0x80, 0x02, 0xe0, 0x01, 1, 2, 3}}
		raw, err := packet.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		write(media.EgressFrame{Type: "rtp", TrackID: id, RTP: raw})
		write(media.EgressFrame{Type: "track.end", TrackID: id})
	}
	err := NewComposer("/usr/bin/false", 640, 360, 25, 1).Capture(context.Background(), context.Background(), &input, t.TempDir(), CaptureOptions{SegmentDuration: time.Second, MaxTracks: 1})
	if err == nil || !strings.Contains(err.Error(), "retained track limit") {
		t.Fatalf("track replacement bypassed state bound: %v", err)
	}
}

// TestCaptureFailsPromptlyWhenCompositorExits проверяет быстрый отказ захвата при завершении процесса композиции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCaptureFailsPromptlyWhenCompositorExits(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		finished <- NewComposer("/usr/bin/false", 640, 360, 25, 1).Capture(ctx, ctx, reader, t.TempDir(), CaptureOptions{SegmentDuration: time.Second})
	}()
	written := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer close(written)
		encoder := json.NewEncoder(writer)
		sequence := uint64(1)
		at := int64(time.Second)
		// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
		//
		// @args
		//   - frame (media.EgressFrame): значение frame типа media.EgressFrame, используемое согласно назначению этой операции.
		//
		// @return:
		//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
		send := func(frame media.EgressFrame) bool {
			frame.Sequence = sequence
			sequence++
			frame.CapturedAt = at
			return encoder.Encode(frame) == nil
		}
		if !send(media.EgressFrame{Type: "hello"}) {
			return
		}
		// Для проверки сбоя процесса композиции публикация медиа не требуется:
		// реальный интервал без камеры также создаёт чёрный сегмент с тишиной.
		at += int64(time.Second)
		if !send(media.EgressFrame{Type: "ping"}) {
			return
		}
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				at += int64(10 * time.Millisecond)
				if !send(media.EgressFrame{Type: "ping"}) {
					return
				}
			}
		}
	}()
	select {
	case err := <-finished:
		if err == nil || !strings.Contains(err.Error(), "compose segment") {
			t.Fatalf("missing compositor failure: %v", err)
		}
	case <-time.After(2 * time.Second):
		cancel()
		reader.Close()
		t.Fatal("failed FFmpeg left capture running")
	}
	cancel()
	writer.Close()
	<-written
}

// TestSparseVideoHoldsAcrossQuietChunksUntilExplicitEnd проверяет сохранение редких видеокадров в тихих сегментах до явного завершения.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestSparseVideoHoldsAcrossQuietChunksUntilExplicitEnd(t *testing.T) {
	ffmpegPath := os.Getenv("RECORDER_TEST_FFMPEG")
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if _, err := exec.LookPath(ffmpegPath); err != nil {
		t.Skip("real FFmpeg is required for decoded sparse-video regression")
	}
	dir := t.TempDir()
	fixture := filepath.Join(dir, "red.ivf")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=red:s=160x90:r=1", "-frames:v", "1", "-c:v", "libvpx", "-deadline", "realtime", fixture).CombinedOutput()
	if err != nil {
		t.Fatalf("generate sparse fixture: %v %s", err, output)
	}
	file, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ivf, _, err := ivfreader.NewWith(file)
	if err != nil {
		t.Fatal(err)
	}
	keyframe, _, err := ivf.ParseNextFrame()
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	track := media.EgressTrack{Track: media.Track{ID: id, Kind: media.KindVideo, Source: media.SourceVideoScreen}, MimeType: "video/VP8", ClockRate: 90000}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	sequence := uint64(1)
	epoch := int64(time.Second)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - at (time.Duration): однозначное время планируемой операции; nil означает отсутствие значения, если это допускает тип.
	//   - frame (media.EgressFrame): значение frame типа media.EgressFrame, используемое согласно назначению этой операции.
	write := func(at time.Duration, frame media.EgressFrame) {
		frame.Sequence = sequence
		sequence++
		frame.CapturedAt = epoch + int64(at)
		if err := encoder.Encode(frame); err != nil {
			t.Fatal(err)
		}
	}
	write(0, media.EgressFrame{Type: "hello"})
	write(0, media.EgressFrame{Type: "track", Track: &track})
	payloads := (&codecs.VP8Payloader{}).Payload(1200, keyframe)
	for i, payload := range payloads {
		packet := rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(i + 1), Timestamp: 90000, Marker: i == len(payloads)-1}, Payload: payload}
		raw, err := packet.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		write(100*time.Millisecond, media.EgressFrame{Type: "rtp", TrackID: id, RTP: raw})
	}
	// Закодированный кадр остаётся видимым на протяжении второго сегмента без новых пакетов
	// и исчезает по track.end в середине третьего сегмента.
	write(time.Second, media.EgressFrame{Type: "ping"})
	write(2*time.Second, media.EgressFrame{Type: "ping"})
	write(2500*time.Millisecond, media.EgressFrame{Type: "track.end", TrackID: id})
	write(3*time.Second, media.EgressFrame{Type: "ping"})
	captureCtx, stopCapture := context.WithCancel(ctx)
	defer stopCapture()
	if err := NewComposer(ffmpegPath, 320, 240, 10, 1).Capture(captureCtx, ctx, cancelAtEOF{&input, stopCapture}, dir, CaptureOptions{SegmentDuration: time.Second}); err != nil {
		t.Fatal(err)
	}
	for index, expected := range [][]bool{{true, true, true, true}, {true, true, true, true}, {true, true, false, false}} {
		segment := filepath.Join(dir, []string{"segment_000000.mp4", "segment_000001.mp4", "segment_000002.mp4"}[index])
		pixels, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "error", "-i", segment, "-an", "-vf", "fps=4,scale=32:18", "-pix_fmt", "rgb24", "-f", "rawvideo", "-").Output()
		if err != nil {
			t.Fatal(err)
		}
		const frameSize = 32 * 18 * 3
		if len(pixels) != len(expected)*frameSize {
			t.Fatalf("segment %d decoded %d bytes", index, len(pixels))
		}
		for frame, shouldBeVisible := range expected {
			red := 0
			for pixel := frame * frameSize; pixel < (frame+1)*frameSize; pixel += 3 {
				if pixels[pixel] > 100 && pixels[pixel+1] < 80 && pixels[pixel+2] < 80 {
					red++
				}
			}
			if visible := red > 10; visible != shouldBeVisible {
				t.Fatalf("segment %d frame %d visibility=%v, want %v (red pixels %d)", index, frame, visible, shouldBeVisible, red)
			}
		}
	}
}

// TestCompositeHandlesCameraResolutionChanges проверяет обработку изменения разрешения камеры в композиции.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCompositeHandlesCameraResolutionChanges(t *testing.T) {
	ffmpegPath := os.Getenv("RECORDER_TEST_FFMPEG")
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if _, err := exec.LookPath(ffmpegPath); err != nil {
		t.Skip("real FFmpeg is required for resolution-change regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sources"), 0o750); err != nil {
		t.Fatal(err)
	}
	var keyframes [][]byte
	for _, fixture := range []struct{ name, input string }{{"red", "color=c=red:s=160x90:r=20"}, {"blue", "color=c=blue:s=320x180:r=20"}} {
		path := filepath.Join(dir, fixture.name+".ivf")
		output, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", fixture.input, "-frames:v", "1", "-c:v", "libvpx", "-deadline", "realtime", path).CombinedOutput()
		if err != nil {
			t.Fatalf("generate changing-size fixture: %v %s", err, output)
		}
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		ivf, _, err := ivfreader.NewWith(file)
		if err != nil {
			file.Close()
			t.Fatal(err)
		}
		frame, _, err := ivf.ParseNextFrame()
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		keyframes = append(keyframes, frame)
	}
	track := media.EgressTrack{Track: media.Track{ID: uuid.NewString(), Kind: media.KindVideo, Source: media.SourceCamera}, MimeType: "video/VP8", ClockRate: 90000}
	writer, err := newSourceWriter(dir, 0, track)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		frame := keyframes[0]
		if i >= 20 {
			frame = keyframes[1]
		}
		if err := writer.writeSample(encodedSample{data: frame, timestamp: uint32(i * 4500), at: int64(time.Second) + int64(i)*int64(50*time.Millisecond), duration: 50 * time.Millisecond}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	chunk := Chunk{Duration: 1.5, Sources: []Source{{Track: track.Track, File: filepath.Base(writer.path), Offset: -0.5, End: 1.5}}}
	output := filepath.Join(dir, "resolution-change.mp4")
	args, err := NewComposer(ffmpegPath, 320, 240, 20, 1).Arguments(dir, chunk, output)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, ffmpegPath, args...)
	// Повторяем ту же регрессию на рабочей версии FFmpeg;
	// локальный FFmpeg 9 не воспроизводит ошибку повторной инициализации графа в FFmpeg 6.
	if dockerImage := os.Getenv("RECORDER_TEST_FFMPEG_DOCKER"); dockerImage != "" {
		dockerArgs := []string{"run", "--rm", "--entrypoint", "ffmpeg", "-v", dir + ":" + dir, dockerImage}
		command = exec.CommandContext(ctx, "docker", append(dockerArgs, args...)...)
	}
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compose changing resolution: %v %s", err, result)
	}
	pixels, err := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-loglevel", "error", "-i", output, "-an", "-vf", "fps=4,scale=32:18", "-pix_fmt", "rgb24", "-f", "rawvideo", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	const frameSize = 32 * 18 * 3
	if len(pixels) != 6*frameSize {
		t.Fatalf("expected 6 decoded frames, got %d bytes", len(pixels))
	}
	for frame := 0; frame < 6; frame++ {
		colored := 0
		for pos := frame * frameSize; pos < (frame+1)*frameSize; pos += 3 {
			if (frame < 2 && pixels[pos] > 100 && pixels[pos+2] < 80) || (frame >= 2 && pixels[pos+2] > 100 && pixels[pos] < 80) {
				colored++
			}
		}
		if colored < 10 {
			t.Fatalf("frame %d lost camera or reset its timeline after dimensions changed: %d colored pixels", frame, colored)
		}
	}
}
