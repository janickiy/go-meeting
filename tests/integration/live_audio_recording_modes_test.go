package integration_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	captions "github.com/janickiy/go-recorder/internal/domain/captions"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/infrastructure/composite"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/janickiy/go-recorder/internal/infrastructure/liveproviders"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// stageEightFFmpeg находит локальный кодировщик для реальных медиафикстур без сетевых ресурсов.
// @args t — исполнитель теста.
// @return абсолютный путь либо пропуск при отсутствии зависимости.
func stageEightFFmpeg(t *testing.T) string {
	t.Helper()
	binary := os.Getenv("RECORDER_TEST_FFMPEG")
	if binary == "" {
		binary = "ffmpeg"
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		t.Skip("FFmpeg required for media verification")
	}
	return path
}

// TestStageEightOpusToLiveCaptions проверяет настоящий Opus→PCM→fake STT, включая финал короткой реплики.
// @args t — исполнитель; исходный звук — синусоида, не персональное аудио.
func TestStageEightOpusToLiveCaptions(t *testing.T) {
	binary := stageEightFFmpeg(t)
	fixture := encodedFixture(t, binary)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	provider := liveproviders.Provider{Mode: "mock", Timeout: time.Second}
	session, err := provider.StartSession(ctx, captions.SessionConfig{SessionID: uuid.NewString(), TrackInstanceID: uuid.NewString(), Language: "en", SampleRate: 16000, Channels: 1, Format: "pcm_s16le"})
	if err != nil {
		t.Fatal(err)
	}
	packets := make(chan media.EgressFrame, len(fixture.audio))
	for i, payload := range fixture.audio {
		raw, _ := (&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: uint16(i), Timestamp: uint32(i * 960), SSRC: 1}, Payload: payload}).Marshal()
		packets <- media.EgressFrame{RTP: raw}
	}
	close(packets)
	done := make(chan []captions.Event, 1)
	go func() {
		var events []captions.Event
		for event := range session.Events() {
			events = append(events, event)
		}
		done <- events
	}()
	var samples, active int
	err = (ffmpeg.LiveAudio{Binary: binary}).Decode(ctx, media.EgressTrack{MimeType: "audio/opus", ClockRate: 48000, Channels: 2}, packets, func(pcm []byte) error {
		samples += len(pcm) / 2
		if ffmpeg.AudioActive(pcm) {
			active++
		}
		return session.WriteAudio(ctx, pcm)
	})
	closeErr := session.Close()
	events := <-done
	if err != nil || closeErr != nil || samples < 30000 || samples > 34000 || active < 90 {
		t.Fatalf("decode samples=%d active=%d err=%v close=%v", samples, active, err, closeErr)
	}
	partial, final := 0, 0
	for _, event := range events {
		if event.Language != "en" {
			t.Fatal("language lost")
		}
		if event.Final {
			final++
		} else {
			partial++
		}
	}
	if partial < 1 || final < 2 {
		t.Fatalf("partial=%d final=%d", partial, final)
	}
	if ffmpeg.AudioActive(make([]byte, 640)) {
		t.Fatal("silence counted")
	}
}

