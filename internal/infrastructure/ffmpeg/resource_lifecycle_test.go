package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/media"
)

type recordingResources struct {
	Phase      string `json:"phase"`
	Goroutines int    `json:"goroutines"`
	FDs        int    `json:"fds"`
	Children   int    `json:"child_processes"`
	Heap       uint64 `json:"heap_alloc_bytes"`
	RSS        uint64 `json:"rss_bytes"`
	TempFiles  int    `json:"temp_files"`
	TempBytes  int64  `json:"temp_bytes"`
}

func recordingSnapshot(t *testing.T, phase, work string) recordingResources {
	t.Helper()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	result := recordingResources{Phase: phase, Goroutines: runtime.NumGoroutine(), Heap: memory.HeapAlloc}
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	result.FDs = len(fds)
	children := make(map[string]bool)
	tasks, err := filepath.Glob("/proc/self/task/*/children")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		data, err := os.ReadFile(task)
		if err == nil {
			for _, child := range strings.Fields(string(data)) {
				children[child] = true
			}
		}
	}
	result.Children = len(children)
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "VmRSS:" {
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			result.RSS = kb * 1024
		}
	}
	err = filepath.Walk(work, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			result.TempFiles++
			result.TempBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(result)
	t.Logf("RESOURCE_SNAPSHOT %s", data)
	if evidence := os.Getenv("RECORDER_P0_EVIDENCE_DIR"); evidence != "" {
		for _, profile := range []string{"heap", "goroutine", "allocs"} {
			file, err := os.Create(filepath.Join(evidence, phase+"-"+profile+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			if err = pprof.Lookup(profile).WriteTo(file, 0); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err = file.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	return result
}

// Runs in an isolated Linux container with real FFmpeg, not an existing worker.
// The segment launcher execs real FFmpeg with deterministic lavfi media instead
// of RTP, so this exercises process/pipe/Wait and finalize ownership separately
// from the existing SFU-to-egress integration test.
func TestP0RealFFmpegResourceCycles(t *testing.T) {
	if os.Getenv("RECORDER_P0_FFMPEG_STRESS") != "true" {
		t.Skip("opt-in isolated real FFmpeg stress")
	}
	if runtime.GOOS != "linux" {
		t.Skip("Linux /proc resource measurements required")
	}
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err = os.Mkdir(work, 0700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "segment-fixture")
	script := "#!/bin/sh\nfor arg; do output=\"$arg\"; done\nexec ffmpeg -hide_banner -loglevel error -re -f lavfi -i color=c=blue:s=160x90:r=10 -f lavfi -i sine=frequency=440:sample_rate=8000 -c:v libx264 -preset ultrafast -g 1 -c:a aac -f segment -segment_time 0.1 -segment_format matroska -reset_timestamps 1 \"$output\"\n"
	if err = os.WriteFile(launcher, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	recorder := NewSegmentRecorder(launcher, log.New(io.Discard, "", 0))
	tracks := []RTPTrack{{Kind: "audio", MimeType: "audio/opus", ClockRate: 48000, Channels: 2, PayloadType: 111, Port: 39003}}
	post := NewPostProcessor(binary)
	var starts, finals, cancellations, failures, decoders int
	segmentCycle := func(finalize bool) {
		dir, err := os.MkdirTemp(work, "cycle-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		process, err := recorder.Start(ctx, "resource-cycle", tracks, filepath.Join(dir, "tmp"), filepath.Join(dir, "records"), 1)
		if err != nil {
			t.Fatal(err)
		}
		starts++
		if finalize {
			time.Sleep(100 * time.Millisecond)
			if err = process.Stop(time.Second); err != nil {
				t.Fatal(err)
			}
			result, err := post.Finalize(ctx, filepath.Join(dir, "records"))
			if err != nil || result.FinalSizeBytes == 0 || result.PreviewSizeBytes == 0 {
				t.Fatalf("finalize: %+v %v stderr=%s", result, err, process.Stderr())
			}
			finals++
		} else {
			cancel()
			select {
			case <-process.Done():
			case <-time.After(4 * time.Second):
				t.Fatal("cancelled segment process retained")
			}
			cancellations++
		}
		if process.cmd.ProcessState == nil {
			t.Fatal("segment process was not waited")
		}
		// A second or concurrent Stop must observe the same broadcast completion.
		stopped := make(chan struct{})
		go func() { _ = process.Stop(time.Second); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(250 * time.Millisecond):
			t.Fatal("duplicate Stop retained process waiter")
		}
		if len(process.Stderr()) > maxStderrBytes {
			t.Fatal("unbounded stderr")
		}
	}
	segmentCycle(true)
	segmentCycle(true)
	baseline := recordingSnapshot(t, "recording-warm", work)
	for round := 0; round < 3; round++ {
		for i := 0; i < 4; i++ {
			segmentCycle(true)
		}
		segmentCycle(false)
		for i := 0; i < 4; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			path := filepath.Join(work, "success.wav")
			out, err := runBounded(ctx, binary, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=duration=0.08", "-y", path)
			cancel()
			if err != nil {
				t.Fatalf("bounded success: %v %s", err, out)
			}
			if err = os.Remove(path); err != nil {
				t.Fatal(err)
			}
			ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			_, err = runBounded(ctx, binary, "-hide_banner", "-loglevel", "error", "-re", "-f", "lavfi", "-i", "anullsrc", "-f", "null", "-")
			cancel()
			if err == nil {
				t.Fatal("infinite real FFmpeg ignored timeout")
			}
			cancellations++
		}
		for _, args := range [][]string{
			{"-hide_banner", "-loglevel", "error", "-i", "/definitely-missing-p0-input", "-f", "null", "-"},
			{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=duration=0.1", "-f", "wav", "-y", "/dev/full"},
		} {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			out, err := runBounded(ctx, binary, args...)
			cancel()
			if err == nil || len(out) > maxStderrBytes {
				t.Fatalf("expected bounded input/disk failure: %v", err)
			}
			failures++
		}
		for i := 0; i < 2; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			err := (LiveAudio{Binary: binary}).Decode(ctx, media.EgressTrack{MimeType: "audio/opus", ClockRate: 48000, Channels: 1}, make(chan media.EgressFrame), func([]byte) error { return nil })
			cancel()
			if err == nil {
				t.Fatal("cancelled live decoder succeeded")
			}
			decoders++
		}
		after := recordingSnapshot(t, fmt.Sprintf("recording-round-%d", round+1), work)
		if after.Children != 0 || after.TempFiles != 0 || after.FDs > baseline.FDs+2 || after.Goroutines > baseline.Goroutines+2 {
			t.Fatalf("retained recording resources: before=%+v after=%+v", baseline, after)
		}
	}
	t.Logf("REAL_FFMPEG_COUNTS segment_starts=%d finalizations=%d cancellations_or_timeouts=%d expected_input_or_disk_failures=%d live_decoders=%d", starts, finals, cancellations, failures, decoders)
}
