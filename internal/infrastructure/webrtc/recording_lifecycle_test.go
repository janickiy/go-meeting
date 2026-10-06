package webrtc

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	pion "github.com/pion/webrtc/v4"
)

// TestStoppedRecordingRejectsLateOffer models an offer already holding the
// session pointer when Stop removes it from the manager's registry.
func TestStoppedRecordingRejectsLateOffer(t *testing.T) {
	m, err := NewManager(Options{StoragePath: t.TempDir(), Logger: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	if err := m.Prepare("stopped-recording", 1); err != nil {
		t.Fatal(err)
	}
	s, err := m.session("stopped-recording")
	if err != nil {
		t.Fatal(err)
	}
	old, err := m.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	s.pc = old
	// An already terminated recorder is still represented by a non-nil process.
	// Stop is a no-op for this process; no external process is needed for this race.
	s.process = &ffmpeg.SegmentProcess{}
	if err := m.Stop("stopped-recording"); err != nil {
		t.Fatal(err)
	}
	if m.Active() != 0 || s.ctx.Err() == nil || old.ConnectionState() != pion.PeerConnectionStateClosed {
		t.Fatal("stop did not reach its terminal state")
	}
	remote, err := m.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	if _, err := remote.AddTransceiverFromKind(pion.RTPCodecTypeAudio); err != nil {
		t.Fatal(err)
	}
	offer, err := remote.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = s.handleOffer(ctx, records.WebRTCOfferRequest{Type: "offer", SDP: offer.SDP})
	s.mu.Lock()
	late, timer := s.pc, s.waitTimer
	s.mu.Unlock()
	if late != nil {
		defer late.Close()
	}
	if timer != nil {
		defer timer.Stop()
	}
	t.Logf("late_offer_error=%v stopped_context=%v manager_active=%d new_peer_created=%v", err, s.ctx.Err(), m.Active(), late != old)
	if err == nil || (late != nil && late != old && late.ConnectionState() != pion.PeerConnectionStateClosed) {
		t.Fatal("stopped recording admitted an unregistered live PeerConnection")
	}
}

func TestCancelledRecordingDoesNotArmTrackTimer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &session{ctx: ctx, cancel: cancel, manager: &Manager{logger: log.New(io.Discard, "", 0)}}
	s.armTrackWaitTimeout(time.Hour)
	s.mu.Lock()
	timer := s.waitTimer
	s.mu.Unlock()
	if timer != nil {
		timer.Stop()
		t.Fatal("cancelled recording retained a track-wait timer")
	}
}

// TestUnexpectedRecorderExitReleasesSession verifies a process failing after the
// successful startup window does not retain a recording slot and PeerConnection.
func TestUnexpectedRecorderExitReleasesSession(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "late-failure")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nsleep 0.5\nexit 23\n"), 0700); err != nil {
		t.Fatal(err)
	}
	failures := make(chan error, 1)
	m, err := NewManager(Options{StoragePath: t.TempDir(), FFmpegPath: binary, Logger: log.New(io.Discard, "", 0), OnFailed: func(_ context.Context, _ string, err error) { failures <- err }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	if err := m.Prepare("late-failed-recording", 1); err != nil {
		t.Fatal(err)
	}
	s, err := m.session("late-failed-recording")
	if err != nil {
		t.Fatal(err)
	}
	pc, err := m.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	s.pc = pc
	s.tracks = []ffmpeg.RTPTrack{{Kind: "audio", MimeType: "audio/opus", ClockRate: 48000, Channels: 2, PayloadType: 111, Port: 39001}}
	if err := s.startFFmpeg(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-failures:
		if err == nil || m.Active() != 0 || s.ctx.Err() == nil || pc.ConnectionState() != pion.PeerConnectionStateClosed {
			t.Fatal("recorder exit did not close the owning media session")
		}
	case <-time.After(1500 * time.Millisecond):
		t.Logf("manager_active=%d peer_state=%s session_context=%v", m.Active(), pc.ConnectionState(), s.ctx.Err())
		t.Fatal("unexpected FFmpeg exit retained recording resources and worker slot")
	}
}

func TestStartedCallbackDoesNotBlockRecordingStop(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "graceful-recorder")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nread command\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	entered, returned := make(chan struct{}), make(chan struct{})
	m, err := NewManager(Options{StoragePath: t.TempDir(), FFmpegPath: binary, Logger: log.New(io.Discard, "", 0), OnStarted: func(ctx context.Context, _ string) { close(entered); <-ctx.Done(); close(returned) }})
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Prepare("blocked-start-callback", 1); err != nil {
		t.Fatal(err)
	}
	s, err := m.session("blocked-start-callback")
	if err != nil {
		t.Fatal(err)
	}
	s.tracks = []ffmpeg.RTPTrack{{Kind: "audio", MimeType: "audio/opus", ClockRate: 48000, Channels: 2, PayloadType: 111, Port: 39005}}
	started := make(chan error, 1)
	go func() { started <- s.startFFmpeg() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("start callback did not run")
	}
	s.mu.Lock()
	s.processSince = time.Now().Add(-10 * time.Second)
	s.mu.Unlock()
	stopped := make(chan error, 1)
	go func() { stopped <- m.Stop("blocked-start-callback") }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("DB callback retained the session lock")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel callback context")
	}
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("start callback retained goroutine")
	}
}

