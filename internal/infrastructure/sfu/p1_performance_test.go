package sfu

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

// TestP1MediaPerformance uses real Pion clients and SFU in the same process.
// CPU/alloc totals include client transports; profiles distinguish SFU call sites.
// Run with RECORDER_P1_MEDIA=true and RECORDER_P1_PROFILE_DIR=/absolute/path.
func TestP1MediaPerformance(t *testing.T) {
	if os.Getenv("RECORDER_P1_MEDIA") != "true" {
		t.Skip("opt-in media performance profiles")
	}
	duration := 5 * time.Second
	if raw := os.Getenv("RECORDER_P1_MEDIA_SECONDS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 60 {
			t.Fatal("RECORDER_P1_MEDIA_SECONDS must be 1..60")
		}
		duration = time.Duration(n) * time.Second
	}
	base := os.Getenv("RECORDER_P1_PROFILE_DIR")
	if base == "" {
		base = t.TempDir()
	}
	for _, scenario := range []struct {
		name                string
		participants, rooms int
		screen, egress      bool
	}{
		{"idle", 0, 1, false, false},
		{"2_peers", 2, 1, false, false},
		{"5_peers", 5, 1, false, false},
		{"10_peers", 10, 1, false, false},
		{"5_peers_2_rooms", 5, 2, false, false},
		{"10_peers_2_rooms", 10, 2, false, false},
		{"5_peers_screen_recording_audio_tap", 5, 1, true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := filepath.Join(base, scenario.name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			h := harness(t, max(3, scenario.participants), Options{MaxRooms: scenario.rooms, NegotiationTimeout: 20 * time.Second})
			h.audioPayloadBytes, h.videoPayloadBytes = 160, 1200
			peers := make([]*testPeer, 0, scenario.participants*scenario.rooms)
			var streams []media.EgressSubscription
			var readers sync.WaitGroup
			t.Cleanup(func() {
				for _, stream := range streams {
					stream.Close()
				}
				readers.Wait()
			})
			for room := 0; room < scenario.rooms && scenario.participants > 0; room++ {
				id := uuid.NewString()
				var roomPeers []*testPeer
				for n := 0; n < scenario.participants; n++ {
					peer := h.joinSlots(t, id, "", scenario.participants-1)
					roomPeers = append(roomPeers, peer)
					peers = append(peers, peer)
					eventually(t, h, "all publishers", func() bool { return h.manager.Snapshot().Tracks == 2*len(peers) })
				}
				if scenario.screen {
					p1StartScreen(t, h, roomPeers[0])
				}
				if scenario.egress {
					recording, err := h.manager.SubscribeRecording(context.Background(), id, uuid.NewString())
					if err != nil {
						t.Fatal(err)
					}
					audio, err := h.manager.SubscribeAudio(context.Background(), id, uuid.NewString())
					if err != nil {
						recording.Close()
						t.Fatal(err)
					}
					for _, stream := range []media.EgressSubscription{recording, audio} {
						streams = append(streams, stream)
						readers.Add(1)
						go func(stream media.EgressSubscription) {
							defer readers.Done()
							for {
								select {
								case <-stream.Done():
									return
								case <-stream.Frames():
								}
							}
						}(stream)
					}
				}
			}
			expectedTracks := 2 * len(peers)
			if scenario.screen {
				expectedTracks += scenario.rooms
			}
			if len(peers) > 0 {
				eventually(t, h, "all current RTP delivered", func() bool {
					s := h.manager.Snapshot()
					if s.Tracks != expectedTracks || s.Subscriptions != expectedTracks*(scenario.participants-1) {
						return false
					}
					for _, peer := range peers {
						tracks := h.manager.Tracks(peer.id)
						peer.mu.Lock()
						for _, track := range tracks {
							if peer.received[track.ID] == "" {
								peer.mu.Unlock()
								return false
							}
						}
						peer.mu.Unlock()
					}
					return true
				})
				// Each default Pion NACK buffer is bounded at 1024 RTP packets.
				// At 50 packets/second, wait long enough to measure steady buffers.
				time.Sleep(22 * time.Second)
			}
			runtime.SetBlockProfileRate(1)
			previousMutex := runtime.SetMutexProfileFraction(1)
			defer runtime.SetBlockProfileRate(0)
			defer runtime.SetMutexProfileFraction(previousMutex)
			p0MediaCheckpoint(t, h, dir, "before", 0)
			var before, after runtime.MemStats
			var cpuBefore, cpuAfter syscall.Rusage
			cpu, err := os.Create(filepath.Join(dir, "cpu.pprof"))
			if err != nil {
				t.Fatal(err)
			}
			if err := pprof.StartCPUProfile(cpu); err != nil {
				_ = cpu.Close()
				t.Fatal(err)
			}
			runtime.ReadMemStats(&before)
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuBefore)
			startStats := h.manager.Snapshot()
			started := time.Now()
			time.Sleep(duration)
			runtime.ReadMemStats(&after)
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuAfter)
			endStats := h.manager.Snapshot()
			elapsed := time.Since(started)
			pprof.StopCPUProfile()
			if err := cpu.Close(); err != nil {
				t.Fatal(err)
			}
			metrics := map[string]any{
				"scenario": scenario.name, "rooms": scenario.rooms, "participants_per_room": scenario.participants,
				"audio_payload_bytes": 160, "video_payload_bytes": 1200, "packets_per_source_per_second": 50,
				"screen": scenario.screen, "recording_audio_tap_drained": scenario.egress,
				"elapsed_seconds": elapsed.Seconds(), "cpu_seconds": processCPU(cpuAfter) - processCPU(cpuBefore),
				"mallocs": after.Mallocs - before.Mallocs, "allocated_bytes": after.TotalAlloc - before.TotalAlloc,
				"mallocs_per_second":         float64(after.Mallocs-before.Mallocs) / elapsed.Seconds(),
				"allocated_bytes_per_second": float64(after.TotalAlloc-before.TotalAlloc) / elapsed.Seconds(),
				"forwarded_packets":          endStats.Packets - startStats.Packets, "forwarded_payload_bytes": endStats.Bytes - startStats.Bytes,
				"dropped_packets": endStats.Dropped - startStats.Dropped, "audio_tap_drops": endStats.AudioTapDrops - startStats.AudioTapDrops,
				"recording_drops": endStats.RecordingDrops - startStats.RecordingDrops,
			}
			encoded, err := json.MarshalIndent(metrics, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "metrics.json"), append(encoded, '\n'), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Log(string(encoded))
			p0MediaCheckpoint(t, h, dir, "after", 0)
			select {
			case err := <-h.errors:
				t.Fatalf("media harness: %v", err)
			default:
			}
			if endStats.Dropped != startStats.Dropped || endStats.AudioTapDrops != 0 || endStats.RecordingDrops != 0 {
				t.Fatalf("unstable media workload: %+v", endStats)
			}
			for _, stream := range streams {
				stream.Close()
			}
			readers.Wait()
			for _, peer := range peers {
				peer.close()
			}
			p0MediaEmpty(t, h)
			p0MediaCheckpoint(t, h, dir, "cleanup", 0)
		})
	}
}

