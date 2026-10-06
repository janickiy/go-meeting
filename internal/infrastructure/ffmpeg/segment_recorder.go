package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// RTPTrack описывает кодек и локальный RTP-вход одной записываемой дорожки.
// @params
//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - MimeType: заявленный либо проверенный MIME-тип содержимого.
//   - PayloadType: значение PayloadType типа uint8, используемое согласно назначению этой операции.
//   - ClockRate: значение ClockRate типа uint32, используемое согласно назначению этой операции.
//   - Channels: значение Channels типа uint16, используемое согласно назначению этой операции.
//   - Port: локальный сетевой порт передачи RTP.
//   - FmtpLine: значение FmtpLine типа string, используемое согласно назначению этой операции.
type RTPTrack struct {
	Kind        string
	MimeType    string
	PayloadType uint8
	ClockRate   uint32
	Channels    uint16
	Port        int
	FmtpLine    string
}

// SegmentRecorder настраивает и запускает FFmpeg для сегментной записи RTP-потоков.
// @params
//   - path: путь к локальному файлу или каталогу операции.
//   - logger: значение logger типа *log.Logger, используемое согласно назначению этой операции.
type SegmentRecorder struct {
	path   string
	logger *log.Logger
}

// SegmentProcess владеет запущенным процессом сегментной записи и его завершением.
// @params
//   - cmd: значение cmd типа *exec.Cmd, используемое согласно назначению этой операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - done: канал уведомления о завершении ресурса.
//   - stderr: значение stderr типа logTail, используемое согласно назначению этой операции.
//   - stdin: значение stdin типа io.WriteCloser, используемое согласно назначению этой операции.
//   - logger: значение logger типа *log.Logger, используемое согласно назначению этой операции.
type SegmentProcess struct {
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	done     chan struct{}
	waitErr  error
	stopOnce sync.Once
	stderr   logTail
	stdin    io.WriteCloser
	logger   *log.Logger
}

// NewSegmentRecorder создаёт компонент записи сегментов FFmpeg.
// @args
// - path: путь к ffmpeg.
// - logger: журнал записывающего воркера.
// @return SegmentRecorder.
func NewSegmentRecorder(path string, logger *log.Logger) *SegmentRecorder {
	if path == "" {
		path = "ffmpeg"
	}
	if logger == nil {
		logger = log.Default()
	}

	return &SegmentRecorder{path: path, logger: logger}
}

// Start создаёт SDP и запускает мультиплексор сегментов FFmpeg.
// @args
// - ctx: context записи.
// - recordID: UUID записи.
// - tracks: входящие RTP-треки.
// - workDir: директория для input.sdp.
// - outputDir: директория segment_*.mkv.
// - segmentDurationSec: длительность сегмента.
// @return процесс FFmpeg или ошибку запуска.
func (r *SegmentRecorder) Start(ctx context.Context, recordID string, tracks []RTPTrack, workDir string, outputDir string, segmentDurationSec int) (*SegmentProcess, error) {
	if len(tracks) == 0 {
		return nil, fmt.Errorf("ffmpeg needs at least one RTP track")
	}
	if segmentDurationSec <= 0 {
		segmentDurationSec = 5
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ffmpeg work dir: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ffmpeg output dir: %w", err)
	}
	sdp, err := buildRTPInputSDP(tracks)
	if err != nil {
		return nil, err
	}
	sdpPath := filepath.Join(workDir, "input.sdp")
	if err := os.WriteFile(sdpPath, []byte(sdp), 0o644); err != nil {
		return nil, fmt.Errorf("write ffmpeg sdp: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	segmentTpl := filepath.Join(outputDir, "segment_%06d.mkv")
	args := []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-protocol_whitelist", "file,udp,rtp",
		"-fflags", "+genpts",
		"-f", "sdp",
		"-i", sdpPath,
		"-map", "0:v:0?",
		"-map", "0:a:0?",
		"-c", "copy",
		"-f", "segment",
		"-segment_time", fmt.Sprintf("%d", segmentDurationSec),
		"-segment_format", "matroska",
		"-reset_timestamps", "1",
		segmentTpl,
	}
	cmd := exec.CommandContext(runCtx, r.path, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdin: %w", err)
	}
	process := &SegmentProcess{
		cmd:    cmd,
		cancel: cancel,
		done:   make(chan struct{}),
		stdin:  stdin,
		logger: r.logger,
	}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start ffmpeg segment recorder: %w", err)
	}
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и проверке аудио- и видеозаписи, используя состояние окружающей функции.

	 */func() {
		process.waitErr = cmd.Wait()
		close(process.done)
	}()

	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-process.done:
		cancel()
		return nil, fmt.Errorf("ffmpeg exited during startup: %v; stderr=%s", process.waitErr, process.Stderr())
	case <-timer.C:
		r.logger.Printf("record %s ffmpeg segment recorder started", recordID)
		return process, nil
	case <-ctx.Done():
		cancel()
		// Startup owns the child until it hands a process to the session. Join
		// Wait here so a cancelled startup cannot outlive Stop/Shutdown.
		<-process.done
		return nil, ctx.Err()
	}
}