// The recorder logs after its startup timer, immediately before returning the
// process to the session. Hold that handoff without relying on a scheduling gap.
type recorderStartupBarrier struct {
	reached, released chan struct{}
	notify, resume    sync.Once
}

func (b *recorderStartupBarrier) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "ffmpeg segment recorder started") {
		b.notify.Do(func() { close(b.reached) })
		<-b.released
	}
	return len(data), nil
}
func (b *recorderStartupBarrier) Release() { b.resume.Do(func() { close(b.released) }) }

func TestStopAndShutdownJoinInFlightRecorderStartup(t *testing.T) {
	for _, action := range []string{"stop", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			root := t.TempDir()
			pidPath := filepath.Join(root, "child.pid")
			t.Setenv("RECORDER_P0_CHILD_PID", pidPath)
			binary := filepath.Join(root, "startup-recorder")
			script := "#!/bin/sh\nprintf '%s' \"$$\" > \"$RECORDER_P0_CHILD_PID\"\nread command\nexit 0\n"
			if action == "shutdown" {
				script = "#!/bin/sh\ntrap '' TERM\nprintf '%s' \"$$\" > \"$RECORDER_P0_CHILD_PID\"\nwhile :; do read command; done\n"
			}
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			barrier := &recorderStartupBarrier{reached: make(chan struct{}), released: make(chan struct{})}
			t.Cleanup(barrier.Release)
			m, err := NewManager(Options{StoragePath: root, FFmpegPath: binary, Logger: log.New(barrier, "", 0)})
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Prepare("inflight-start", 1); err != nil {
				t.Fatal(err)
			}
			s, err := m.session("inflight-start")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				barrier.Release()
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				_ = m.Shutdown(ctx)
			})
			s.tracks = []ffmpeg.RTPTrack{{Kind: "audio", MimeType: "audio/opus", ClockRate: 48000, Channels: 2, PayloadType: 111, Port: 39007}}
			started := make(chan error, 1)
			go func() { started <- s.startFFmpeg() }()
			select {
			case <-barrier.reached:
			case err := <-started:
				t.Fatalf("startup returned before the handoff barrier: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("recorder startup did not reach the handoff barrier")
			}
			var pid int
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				data, err := os.ReadFile(pidPath)
				if err == nil {
					pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
					if pid > 0 {
						break
					}
				}
				time.Sleep(time.Millisecond)
			}
			if pid == 0 {
				t.Fatal("child did not start")
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL); s.cancel() })
			s.mu.Lock()
			pending := s.process == nil
			s.mu.Unlock()
			if !pending {
				t.Fatal("startup crossed the held handoff barrier")
			}
			completed := make(chan error, 1)
			go func() {
				if action == "stop" {
					completed <- m.Stop("inflight-start")
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				completed <- m.Shutdown(ctx)
			}()
			// Observe that the lifecycle operation entered while startup still owns
			// the real child. Both flags remain stable until the barrier is released.
			entered := false
			deadline = time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				s.mu.Lock()
				entered = (action == "stop" && s.stopRequested) || (action == "shutdown" && s.stopping && s.ctx.Err() != nil)
				s.mu.Unlock()
				if entered {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if !entered {
				t.Fatal("lifecycle operation did not enter during the held startup")
			}
			barrier.Release()
			select {
			case err = <-completed:
				if err != nil {
					t.Fatalf("%s did not join accepted startup: %v", action, err)
				}
			case <-time.After(6 * time.Second):
				t.Fatalf("%s retained the startup worker", action)
			}
			if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
				t.Fatalf("%s returned before process Wait: pid=%d error=%v", action, pid, err)
			}
			select {
			case err := <-started:
				if action == "stop" && err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("start worker retained")
			}
			if m.Active() != 0 {
				t.Fatal("in-flight startup retained manager slot")
			}
		})
	}
}
