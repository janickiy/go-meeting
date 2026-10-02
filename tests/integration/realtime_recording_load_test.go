package integration_test

import (
	"context"
	"os"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestStageSixWSReconnectBurst измеряет 100 физических WS и одновременный
// повторный вход после разрыва. t управляет отдельной БД и Redis namespace.
func TestStageSixWSReconnectBurst(t *testing.T) {
	if os.Getenv("RECORDER_WS_LOAD") != "true" {
		t.Skip("manual WebSocket load")
	}
	f := stageTwo(t)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	g0 := runtime.NumGoroutine()
	var cpu0, cpu1 syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpu0)
	sockets := make([]*testSocket, 100)
	for i := range sockets {
		sockets[i] = f.connect(t, i%2, f.ownerToken, f.conference.ID)
	}
	var wg sync.WaitGroup
	for _, s := range sockets {
		wg.Add(1)
		go func(s *testSocket) { defer wg.Done(); _ = s.conn.Close(); <-s.done }(s)
	}
	wg.Wait()
	latencies := make([]time.Duration, 100)
	start := time.Now()
	for i := range sockets {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started := time.Now()
			sockets[i] = f.connect(t, i%2, f.ownerToken, f.conference.ID)
			latencies[i] = time.Since(started)
		}(i)
	}
	wg.Wait()
	active, err := f.store.Active(context.Background(), f.conference.ID)
	if err != nil || len(active) != 100 {
		t.Fatalf("burst active=%d: %v", len(active), err)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	runtime.ReadMemStats(&after)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &cpu1)
	cpu := float64(cpu1.Utime.Sec+cpu1.Stime.Sec-cpu0.Utime.Sec-cpu0.Stime.Sec) + float64(cpu1.Utime.Usec+cpu1.Stime.Usec-cpu0.Utime.Usec-cpu0.Stime.Usec)/1e6
	t.Logf("WS active=100 burst=%s p50=%s p95=%s p99=%s heap_bytes=%d->%d goroutines=%d->%d cpu_seconds=%.3f", time.Since(start).Round(time.Millisecond), latencies[49].Round(time.Millisecond), latencies[94].Round(time.Millisecond), latencies[98].Round(time.Millisecond), before.HeapAlloc, after.HeapAlloc, g0, runtime.NumGoroutine(), cpu)
	for _, s := range sockets {
		_ = s.conn.Close()
	}
	for _, hub := range f.hubs {
		hub.Shutdown()
	}
	active, err = f.store.Active(context.Background(), f.conference.ID)
	if err != nil || len(active) != 0 {
		t.Fatal("WS cleanup left presence")
	}
}

// TestStageSixConcurrentRecordings запускает два независимых полных конвейера
// SFU→FFmpeg→MinIO. t включает их только при RECORDER_RECORDING_LOAD=true;
// данные каждой записи изолированы отдельной БД, Redis namespace и каталогом.
func TestStageSixConcurrentRecordings(t *testing.T) {
	if os.Getenv("RECORDER_RECORDING_LOAD") != "true" {
		t.Skip("manual concurrent recording load")
	}
	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) { t.Parallel(); TestStageFourCompositeRecording(t) })
	}
}