// Stop плавно завершает FFmpeg, чтобы мультиплексор сегментов закрыл текущий файл.
// @args
// - timeout: время ожидания плавной остановки.
// @return nil; незавершенный хвостовой сегмент отфильтрует post-processing.
func (p *SegmentProcess) Stop(timeout time.Duration) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	p.stopOnce.Do(func() { p.stop(timeout) })
	return nil
}

func (p *SegmentProcess) stop(timeout time.Duration) {
	defer p.cancel()
	select {
	case <-p.done:
		return
	default:
	}
	if p.stdin != nil {
		_, _ = io.WriteString(p.stdin, "q\n")
		_ = p.stdin.Close()
	} else {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		if p.waitErr != nil && p.logger != nil {
			p.logger.Printf("ffmpeg stopped with error: %v; stderr=%s", p.waitErr, p.Stderr())
		}
		return
	case <-timer.C:
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-p.done:
			if p.waitErr != nil && p.logger != nil {
				p.logger.Printf("ffmpeg stopped after SIGTERM: %v; stderr=%s", p.waitErr, p.Stderr())
			}
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			p.cancel()
			select {
			case <-p.done:
				if p.waitErr != nil && p.logger != nil {
					p.logger.Printf("ffmpeg killed after stop timeout: %v; stderr=%s", p.waitErr, p.Stderr())
				}
			case <-time.After(2 * time.Second):
				if p.logger != nil {
					p.logger.Printf("ffmpeg kill wait timeout; stderr=%s", p.Stderr())
				}
			}
		}
		return
	}
}

// Done broadcasts completion after cmd.Wait has reaped the process and closed its pipes.
func (p *SegmentProcess) Done() <-chan struct{} { return p.done }

// WaitErr returns the process result; callers must first observe Done.
func (p *SegmentProcess) WaitErr() error { return p.waitErr }

// Stderr возвращает поток ошибок stderr процесса FFmpeg.
// @args нет.
// @return строку stderr без крайних пробелов.
func (p *SegmentProcess) Stderr() string {
	if p == nil {
		return ""
	}

	return strings.TrimSpace(p.stderr.String())
}

// buildRTPInputSDP формирует локальное SDP-описание RTP-входов FFmpeg для выбранных дорожек.
//
// @args
//   - tracks ([]RTPTrack): набор дорожек, входящих в операцию.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func buildRTPInputSDP(tracks []RTPTrack) (string, error) {
	var b strings.Builder
	b.WriteString("v=0\n")
	b.WriteString("o=- 0 0 IN IP4 127.0.0.1\n")
	b.WriteString("s=go-recorder\n")
	b.WriteString("c=IN IP4 127.0.0.1\n")
	b.WriteString("t=0 0\n")

	supported := 0
	for _, track := range tracks {
		codec := sdpCodecName(track.MimeType)
		if codec == "" {
			continue
		}
		supported++
		if track.ClockRate == 0 {
			track.ClockRate = defaultClockRate(track.Kind)
		}
		b.WriteString(fmt.Sprintf("m=%s %d RTP/AVP %d\n", track.Kind, track.Port, track.PayloadType))
		if track.Kind == "audio" && track.Channels > 0 {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d/%d\n", track.PayloadType, codec, track.ClockRate, track.Channels))
		} else {
			b.WriteString(fmt.Sprintf("a=rtpmap:%d %s/%d\n", track.PayloadType, codec, track.ClockRate))
		}
		if track.FmtpLine != "" {
			b.WriteString(fmt.Sprintf("a=fmtp:%d %s\n", track.PayloadType, track.FmtpLine))
		}
		b.WriteString("a=recvonly\n")
	}
	if supported == 0 {
		return "", fmt.Errorf("ffmpeg does not support received WebRTC codecs")
	}

	return b.String(), nil
}

// sdpCodecName преобразует имя кодека в форму, требуемую SDP.
//
// @args
//   - mime (string): тип содержимого объекта.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func sdpCodecName(mime string) string {
	switch strings.ToLower(mime) {
	case "video/vp8":
		return "VP8"
	case "video/vp9":
		return "VP9"
	case "video/h264":
		return "H264"
	case "audio/opus":
		return "opus"
	case "audio/pcmu":
		return "PCMU"
	case "audio/pcma":
		return "PCMA"
	default:
		return ""
	}
}

// defaultClockRate выбирает стандартную частоту RTP-часов для типа кодека.
//
// @args
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//
// @return:
//   - результат 1 (uint32): значение, подготовленное операцией для вызывающей стороны.
func defaultClockRate(kind string) uint32 {
	if kind == "audio" {
		return 48000
	}

	return 90000
}
