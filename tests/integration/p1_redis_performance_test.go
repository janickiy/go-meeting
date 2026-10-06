package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// Counts client request batches, not the internal commands executed by Lua.
type p1RedisHook struct {
	batches, commands atomic.Int64
	prefix            string
}

func (*p1RedisHook) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h *p1RedisHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.batches.Add(1)
		h.commands.Add(1)
		return next(ctx, cmd)
	}
}
func (h *p1RedisHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		h.batches.Add(1)
		h.commands.Add(int64(len(cmds)))
		return next(ctx, cmds)
	}
}

func p1RedisFixture(tb testing.TB, count int) (*redisinfra.RealtimeStore, *goredis.Client, []domain.Session, *p1RedisHook) {
	tb.Helper()
	addr := os.Getenv("RECORDER_P1_REDIS_ADDR")
	if addr == "" {
		tb.Skip("set RECORDER_P1_REDIS_ADDR to isolated loopback Redis")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		tb.Fatal("P1 load requires local Redis")
	}
	client, err := redisinfra.NewClient(context.Background(), addr, "", 0)
	if err != nil {
		tb.Fatal(err)
	}
	prefix := "test:p1:" + uuid.NewString()
	store := redisinfra.NewRealtimeStore(client, prefix)
	rows := make([]domain.Session, count)
	for i := range rows {
		rows[i] = domain.Session{ID: uuid.NewString(), ConnectionID: uuid.NewString(), ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), UserID: uuid.NewString(), Status: "connected", ConnectedAt: time.Now().Add(-5 * time.Minute), LastSeenAt: time.Now().UTC()}
		if err := store.Register(context.Background(), rows[i], 10*time.Minute); err != nil {
			tb.Fatal(err)
		}
	}
	if err := store.Prune(context.Background()); err != nil {
		tb.Fatal(err)
	}
	hook := &p1RedisHook{prefix: prefix}
	client.AddHook(hook)
	tb.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+":*").Result()
		if err == nil && len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
		_ = client.Close()
	})
	return store, client, rows, hook
}

// Compare the existing GET+decode loop with a bounded pipeline experiment.
// Both variants check precisely the same route keys; no product state is used.
func BenchmarkP1PresencePage(b *testing.B) {
	for _, count := range []int{2, 20, 500} {
		for _, variant := range []string{"SequentialGet", "PipelinedExists", "Missing"} {
			b.Run(fmt.Sprintf("%s/%d", variant, count), func(b *testing.B) {
				store, client, rows, hook := p1RedisFixture(b, count)
				ctx := context.Background()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if variant == "SequentialGet" {
						for _, row := range rows {
							if _, err := store.Get(ctx, row.ConnectionID); err != nil {
								b.Fatal(err)
							}
						}
					} else if variant == "Missing" {
						ids := make([]string, len(rows))
						for i := range rows {
							ids[i] = rows[i].ConnectionID
						}
						if missing, err := store.Missing(ctx, ids); err != nil || len(missing) != 0 {
							b.Fatalf("missing=%v err=%v", missing, err)
						}
					} else {
						_, err := client.Pipelined(ctx, func(pipe goredis.Pipeliner) error {
							for _, row := range rows {
								pipe.Exists(ctx, hook.prefix+":route:"+row.ConnectionID)
							}
							return nil
						})
						if err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				b.ReportMetric(float64(hook.batches.Load())/float64(b.N), "batches/op")
				b.ReportMetric(float64(hook.commands.Load())/float64(b.N), "commands/op")
			})
		}
	}
}

type p1StaleRepository struct {
	realtimeusecase.Repository
	rows  []domain.Session
	scans atomic.Int64
}

func (r *p1StaleRepository) Stale(context.Context, time.Time, string) ([]domain.Session, error) {
	r.scans.Add(1)
	return r.rows, nil
}

// The repository is synthetic: this isolates Redis reconciliation from SQL
// execution. Real DB query/pool load is measured by the separate P1 DB harness.
func TestP1RedisReconciliationLoad(t *testing.T) {
	if os.Getenv("RECORDER_P1_REDIS_LOAD") != "true" {
		t.Skip("set RECORDER_P1_REDIS_LOAD=true")
	}
	for _, count := range []int{0, 20, 500} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store, _, rows, hook := p1RedisFixture(t, count)
			repo := &p1StaleRepository{rows: rows}
			hub, err := realtimeusecase.NewHub(repo, store, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer hub.Shutdown()
			profileDir := os.Getenv("RECORDER_P1_PROFILE_DIR")
			if profileDir != "" {
				profileDir = filepath.Join(profileDir, fmt.Sprint(count))
				if err := os.MkdirAll(profileDir, 0o755); err != nil {
					t.Fatal(err)
				}
				runtime.SetMutexProfileFraction(1)
				runtime.SetBlockProfileRate(1)
				defer runtime.SetMutexProfileFraction(0)
				defer runtime.SetBlockProfileRate(0)
				cpu, err := os.Create(filepath.Join(profileDir, "cpu.pprof"))
				if err != nil {
					t.Fatal(err)
				}
				if err := pprof.StartCPUProfile(cpu); err != nil {
					t.Fatal(err)
				}
				defer cpu.Close()
			}
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			started := time.Now()
			time.Sleep(5 * time.Second)
			if profileDir != "" {
				pprof.StopCPUProfile()
			}
			if err := hub.ShutdownContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&after)
			result := map[string]any{"sessions": count, "seconds": time.Since(started).Seconds(), "scans": repo.scans.Load(), "redis_request_batches": hook.batches.Load(), "redis_commands": hook.commands.Load(), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs, "sql": "synthetic Stale repository; no SQL measured"}
			raw, _ := json.MarshalIndent(result, "", "  ")
			t.Log(string(raw))
			if err := hub.Check(context.Background()); err == nil {
				t.Fatal("shutdown Hub remained ready")
			}
			if profileDir != "" {
				if err := os.WriteFile(filepath.Join(profileDir, "metrics.json"), raw, 0o644); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"heap", "allocs", "mutex", "block", "goroutine"} {
					f, err := os.Create(filepath.Join(profileDir, name+".pprof"))
					if err != nil {
						t.Fatal(err)
					}
					if err := pprof.Lookup(name).WriteTo(f, 0); err != nil {
						t.Fatal(err)
					}
					f.Close()
				}
			}
		})
	}
}

