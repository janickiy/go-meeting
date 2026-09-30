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
	"syscall"
	"time"
)

// RTPTrack описывает RTP-трек, который FFmpeg будет принимать через UDP.
type RTPTrack struct {
	Kind        string
	MimeType    string
	PayloadType uint8
	ClockRate   uint32
	Channels    uint16
	Port        int
	FmtpLine    string
}

// SegmentRecorder запускает FFmpeg для записи RTP в segment_*.mkv.
type SegmentRecorder struct {
	path   string
	logger *log.Logger
}

// SegmentProcess хранит процесс FFmpeg записи сегментов.
type SegmentProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan error
	stderr logTail
	stdin  io.WriteCloser
	logger *log.Logger
}

// NewSegmentRecorder создает recorder FFmpeg-сегментов.
// Параметры:
// - path: путь к ffmpeg.
// - logger: logger worker-а.
// Возвращает: SegmentRecorder.
func NewSegmentRecorder(path string, logger *log.Logger) *SegmentRecorder {
	if path == "" {
		path = "ffmpeg"
	}
	if logger == nil {
		logger = log.Default()
	}

	return &SegmentRecorder{path: path, logger: logger}
}

// Start создает SDP и запускает FFmpeg segment muxer.
// Параметры:
// - ctx: context записи.
// - recordID: UUID записи.
// - tracks: входящие RTP-треки.
// - workDir: директория для input.sdp.
// - outputDir: директория segment_*.mkv.
// - segmentDurationSec: длительность сегмента.
// Возвращает: процесс FFmpeg или ошибку запуска.
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
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdin: %w", err)
	}
	process := &SegmentProcess{
		cmd:    cmd,
		cancel: cancel,
		done:   make(chan error, 1),
		stdin:  stdin,
		logger: r.logger,
	}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start ffmpeg segment recorder: %w", err)
	}
	go func() {
		process.done <- cmd.Wait()
	}()

	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-process.done:
		cancel()
		return nil, fmt.Errorf("ffmpeg exited during startup: %w; stderr=%s", err, process.Stderr())
	case <-timer.C:
		r.logger.Printf("record %s ffmpeg segment recorder started", recordID)
		return process, nil
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

// Stop мягко завершает FFmpeg, чтобы segment muxer закрыл текущий файл.
// Параметры:
// - timeout: сколько ждать graceful stop.
// Возвращает: nil; незавершенный хвостовой сегмент отфильтрует post-processing.
func (p *SegmentProcess) Stop(timeout time.Duration) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
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
	case err := <-p.done:
		p.cancel()
		if err != nil && p.logger != nil {
			p.logger.Printf("ffmpeg stopped with error: %v; stderr=%s", err, p.Stderr())
		}
		return nil
	case <-timer.C:
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-p.done:
			p.cancel()
			if err != nil && p.logger != nil {
				p.logger.Printf("ffmpeg stopped after SIGTERM: %v; stderr=%s", err, p.Stderr())
			}
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			p.cancel()
			select {
			case err := <-p.done:
				if err != nil && p.logger != nil {
					p.logger.Printf("ffmpeg killed after stop timeout: %v; stderr=%s", err, p.Stderr())
				}
			case <-time.After(2 * time.Second):
				if p.logger != nil {
					p.logger.Printf("ffmpeg kill wait timeout; stderr=%s", p.Stderr())
				}
			}
		}
		return nil
	}
}

// Stderr возвращает stderr FFmpeg.
// Параметры: нет.
// Возвращает: строку stderr без крайних пробелов.
func (p *SegmentProcess) Stderr() string {
	if p == nil {
		return ""
	}

	return strings.TrimSpace(p.stderr.String())
}

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

func defaultClockRate(kind string) uint32 {
	if kind == "audio" {
		return 48000
	}

	return 90000
}
