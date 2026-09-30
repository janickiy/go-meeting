package ffmpeg

import (
	"strings"
	"sync"
	"testing"
)

func TestLogTailRetainsNewestDiagnostics(t *testing.T) {
	var b logTail
	for _, chunk := range []string{strings.Repeat("x", maxStderrBytes*2), "last error"} {
		if n, err := b.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write() = %d, %v", n, err)
		}
	}
	if got := b.String(); len(got) != maxStderrBytes || !strings.HasSuffix(got, "last error") {
		t.Fatal("expected bounded log tail with the latest error")
	}
}

func TestLogTailConcurrentReadsAndWrites(t *testing.T) {
	var b logTail
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				_, _ = b.Write([]byte("ffmpeg diagnostic\n"))
				if len(b.String()) > maxStderrBytes {
					t.Error("log tail exceeded its limit")
				}
			}
		}()
	}
	wg.Wait()
}
