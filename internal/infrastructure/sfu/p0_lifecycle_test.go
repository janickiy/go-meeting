package sfu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

// TestP0MediaLifecycle profiles repeated real Pion/UDP publish/receive/leave
// in one manager. It is opt-in because profiling changes runtime costs.
// RECORDER_P0_MEDIA_CYCLES=100 RECORDER_P0_PROFILE_DIR=/absolute/evidence/sfu
// go test -v ./internal/infrastructure/sfu -run '^TestP0MediaLifecycle$' -count=1 -timeout=10m
func TestP0MediaLifecycle(t *testing.T) {
	cycles, dir := p0MediaOptions(t)
	runtime.SetBlockProfileRate(1)
	previousMutexRate := runtime.SetMutexProfileFraction(1)
	t.Cleanup(func() {
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(previousMutexRate)
	})
	h := harness(t, 5)
	p0MediaCheckpoint(t, h, dir, "idle", 0)
	for i := 0; i < 3; i++ {
		p0MediaCycle(t, h, []int{2, 3, 5}[i], false)
	}
	warm := p0MediaCheckpoint(t, h, dir, "warmup", 0)
	for i := 1; i <= cycles; i++ {
		p0MediaCycle(t, h, []int{2, 3, 5}[(i-1)%3], i%10 == 0)
		if i%25 == 0 || i == cycles {
			current := p0MediaCheckpoint(t, h, dir, fmt.Sprintf("cycle_%03d", i), i)
			if current.Goroutines > warm.Goroutines+12 || (warm.FDs >= 0 && current.FDs > warm.FDs+8) {
				t.Fatalf("resources did not return near warm baseline: warm=%+v current=%+v", warm, current)
			}
		}
	}
	p0IdleCPU(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.manager.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	p0MediaCheckpoint(t, h, dir, "shutdown", cycles)
}

// TestP0MediaSourceChurn exercises repeated camera/microphone retirement and
// republishing plus actual screen sender add/remove without recreating peers.
func TestP0MediaSourceChurn(t *testing.T) {
	cycles, dir := p0MediaOptions(t)
	dir = filepath.Join(dir, "source-churn")
	h := harness(t, 3)
	p0MediaCheckpoint(t, h, dir, "idle", 0)
	roomID := uuid.NewString()
	a, b := h.join(t, roomID, ""), h.join(t, roomID, "")
	p0MediaReceived(t, h, []*testPeer{a, b}, 4)
	server, err := h.manager.get(a.id)
	if err != nil {
		t.Fatal(err)
	}
	a.neg.Lock()
	a.sourceDeclarations = map[string]media.Source{"microphone": media.SourceMicrophone, "camera": media.SourceCamera}
	a.neg.Unlock()
	if err := a.negotiate(); err != nil {
		t.Fatal(err)
	}
	for cycle := 1; cycle <= cycles; cycle++ {
		for _, source := range []media.Source{media.SourceCamera, media.SourceMicrophone} {
			publication := ownPublication(server, source)
			if publication == nil {
				t.Fatalf("missing publication %s", source)
			}
			if err := h.manager.Unpublish(context.Background(), a.id, publication.metadata.ID); err != nil {
				t.Fatal(err)
			}
			eventually(t, h, "source removed", func() bool {
				s := h.manager.Snapshot()
				return s.Tracks == 3 && s.Subscriptions == 3
			})
			a.neg.Lock()
			a.declarationGeneration = fmt.Sprintf("-%d-%s", cycle, source)
			a.neg.Unlock()
			if err := a.negotiate(); err != nil {
				t.Fatal(err)
			}
			p0MediaReceived(t, h, []*testPeer{a, b}, 4)
		}
		p0ScreenCycle(t, h, a, b, cycle)
		if cycle%25 == 0 || cycle == cycles {
			p0MediaCheckpoint(t, h, dir, fmt.Sprintf("active_%03d", cycle), cycle)
		}
	}
	// Continue ordinary RTP until the default 1024-packet NACK buffers have
	// filled, distinguishing bounded transport caches from retained sources.
	previous := 0
	for _, seconds := range []int{25, 30} {
		time.Sleep(time.Duration(seconds-previous) * time.Second)
		previous = seconds
		p0MediaCheckpoint(t, h, dir, fmt.Sprintf("active_steady_%02d", seconds), cycles)
	}
	a.close()
	b.close()
	p0MediaEmpty(t, h)
	p0MediaCheckpoint(t, h, dir, "after_leave", cycles)
}

func p0MediaOptions(t *testing.T) (int, string) {
	t.Helper()
	raw := os.Getenv("RECORDER_P0_MEDIA_CYCLES")
	if raw == "" {
		t.Skip("set RECORDER_P0_MEDIA_CYCLES=100 for resource lifecycle audit")
	}
	cycles, err := strconv.Atoi(raw)
	if err != nil || cycles < 1 || cycles > 1000 {
		t.Fatal("RECORDER_P0_MEDIA_CYCLES must be 1..1000")
	}
	dir := os.Getenv("RECORDER_P0_PROFILE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	return cycles, dir
}

func p0MediaCycle(t *testing.T, h *pionHarness, n int, reconnect bool) {
	t.Helper()
	roomID := uuid.NewString()
	peers := make([]*testPeer, 0, n)
	for i := 0; i < n; i++ {
		peers = append(peers, h.joinSlots(t, roomID, "", n-1))
		eventually(t, h, "publisher available", func() bool { return h.manager.Snapshot().Tracks == 2*len(peers) })
	}
	p0MediaReceived(t, h, peers, 2*n)
	if reconnect {
		participantID, oldPeerID := peers[0].binding.ParticipantID, peers[0].id
		peers[0].close()
		peers[0] = h.joinSlots(t, roomID, participantID, n-1)
		if peers[0].id == oldPeerID {
			t.Fatal("reconnect reused the old media peer")
		}
		p0MediaReceived(t, h, peers, 2*n)
	}
	for _, p := range peers {
		p.close()
	}
	p0MediaEmpty(t, h)
}

func p0MediaReceived(t *testing.T, h *pionHarness, peers []*testPeer, tracks int) {
	t.Helper()
	eventually(t, h, "current publications delivered", func() bool {
		s := h.manager.Snapshot()
		if s.Tracks != tracks || s.Subscriptions != tracks*(len(peers)-1) {
			return false
		}
		for _, p := range peers {
			current := h.manager.Tracks(p.id)
			p.mu.Lock()
			for _, track := range current {
				if p.received[track.ID] != string(track.Kind) {
					p.mu.Unlock()
					return false
				}
			}
			p.mu.Unlock()
		}
		return true
	})
}

func p0MediaEmpty(t *testing.T, h *pionHarness) {
	t.Helper()
	eventually(t, h, "all media resources removed", func() bool {
		s := h.manager.Snapshot()
		if s.Rooms != 0 || s.Peers != 0 || s.Tracks != 0 || s.Subscriptions != 0 || s.RecordingOutputs != 0 || s.AudioTaps != 0 {
			return false
		}
		h.manager.mu.Lock()
		empty := len(h.manager.connections) == 0 && len(h.manager.roomClosures) == 0
		h.manager.mu.Unlock()
		return empty
	})
}

func p0ScreenCycle(t *testing.T, h *pionHarness, a, b *testPeer, cycle int) {
	t.Helper()
	name := fmt.Sprintf("screen-%d", cycle)
	track, err := pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, name, "display")
	if err != nil {
		t.Fatal(err)
	}
	a.neg.Lock()
	a.sourceDeclarations[name] = media.SourceVideoScreen
	sender, err := a.pc.AddTrack(track)
	a.neg.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		for {
			if _, _, err := sender.ReadRTCP(); err != nil {
				return
			}
		}
	}()
	if err := a.negotiate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(a.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		var sequence uint16
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sequence++
				_ = track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: sequence, Timestamp: uint32(sequence) * 1800, Marker: true}, Payload: []byte{0x10, 1, 2, 3}})
			}
		}
	}()
	defer func() { cancel(); <-done }()
	p0MediaReceived(t, h, []*testPeer{a, b}, 5)
	a.neg.Lock()
	err = a.pc.RemoveTrack(sender)
	delete(a.sourceDeclarations, name)
	a.neg.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.negotiate(); err != nil {
		t.Fatal(err)
	}
	p0MediaReceived(t, h, []*testPeer{a, b}, 4)
}