func p1StartScreen(t *testing.T, h *pionHarness, peer *testPeer) {
	t.Helper()
	track, err := pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, "screen", "display")
	if err != nil {
		t.Fatal(err)
	}
	peer.neg.Lock()
	peer.sourceDeclarations = map[string]media.Source{"microphone": media.SourceMicrophone, "camera": media.SourceCamera, "screen": media.SourceVideoScreen}
	sender, err := peer.pc.AddTrack(track)
	peer.neg.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.negotiate(); err != nil {
		t.Fatal(err)
	}
	peer.wg.Add(2)
	go func() {
		defer peer.wg.Done()
		for {
			if _, _, err := sender.ReadRTCP(); err != nil {
				return
			}
		}
	}()
	go func() {
		defer peer.wg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		payload := make([]byte, h.payloadBytes(pion.RTPCodecTypeVideo))
		payload[0] = 0x10
		var sequence uint16
		for {
			select {
			case <-peer.ctx.Done():
				return
			case <-ticker.C:
				sequence++
				_ = track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: sequence, Timestamp: uint32(sequence) * 1800, Marker: true}, Payload: payload})
			}
		}
	}()
}

// BenchmarkP1RecordPacket calls the actual SFU encoded-packet egress path.
// Consumers drain synchronously, isolating marshal/snapshot cost from scheduling.
func BenchmarkP1RecordPacket(b *testing.B) {
	for _, size := range []int{160, 1200} {
		for _, sinks := range []int{0, 1, 2} {
			b.Run(fmt.Sprintf("payload_%d/sinks_%d", size, sinks), func(b *testing.B) {
				m := &Manager{}
				r := newRoom("benchmark")
				t := &publishedTrack{metadata: media.Track{ID: "track", Kind: media.KindAudio}, publisher: &peer{room: r}}
				t.permitted.Store(true)
				for n := 0; n < sinks; n++ {
					r.egresses[strconv.Itoa(n)] = &egress{manager: m, frames: make(chan media.EgressFrame, 1), done: make(chan struct{}), tracks: map[string]bool{"track": true}}
				}
				packet := &rtp.Packet{Header: rtp.Header{Version: 2, SSRC: 1234, PayloadType: 111}, Payload: make([]byte, size)}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for b.Loop() {
					m.recordPacket(t, packet)
					for _, sink := range r.egresses {
						<-sink.frames
					}
				}
			})
		}
	}
}
