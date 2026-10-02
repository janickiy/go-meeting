package sfu

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestStageEightAudioTapIsolation моделирует зависший STT consumer рядом с исправной записью.
// @args t — исполнитель; отдельная audio очередь должна отказать, не влияя на RTP или recorder.
func TestStageEightAudioTapIsolation(t *testing.T) {
	sizes := []int{2}
	if os.Getenv("RECORDER_MEDIA_LOAD") == "true" {
		sizes = []int{2, 5, 10}
	}
	for _, n := range sizes {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			h := harness(t, n, Options{MaxRooms: 2, NegotiationTimeout: 20 * time.Second, EgressQueueSize: 256})
			runtime.GC()
			var before, during runtime.MemStats
			runtime.ReadMemStats(&before)
			var c0, c1 syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &c0)
			started, g0 := time.Now(), runtime.NumGoroutine()
			peers := []*testPeer{}
			taps := []media.EgressSubscription{}
			recordings := []media.EgressSubscription{}
			var readers sync.WaitGroup
			for r := 0; r < 2; r++ {
				room := uuid.NewString()
				for i := 0; i < n; i++ {
					peers = append(peers, h.joinSlots(t, room, "", n-1))
					eventually(t, h, "tracks", func() bool { return h.manager.Snapshot().Tracks == 2*len(peers) })
				}
				record, err := h.manager.SubscribeRecording(context.Background(), room, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				recordings = append(recordings, record)
				readers.Add(1)
				go func() {
					defer readers.Done()
					for {
						select {
						case frame := <-record.Frames():
							if frame.Type == "" {
								return
							}
						case <-record.Done():
							return
						}
					}
				}()
				tap, err := h.manager.SubscribeAudio(context.Background(), room, uuid.NewString())
				if err != nil {
					t.Fatal(err)
				}
				taps = append(taps, tap)
				if _, err = h.manager.SubscribeAudio(context.Background(), room, uuid.NewString()); err == nil {
					t.Fatal("duplicate STT tap")
				}
			}
			eventually(t, h, "all subscriptions", func() bool { return h.manager.Snapshot().Subscriptions == 4*n*(n-1) })
			for _, tap := range taps {
				select {
				case <-tap.Done():
				case <-time.After(4 * time.Second):
					t.Fatal("stalled tap remained unbounded")
				}
				if tap.Err() == nil {
					t.Fatal("silent overflow")
				}
			}
			packets := h.manager.Snapshot().Packets
			time.Sleep(500 * time.Millisecond)
			state := h.manager.Snapshot()
			runtime.ReadMemStats(&during)
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &c1)
			if state.Packets <= packets || state.AudioTapDrops != 2 || state.RecordingDrops != 0 || state.Peers != n*2 || state.Dropped != 0 {
				t.Fatalf("auxiliary affected media %+v", state)
			}
			t.Logf("LIVE_STALLED rooms=2 participants=%d elapsed=%s cpu=%.3f heap=%d->%d goroutines=%d->%d packets=%d dropped=%d audioTapDrops=%d", n, time.Since(started).Round(time.Millisecond), processCPU(c1)-processCPU(c0), before.HeapAlloc, during.HeapAlloc, g0, runtime.NumGoroutine(), state.Packets, state.Dropped, state.AudioTapDrops)
			for _, tap := range taps {
				tap.Close()
			}
			for _, record := range recordings {
				record.Close()
			}
			readers.Wait()
			for _, peer := range peers {
				peer.close()
			}
			eventually(t, h, "cleanup", func() bool { return h.manager.Snapshot().Rooms == 0 })
		})
	}
}
