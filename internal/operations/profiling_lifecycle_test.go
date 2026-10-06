package operations

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/config"
)

func TestProfilingEndpointsAndShutdown(t *testing.T) {
	if err := (&Runtime{}).Profiling(context.Background()); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Runtime{Config: config.OperationsConfig{PprofPort: port}}
	done := make(chan error, 1)
	go func() { done <- r.Profiling(ctx) }()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}
	base := "http://127.0.0.1:" + strconv.Itoa(port) + "/debug/pprof/"
	deadline := time.Now().Add(time.Second)
	for {
		response, err := client.Get(base)
		if err == nil {
			_ = response.Body.Close()
			break
		}
		select {
		case err := <-done:
			t.Fatalf("profiling server failed: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, profile := range []string{"heap", "allocs", "goroutine", "mutex", "block", "profile?seconds=1"} {
		response, err := client.Get(base + profile)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil || len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
			t.Fatalf("profile %s: HTTP=%d bytes=%d error=%v", profile, response.StatusCode, len(body), readErr)
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("profiling listener did not close on cancellation")
	}
}
