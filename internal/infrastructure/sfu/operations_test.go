package sfu

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestStageSixSoak проверяет повторные входы/выходы и доставку RTP в течение
// RECORDER_SOAK_DURATION (например 30m). t получает ошибки и снимки ресурсов.
// Тест не запускается в обычном CI; Redis/запись проверяются отдельными сценариями.
func TestStageSixSoak(t *testing.T) {
	raw := os.Getenv("RECORDER_SOAK_DURATION")
	if raw == "" {
		t.Skip("set RECORDER_SOAK_DURATION=30m for manual soak")
	}
	duration, err := time.ParseDuration(raw)
	if err != nil || duration < time.Second || duration > time.Hour {
		t.Fatal("soak duration must be 1s..1h")
	}
	h := harness(t, 3)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	beforeG, beforeFD := runtime.NumGoroutine(), openFDs()
	started, cycles := time.Now(), 0
	var cpuStart, cpuEnd syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuStart)
	var packets, bytes uint64
	for time.Since(started) < duration {
		room := uuid.NewString()
		a, b := h.join(t, room, ""), h.join(t, room, "")
		eventually(t, h, "bidirectional RTP", func() bool {
			a.mu.Lock()
			an := len(a.received)
			a.mu.Unlock()
			b.mu.Lock()
			bn := len(b.received)
			b.mu.Unlock()
			return an >= 2 && bn >= 2
		})
		time.Sleep(500 * time.Millisecond)
		a.close()
		b.close()
		eventually(t, h, "soak cleanup", func() bool {
			s := h.manager.Snapshot()
			return s.Rooms == 0 && s.Peers == 0 && s.Tracks == 0 && s.Subscriptions == 0
		})
		s := h.manager.Snapshot()
		packets, bytes = s.Packets, s.Bytes
		cycles++
		if cycles%30 == 0 {
			t.Logf("soak elapsed=%s cycles=%d goroutines=%d fd=%d", time.Since(started).Round(time.Second), cycles, runtime.NumGoroutine(), openFDs())
		}
		remaining := duration - time.Since(started)
		if remaining > 10*time.Second {
			time.Sleep(10 * time.Second)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.manager.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	runtime.GC()
	runtime.ReadMemStats(&after)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpuEnd)
	t.Logf("SOAK elapsed=%s cycles=%d cpu_seconds=%.3f goroutines=%d->%d heap_bytes=%d->%d fd=%d->%d packets=%d bytes=%d rooms=%d peers=%d", time.Since(started).Round(time.Second), cycles, processCPU(cpuEnd)-processCPU(cpuStart), beforeG, runtime.NumGoroutine(), before.HeapAlloc, after.HeapAlloc, beforeFD, openFDs(), packets, bytes, h.manager.Snapshot().Rooms, h.manager.Snapshot().Peers)
	if runtime.NumGoroutine() > beforeG+20 || (beforeFD >= 0 && openFDs() > beforeFD+10) {
		t.Fatal("persistent goroutine/FD growth after cleanup")
	}
}

// openFDs считает дескрипторы Linux/macOS; результат -1 означает недоступный учёт.
func openFDs() int {
	for _, path := range []string{"/proc/self/fd", "/dev/fd"} {
		if entries, err := os.ReadDir(path); err == nil {
			return len(entries)
		}
	}
	return -1
}

// processCPU переводит замер r системного и пользовательского CPU в секунды.
func processCPU(r syscall.Rusage) float64 {
	return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
}

// TestStageSixMediaLoad проверяет 2/5/10 издателей в двух независимых комнатах.
// t сохраняет фактическую стоимость синтетического RTP, не реального кодирования.
func TestStageSixMediaLoad(t *testing.T) {
	if os.Getenv("RECORDER_MEDIA_LOAD") != "true" {
		t.Skip("manual media load")
	}
	for _, n := range []int{2, 5, 10} {
		t.Run(fmt.Sprintf("%d_peers", n), func(t *testing.T) {
			h := harness(t, n, Options{MaxRooms: 2, NegotiationTimeout: 20 * time.Second})
			var before, during runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			var c0, c1 syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &c0)
			g0, fd0, start := runtime.NumGoroutine(), openFDs(), time.Now()
			peers := make([]*testPeer, 0, n*2)
			for r := 0; r < 2; r++ {
				room := uuid.NewString()
				for i := 0; i < n; i++ {
					peers = append(peers, h.joinSlots(t, room, "", n-1))
					eventually(t, h, "publisher tracks", func() bool { return h.manager.Snapshot().Tracks == 2*len(peers) })
				}
			}
			eventually(t, h, "all subscriptions", func() bool { return h.manager.Snapshot().Subscriptions == 4*n*(n-1) })
			eventually(t, h, "all receivers", func() bool {
				for _, p := range peers {
					p.mu.Lock()
					count := len(p.received)
					p.mu.Unlock()
					if count < 2*(n-1) {
						return false
					}
				}
				return true
			})
			time.Sleep(2 * time.Second)
			runtime.ReadMemStats(&during)
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &c1)
			s := h.manager.Snapshot()
			t.Logf("MEDIA rooms=2 participants_per_room=%d elapsed=%s cpu_seconds=%.3f heap_bytes=%d->%d goroutines=%d->%d fd=%d->%d packets=%d bytes=%d dropped=%d", n, time.Since(start).Round(time.Millisecond), processCPU(c1)-processCPU(c0), before.HeapAlloc, during.HeapAlloc, g0, runtime.NumGoroutine(), fd0, openFDs(), s.Packets, s.Bytes, s.Dropped)
			for _, p := range peers {
				p.close()
			}
			eventually(t, h, "load cleanup", func() bool { return h.manager.Snapshot().Rooms == 0 })
		})
	}
}