// TestStageEightRecordingModes проверяет захват RTP, восстановление, AAC/H264 и архив incarnation дорожек.
// @args t — исполнитель реальных FFmpeg/ffprobe проверок всех стратегий.
func TestStageEightRecordingModes(t *testing.T) {
	binary := stageEightFFmpeg(t)
	fixture := encodedFixture(t, binary)
	for _, mode := range []string{"composite", "audio_only", "individual_tracks", "screen_focus"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			dir := t.TempDir()
			var input bytes.Buffer
			encoder := json.NewEncoder(&input)
			var sequence uint64
			epoch := time.Now().Add(-2 * time.Second).UnixNano()
			emit := func(frame media.EgressFrame, ms int) {
				sequence++
				frame.Sequence = sequence
				frame.CapturedAt = epoch + int64(ms)*int64(time.Millisecond)
				if err := encoder.Encode(frame); err != nil {
					t.Fatal(err)
				}
			}
			pid := uuid.NewString()
			audioID, replacement, videoID := uuid.NewString(), uuid.NewString(), uuid.NewString()
			audio := media.EgressTrack{Track: media.Track{ID: audioID, ParticipantID: pid, Kind: media.KindAudio, Source: media.SourceMicrophone}, MimeType: "audio/opus", ClockRate: 48000, Channels: 2, PayloadType: 111, SSRC: 1}
			video := media.EgressTrack{Track: media.Track{ID: videoID, ParticipantID: pid, Kind: media.KindVideo, Source: media.SourceVideoScreen}, MimeType: "video/VP8", ClockRate: 90000, PayloadType: 96, SSRC: 2}
			emit(media.EgressFrame{Type: "hello"}, 0)
			emit(media.EgressFrame{Type: "track", TrackID: audioID, Track: &audio}, 0)
			emit(media.EgressFrame{Type: "track", TrackID: videoID, Track: &video}, 0)
			payloader := &codecs.VP8Payloader{}
			var videoSeq uint16
			for i := 0; i < 100; i++ {
				if i == 50 {
					emit(media.EgressFrame{Type: "track.end", TrackID: audioID}, 1000)
					audio.ID = replacement
					emit(media.EgressFrame{Type: "track", TrackID: replacement, Track: &audio}, 1000)
				}
				raw, _ := (&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: uint16(i), Timestamp: uint32(i * 960), SSRC: 1}, Payload: fixture.audio[i]}).Marshal()
				emit(media.EgressFrame{Type: "rtp", TrackID: audio.ID, RTP: raw}, i*20)
				if i%2 == 0 {
					parts := payloader.Payload(1200, fixture.video[i/2])
					for n, part := range parts {
						videoSeq++
						raw, _ := (&rtp.Packet{Header: rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: videoSeq, Timestamp: uint32(i / 2 * 3600), SSRC: 2, Marker: n == len(parts)-1}, Payload: part}).Marshal()
						emit(media.EgressFrame{Type: "rtp", TrackID: videoID, RTP: raw}, i*20)
					}
				}
			}
			emit(media.EgressFrame{Type: "track.end", TrackID: audio.ID}, 2000)
			emit(media.EgressFrame{Type: "track.end", TrackID: videoID}, 2000)
			composer := composite.NewComposer(binary, 320, 240, 25, 1)
			err := composer.Capture(ctx, ctx, &input, dir, composite.CaptureOptions{Mode: mode, SegmentDuration: time.Second, MaxBytes: 10 << 20})
			if err != nil && !composite.OnlyEgressEnded(err) {
				t.Fatal(err)
			}
			if err = composer.Recover(ctx, dir); err != nil {
				t.Fatal("recovery", err)
			}
			post := ffmpeg.NewPostProcessor(binary)
			var result ffmpeg.Result
			if mode == "audio_only" || mode == "individual_tracks" {
				result, err = post.FinalizeAudio(ctx, dir)
			} else {
				result, err = post.FinalizeComposite(ctx, dir)
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.FinalSizeBytes < 1000 || result.DurationSec < 1 || result.DurationSec > 3 {
				t.Fatalf("bad output %+v", result)
			}
			probe := strings.Replace(binary, "ffmpeg", "ffprobe", 1)
			out, err := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "stream=codec_name,codec_type", "-of", "json", result.FinalPath).Output()
			if err != nil {
				t.Fatal(err)
			}
			audioOnly := mode == "audio_only" || mode == "individual_tracks"
			if !bytes.Contains(out, []byte("aac")) || bytes.Contains(out, []byte("video")) == audioOnly {
				t.Fatalf("wrong codecs %s", out)
			}
			origin, err := composite.TimelineOrigin(dir)
			if err != nil || origin != epoch {
				t.Fatal("timeline origin", origin, err)
			}
			if mode == "individual_tracks" {
				path, _, _, err := composite.ArchiveTracks(ctx, dir, 10<<20)
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(path)
				archive, err := zip.OpenReader(path)
				if err != nil {
					t.Fatal(err)
				}
				defer archive.Close()
				var manifest composite.TrackManifest
				for _, file := range archive.File {
					if file.Name == "manifest.json" {
						r, e := file.Open()
						if e != nil {
							t.Fatal(e)
						}
						raw, e := io.ReadAll(r)
						r.Close()
						if e != nil {
							t.Fatal(e)
						}
						if e = json.Unmarshal(raw, &manifest); e != nil {
							t.Fatal(e)
						}
					}
				}
				ids := map[string]bool{}
				for _, fragment := range manifest.Fragments {
					ids[fragment.TrackInstanceID] = true
					if fragment.ParticipantID != pid || fragment.EndMS < fragment.StartMS {
						t.Fatal("bad track metadata")
					}
				}
				if !ids[audioID] || !ids[replacement] || !ids[videoID] {
					t.Fatal("replacement overwrote track", ids)
				}
			}
			t.Logf("mode=%s duration=%ds bytes=%d segments=%d", mode, result.DurationSec, result.FinalSizeBytes, len(result.Segments))
		})
	}
}