type p1CountedSubscription struct {
	domain.Subscription
	read *atomic.Int64
}

func (s p1CountedSubscription) Receive(ctx context.Context) (domain.Bus, error) {
	bus, err := s.Subscription.Receive(ctx)
	if err == nil && bus.Kind == "event" {
		s.read.Add(1)
	}
	return bus, err
}

type p1CountedStore struct {
	*redisinfra.RealtimeStore
	read atomic.Int64
}

func (s *p1CountedStore) Subscribe(ctx context.Context) (domain.Subscription, error) {
	sub, err := s.RealtimeStore.Subscribe(ctx)
	if err != nil {
		return nil, err
	}
	return p1CountedSubscription{Subscription: sub, read: &s.read}, nil
}

// Disconnect only this fixture's named Pub/Sub connection, then verify both
// readiness and actual event delivery through the replacement subscription.
func TestP1RedisRecoveryLatency(t *testing.T) {
	addr := os.Getenv("RECORDER_P1_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RECORDER_P1_REDIS_ADDR to isolated loopback Redis")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		t.Fatal("local Redis required")
	}
	name := "p1-recovery-" + uuid.NewString()
	client := goredis.NewClient(&goredis.Options{Addr: addr, ClientName: name, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, ContextTimeoutEnabled: true})
	defer client.Close()
	store := &p1CountedStore{RealtimeStore: redisinfra.NewRealtimeStore(client, "test:"+name)}
	hub, err := realtimeusecase.NewHub(&p1StaleRepository{}, store, time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Shutdown()
	var elapsed []float64
	for i := 0; i < 5; i++ {
		list, err := client.ClientList(context.Background()).Result()
		if err != nil {
			t.Fatal(err)
		}
		killed := 0
		started := time.Now()
		for _, line := range strings.Split(list, "\n") {
			fields := map[string]string{}
			for _, field := range strings.Fields(line) {
				k, v, ok := strings.Cut(field, "=")
				if ok {
					fields[k] = v
				}
			}
			if fields["name"] == name && strings.Contains(fields["flags"], "P") {
				if err := client.Do(context.Background(), "CLIENT", "KILL", "ID", fields["id"]).Err(); err != nil {
					t.Fatal(err)
				}
				killed++
			}
		}
		if killed != 1 {
			t.Fatalf("killed subscriptions=%d", killed)
		}
		// The failed Receive can still be in progress when CLIENT KILL returns.
		until := time.Now().Add(2 * time.Second)
		for hub.Check(context.Background()) == nil && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if hub.Check(context.Background()) == nil {
			t.Fatal("disconnect did not invalidate Hub readiness")
		}
		for hub.Check(context.Background()) != nil && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if err := hub.Check(context.Background()); err != nil {
			t.Fatal("Hub did not recover", err)
		}
		elapsed = append(elapsed, float64(time.Since(started).Microseconds())/1000)
		if err := store.Publish(context.Background(), domain.Bus{Kind: "event", ConferenceID: "no-local-sockets", Event: &domain.Envelope{Type: "test.recovered"}}); err != nil {
			t.Fatal(err)
		}
		for store.read.Load() != int64(i+1) && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if store.read.Load() != int64(i+1) {
			t.Fatal("recovered subscription did not deliver published event")
		}
	}
	t.Logf("Redis CLIENT KILL -> acknowledged Hub readiness recovery ms: %v", elapsed)
	if dir := os.Getenv("RECORDER_P1_PROFILE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.MarshalIndent(map[string]any{"recovery_ms": elapsed, "fault": "CLIENT KILL named subscription; Redis remains healthy"}, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "recovery.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
