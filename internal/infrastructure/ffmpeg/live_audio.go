package ffmpeg

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
	"syscall"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/operations"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// LiveAudio декодирует одну дорожку Opus в отдельном процессе с одним потоком CPU.
// Конкурентность задаёт общий пул live-worker, а не неограниченное число горутин издателей.
type LiveAudio struct{ Binary string }

// Decode читает ограниченную очередь RTP и передаёт порции 20 ms PCM16LE mono/16k.
// @args ctx — срок жизни дорожки; track — доверенные метаданные кодека; packets — ограниченный поток перехваченных пакетов;
// pcm — обработчик декодированных отсчётов вне SFU; учитывает отмену ctx.
// @return ошибка декодирования/отмены; исходные аудиобайты на диск не записываются.
func (d LiveAudio) Decode(ctx context.Context, track media.EgressTrack, packets <-chan media.EgressFrame, pcm func([]byte) error) error {
	if track.MimeType != "audio/opus" || track.ClockRate != 48000 || track.Channels < 1 || track.Channels > 2 {
		return fmt.Errorf("unsupported live audio codec")
	}
	op, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(op, d.Binary, "-hide_banner", "-loglevel", "error", "-nostdin", "-threads", "1", "-probesize", "4096", "-analyzeduration", "0", "-f", "ogg", "-i", "pipe:0", "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-threads", "1", "-f", "s16le", "-flush_packets", "1", "pipe:1")
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = time.Second
	input, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	started := time.Now()
	defer func() {
		operations.Observe("live_decode", time.Since(started).Seconds())
		if cmd.ProcessState != nil {
			operations.Observe("live_decode_cpu", (cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime()).Seconds())
		}
	}()
	readDone := make(chan error, 1)
	go func() {
		var result error
		defer func() {
			readDone <- result
			if result != nil {
				cancel()
			}
		}()
		buffer := make([]byte, 640)
		for {
			n, e := io.ReadFull(output, buffer)
			if n > 0 {
				if result = pcm(buffer[:n]); result != nil {
					return
				}
			}
			if e != nil {
				if e != io.EOF && e != io.ErrUnexpectedEOF {
					result = e
				}
				return
			}
		}
	}()
	writer, err := oggwriter.NewWith(input, 48000, track.Channels)
	if err != nil {
		cancel()
		input.Close()
		<-readDone
		cmd.Wait()
		return err
	}
	builder := samplebuilder.New(10, &codecs.OpusPacket{}, 48000)
	var stamp uint32
	var initialized bool
	writeSample := func() error {
		for sample := builder.Pop(); sample != nil; sample = builder.Pop() {
			if err := writer.WriteRTP(&rtp.Packet{Header: rtp.Header{Timestamp: stamp}, Payload: sample.Data}); err != nil {
				return err
			}
			stamp += uint32(sample.Duration.Seconds() * 48000)
		}
		return nil
	}
loop:
	for {
		select {
		case <-op.Done():
			break loop
		case frame, ok := <-packets:
			if !ok {
				builder.Flush()
				err = writeSample()
				break loop
			}
			var packet rtp.Packet
			if packet.Unmarshal(frame.RTP) != nil {
				continue
			}
			if !initialized {
				stamp = packet.Timestamp
				initialized = true
			}
			builder.Push(&packet)
			if err = writeSample(); err != nil {
				break loop
			}
		}
	}
	_ = writer.Close()
	_ = input.Close()
	if err != nil {
		cancel()
	}
	readErr := <-readDone
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if readErr != nil {
		return readErr
	}
	// Нормальный EOF не отменяет процесс: ошибка кода завершения остаётся наблюдаемой.
	if waitErr != nil {
		return fmt.Errorf("live decoder failed")
	}
	return nil
}

// AudioActive оценивает энергию PCM, не выдавая открытый микрофон за произнесённую речь.
// @args pcm — mono signed PCM16LE.
// @return true при RMS выше -42 dBFS; шум и музыка могут давать ложное срабатывание.
func AudioActive(pcm []byte) bool {
	var sum float64
	n := len(pcm) / 2
	if n == 0 {
		return false
	}
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:i+2]))) / 32768
		sum += v * v
	}
	return math.Sqrt(sum/float64(n)) >= 0.007943282
}
