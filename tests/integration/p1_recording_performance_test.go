package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Measures the existing independent full recording pipelines without changing
// their media, command reliability, durable chunks or cleanup behavior.
func TestP1ConcurrentRecordingProfiles(t *testing.T) {
	if os.Getenv("RECORDER_P1_RECORDING_PROFILE") != "true" {
		t.Skip("opt-in isolated recording performance workload")
	}
	if runtime.GOOS != "linux" {
		t.Skip("Linux child CPU and /proc evidence required")
	}
	// Provision this disposable pod's shared bucket before parallel client
	// bootstrap: StageFour itself intentionally retains its normal bootstrap.
	prepareRecordingProfileBucket(t)
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	var before, after runtime.MemStats
	var parentBefore, parentAfter, childBefore, childAfter syscall.Rusage
	runtime.GC()
	runtime.ReadMemStats(&before)
	g0 := runtime.NumGoroutine()
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &parentBefore)
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &childBefore)
	started := time.Now()
	t.Run("pipelines", func(t *testing.T) { TestStageSixConcurrentRecordings(t) })
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &parentAfter)
	_ = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &childAfter)
	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	runtime.ReadMemStats(&after)
	cpu := func(u syscall.Rusage) int64 {
		return u.Utime.Sec*1e6 + int64(u.Utime.Usec) + u.Stime.Sec*1e6 + int64(u.Stime.Usec)
	}
	metrics := map[string]any{"wall_seconds": time.Since(started).Seconds(), "pipelines": 2, "parent_cpu_seconds": float64(cpu(parentAfter)-cpu(parentBefore)) / 1e6, "all_child_cpu_seconds": float64(cpu(childAfter)-cpu(childBefore)) / 1e6, "child_input_blocks": childAfter.Inblock - childBefore.Inblock, "child_output_blocks": childAfter.Oublock - childBefore.Oublock, "goroutines_before": g0, "goroutines_after_gc": runtime.NumGoroutine(), "heap_before": before.HeapAlloc, "heap_after_gc": after.HeapAlloc, "total_alloc_bytes": after.TotalAlloc - before.TotalAlloc, "mallocs": after.Mallocs - before.Mallocs, "parent_peak_rss_kib": parentAfter.Maxrss}
	encoded, _ := json.Marshal(metrics)
	t.Logf("P1_RECORDING_METRICS %s", encoded)
	if dir := os.Getenv("RECORDER_P1_PROFILE_DIR"); dir != "" {
		for _, kind := range []string{"heap", "allocs", "mutex", "block", "goroutine"} {
			file, err := os.Create(dir + "/recording-" + kind + ".pprof")
			if err != nil {
				t.Fatal(err)
			}
			if err = pprof.Lookup(kind).WriteTo(file, 0); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func prepareRecordingProfileBucket(t *testing.T) {
	t.Helper()
	endpoint := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT")
	parsed, err := url.Parse("http://" + endpoint)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") {
		t.Fatal("recording profile requires isolated local MinIO")
	}
	transport := &http.Transport{}
	defer transport.CloseIdleConnections()
	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY"), ""), Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exists, err := client.BucketExists(ctx, "recordings")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		if err = client.MakeBucket(ctx, "recordings", minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}
