package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/sys/unix"
)

// A connection owns its reader; closing it joins that reader. Unlike connect(),
// this helper does not retain completed connections in testing.T cleanup hooks.
func p0OpenSocket(address, token string) (*ws.Conn, <-chan struct{}, error) {
	conn, response, err := ws.DefaultDialer.Dial(address, http.Header{"Authorization": {"Bearer " + token}})
	if err != nil {
		if response != nil {
			response.Body.Close()
			return nil, nil, fmt.Errorf("upgrade status=%d: %w", response.StatusCode, err)
		}
		return nil, nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var first domain.Envelope
	if err := conn.ReadJSON(&first); err != nil || first.Type != "conference.state" {
		conn.Close()
		return nil, nil, fmt.Errorf("initial state type=%q: %v", first.Type, err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return conn, done, nil
}

// Killing only a named subscription simulates a transient Redis TCP loss;
// unrelated subscribers, Redis itself, and database connections stay alive.
func TestP0WSRecoversAfterRedisSubscriptionDisconnect(t *testing.T) {
	p0WSRecoveryCase(t, false)
}

type p0TransientPruneStore struct {
	realtimeusecase.Store
	fail atomic.Bool
}

func (s *p0TransientPruneStore) Prune(ctx context.Context) error {
	if s.fail.Swap(false) {
		return errors.New("injected single Redis pruning transport failure")
	}
	return s.Store.Prune(ctx)
}

func TestP0WSRecoversAfterTransientPruneFailure(t *testing.T) {
	p0WSRecoveryCase(t, true)
}

func p0WSRecoveryCase(t *testing.T, pruneFailure bool) {
	t.Helper()
	f := stageTwo(t)
	name := "p0-pubsub-" + uuid.NewString()
	client := goredis.NewClient(&goredis.Options{Addr: os.Getenv("RECORDER_STAGE2_TEST_REDIS_ADDR"), Password: os.Getenv("RECORDER_STAGE2_TEST_REDIS_PASSWORD"), ClientName: name})
	defer client.Close()
	store := redisinfra.NewRealtimeStore(client, f.config.Namespace)
	pruning := &p0TransientPruneStore{Store: store}
	hub, err := realtimeusecase.NewHub(pg.NewSessionRepository(f.db), pruning, f.config.SessionTTL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Shutdown()
	router := gin.New()
	wstransport.NewHandler(hub, f.tokens, store, nil, f.config).RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/conferences/" + f.conference.ID + "/ws"
	conn, done, err := p0OpenSocket(address, f.ownerToken)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if pruneFailure {
		pruning.fail.Store(true)
	} else {
		list, err := client.ClientList(context.Background()).Result()
		if err != nil {
			t.Fatal(err)
		}
		var killed int
		for _, line := range strings.Split(list, "\n") {
			fields := map[string]string{}
			for _, field := range strings.Fields(line) {
				key, value, ok := strings.Cut(field, "=")
				if ok {
					fields[key] = value
				}
			}
			if fields["name"] != name || !strings.Contains(fields["flags"], "P") {
				continue
			}
			if err := client.Do(context.Background(), "CLIENT", "KILL", "ID", fields["id"]).Err(); err != nil {
				t.Fatal(err)
			}
			killed++
		}
		if killed != 1 {
			t.Fatalf("expected one isolated PubSub connection, killed=%d", killed)
		}
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("old WebSocket did not close after its subscription was lost")
	}
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal("Redis did not recover", err)
	}
	var latest error
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		next, stopped, err := p0OpenSocket(address, f.ownerToken)
		if err == nil {
			next.Close()
			<-stopped
			t.Log("Redis Ping succeeded; WebSocket admission and initial state recovered without restarting the Hub")
			return
		}
		latest = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("Redis Ping succeeds but WebSocket admission never recovers: %v", latest)
}

type p0WSSnapshot struct {
	Phase             string `json:"phase"`
	Goroutines        int    `json:"goroutines"`
	HeapBytes         uint64 `json:"heapBytes"`
	HeapObjects       uint64 `json:"heapObjects"`
	RSSBytes          int64  `json:"rssBytes"`
	OpenFDs           int    `json:"openFDs"`
	LocalWebSockets   int    `json:"localWebSockets"`
	RedisKeys         int    `json:"redisKeys"`
	ConnectedSessions int64  `json:"connectedSessions"`
}

func p0WaitWSCleanup(t *testing.T, f *stageTwoFixture) {
	t.Helper()
	until := time.Now().Add(10 * time.Second)
	for time.Now().Before(until) {
		local := 0
		for _, hub := range f.hubs {
			local += hub.LocalCount()
		}
		var connected int64
		err := f.db.Model(&domain.Session{}).Where("status = 'connected'").Count(&connected).Error
		active, storeErr := f.store.Active(context.Background(), f.conference.ID)
		if err == nil && storeErr == nil && local == 0 && connected == 0 && len(active) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("WebSocket/Redis/ParticipantSession cleanup did not return to zero")
}

func p0CaptureWS(t *testing.T, f *stageTwoFixture, phase, directory string) p0WSSnapshot {
	t.Helper()
	p0WaitWSCleanup(t, f)
	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	snapshot := p0WSSnapshot{Phase: phase, Goroutines: runtime.NumGoroutine(), HeapBytes: stats.HeapAlloc, HeapObjects: stats.HeapObjects, RSSBytes: -1, OpenFDs: -1}
	for _, path := range []string{"/proc/self/fd", "/dev/fd"} {
		if fds, err := os.ReadDir(path); err == nil {
			snapshot.OpenFDs = len(fds)
			break
		}
	}
	// macOS does not always enumerate /dev/fd. F_GETFD measures the calling
	// process directly without creating a child process or measurement pipes.
	if snapshot.OpenFDs < 0 {
		var limit unix.Rlimit
		if unix.Getrlimit(unix.RLIMIT_NOFILE, &limit) == nil {
			snapshot.OpenFDs = 0
			for fd := uint64(0); fd < limit.Cur; fd++ {
				if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
					snapshot.OpenFDs++
				}
			}
		}
	}
	if output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output(); err == nil {
		if kb, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64); err == nil {
			snapshot.RSSBytes = kb * 1024
		}
	}
	for _, hub := range f.hubs {
		snapshot.LocalWebSockets += hub.LocalCount()
	}
	keys, err := f.redis.Keys(context.Background(), f.config.Namespace+":*").Result()
	if err != nil {
		t.Fatal(err)
	}
	snapshot.RedisKeys = len(keys)
	if err := f.db.Model(&domain.Session{}).Where("status = 'connected'").Count(&snapshot.ConnectedSessions).Error; err != nil {
		t.Fatal(err)
	}
	if directory != "" {
		for _, profile := range []string{"heap", "allocs", "goroutine", "mutex", "block"} {
			out, err := os.Create(filepath.Join(directory, phase+"-"+profile+".pprof"))
			if err != nil {
				t.Fatal(err)
			}
			err = pprof.Lookup(profile).WriteTo(out, 0)
			closeErr := out.Close()
			if err != nil || closeErr != nil {
				t.Fatal("profile output failed", err, closeErr)
			}
		}
	}
	raw, _ := json.Marshal(snapshot)
	t.Log(string(raw))
	return snapshot
}

// Opt-in resource audit: a warmup followed by 4x250 complete physical socket
// lifecycles on two API instances, with no retained test client/history slices.
func TestP0WSRepeatedLifecycle(t *testing.T) {
	if os.Getenv("RECORDER_P0_WS_STRESS") != "true" {
		t.Skip("set RECORDER_P0_WS_STRESS=true for repeated resource profiling")
	}
	f := stageTwo(t)
	directory := os.Getenv("RECORDER_P0_PROFILE_DIR")
	if directory != "" {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	defer runtime.SetMutexProfileFraction(0)
	defer runtime.SetBlockProfileRate(0)
	run := func(count int) {
		for i := 0; i < count; i++ {
			address := "ws" + strings.TrimPrefix(f.servers[i%2].URL, "http") + "/api/v1/conferences/" + f.conference.ID + "/ws"
			conn, done, err := p0OpenSocket(address, f.ownerToken)
			if err != nil {
				t.Fatalf("cycle %d: %v", i, err)
			}
			conn.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("test client reader did not stop")
			}
		}
		p0WaitWSCleanup(t, f)
	}
	snapshots := []p0WSSnapshot{p0CaptureWS(t, f, "idle", directory)}
	run(20)
	warm := p0CaptureWS(t, f, "warm", directory)
	snapshots = append(snapshots, warm)
	for round := 1; round <= 4; round++ {
		run(250)
		snapshot := p0CaptureWS(t, f, "round-"+strconv.Itoa(round), directory)
		snapshots = append(snapshots, snapshot)
		if snapshot.Goroutines > warm.Goroutines+8 || snapshot.OpenFDs > warm.OpenFDs+4 || snapshot.HeapBytes > warm.HeapBytes+8<<20 || snapshot.LocalWebSockets != 0 || snapshot.ConnectedSessions != 0 || snapshot.RedisKeys != 0 {
			t.Fatalf("post-GC resources did not stabilize: warm=%+v current=%+v", warm, snapshot)
		}
	}
	if directory != "" {
		raw, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "metrics.json"), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		cpu, err := os.Create(filepath.Join(directory, "idle-after-load-cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(cpu); err != nil {
			cpu.Close()
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		pprof.StopCPUProfile()
		if err := cpu.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