type p0MediaSample struct {
	Phase       string    `json:"phase"`
	Cycle       int       `json:"cycle"`
	At          time.Time `json:"at"`
	Goroutines  int       `json:"goroutines"`
	HeapBytes   uint64    `json:"heapBytes"`
	HeapObjects uint64    `json:"heapObjects"`
	RSSBytes    int64     `json:"rssBytes"`
	FDs         int       `json:"fds"`
	Stats       Stats     `json:"stats"`
}

func p0MediaCheckpoint(t *testing.T, h *pionHarness, dir, phase string, cycle int) p0MediaSample {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	time.Sleep(250 * time.Millisecond)
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	sample := p0MediaSample{Phase: phase, Cycle: cycle, At: time.Now().UTC(), Goroutines: runtime.NumGoroutine(), HeapBytes: mem.HeapAlloc, HeapObjects: mem.HeapObjects, FDs: p0MediaFDs(), Stats: h.manager.Snapshot(), RSSBytes: -1}
	if output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output(); err == nil {
		if kib, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
			sample.RSSBytes = kib * 1024
		}
	}
	encoded, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, phase+".json"), append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"heap", "allocs", "goroutine", "mutex", "block"} {
		f, err := os.Create(filepath.Join(dir, phase+"."+kind+".pprof"))
		if err != nil {
			t.Fatal(err)
		}
		err = pprof.Lookup(kind).WriteTo(f, 0)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("profile %s: write=%v close=%v", kind, err, closeErr)
		}
	}
	t.Logf("P0_MEDIA phase=%s cycle=%d goroutines=%d heap_bytes=%d heap_objects=%d rss_bytes=%d fd=%d rooms=%d peers=%d tracks=%d subscriptions=%d packets=%d", phase, cycle, sample.Goroutines, sample.HeapBytes, sample.HeapObjects, sample.RSSBytes, sample.FDs, sample.Stats.Rooms, sample.Stats.Peers, sample.Stats.Tracks, sample.Stats.Subscriptions, sample.Stats.Packets)
	return sample
}

func p0IdleCPU(t *testing.T, dir string) {
	t.Helper()
	file, err := os.Create(filepath.Join(dir, "idle_after_load.cpu.pprof"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	var before, after syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &before)
	started := time.Now()
	time.Sleep(2 * time.Second)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &after)
	pprof.StopCPUProfile()
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("P0_MEDIA_IDLE_CPU elapsed=%s cpu_seconds=%.6f", time.Since(started), processCPU(after)-processCPU(before))
}

// /dev/fd cannot be enumerated on every macOS runtime. Count numeric descriptor
// records from lsof as a fallback; its bounded subprocess is included equally
// in each checkpoint. cwd/txt/memory mappings are not file descriptors.
func p0MediaFDs() int {
	if count := openFDs(); count >= 0 {
		return count
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "lsof", "-nP", "-a", "-p", strconv.Itoa(os.Getpid()), "-Ff").Output()
	if err != nil {
		return -1
	}
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) > 1 && line[0] == 'f' {
			if _, err := strconv.Atoi(line[1:]); err == nil {
				count++
			}
		}
	}
	return count
}
