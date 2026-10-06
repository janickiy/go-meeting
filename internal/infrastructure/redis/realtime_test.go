package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

func TestPresenceMissingBatchesAndCancellation(t *testing.T) {
	store := presenceFixture(t)
	ctx := context.Background()
	ids := make([]string, 1001)
	var want []string
	for i := range ids {
		ids[i] = fmt.Sprint(i)
		if i%3 == 0 {
			want = append(want, ids[i])
		} else if err := store.client.Set(ctx, store.prefix+":route:"+ids[i], "opaque session", time.Minute).Err(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.Missing(ctx, ids)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("missing=%v err=%v", got, err)
	}
	if err := store.client.PExpireAt(ctx, store.prefix+":route:1", time.Now().Add(-time.Second)).Err(); err != nil {
		t.Fatal(err)
	}
	got, err = store.Missing(ctx, []string{"1", "2"})
	if err != nil || !reflect.DeepEqual(got, []string{"1"}) {
		t.Fatalf("expired route=%v err=%v", got, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	got, err = store.Missing(cancelled, ids)
	if err == nil || len(got) != 0 {
		t.Fatalf("cancelled lookup returned partial absence: %v %v", got, err)
	}
}

// presenceFixture выделяет случайное пространство ключей в специально указанном
// локальном Redis; очистка не затрагивает ключи приложения или других тестов.
//
// @args
//   - t: контекст интеграционного теста Redis.
//
// @return: хранилище присутствия с отдельным случайным префиксом.
func presenceFixture(t *testing.T) *RealtimeStore {
	t.Helper()
	addr := os.Getenv("RECORDER_PRESENCE_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RECORDER_PRESENCE_TEST_REDIS_ADDR to an isolated local Redis")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		t.Fatal("presence tests require local Redis")
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	store := NewRealtimeStore(client, "test:presence:"+uuid.NewString())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), store.prefix+":*").Result()
		if err == nil && len(keys) > 0 {
			_ = client.Del(context.Background(), keys...).Err()
		}
		_ = client.Close()
	})
	return store
}

// presenceSession создаёт отдельное физическое соединение одного участника.
//
// @args
//   - seen: момент первоначального подтверждения связи сервером.
//
// @return: сессия с уникальным connectionId в общей тестовой конференции.
func presenceSession(seen time.Time) domain.Session {
	return domain.Session{ID: uuid.NewString(), ConferenceID: "meeting", ParticipantID: "member", UserID: "user", ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: seen, LastSeenAt: seen}
}

// TestPresenceDeadlineDoesNotMoveWithProcessing проверяет абсолютную границу
// первоначальной регистрации и обновления после задержанного подтверждения.
//
// @args
//   - t: контекст интеграционного теста настоящего Lua-сценария.
func TestPresenceDeadlineDoesNotMoveWithProcessing(t *testing.T) {
	store := presenceFixture(t)
	ctx := context.Background()
	const ttl = 5 * time.Second
	session := presenceSession(time.Now().UTC().Add(-4 * time.Second))
	if err := store.Register(ctx, session, ttl); err != nil {
		t.Fatal(err)
	}
	route := store.prefix + ":route:" + session.ConnectionID
	remaining, err := store.client.PTTL(ctx, route).Result()
	if err != nil || remaining <= 0 || remaining > 1100*time.Millisecond {
		t.Fatalf("registration added processing delay: ttl=%s err=%v", remaining, err)
	}
	confirmed := time.Now().UTC().Add(-4 * time.Second)
	if err := store.Touch(ctx, session.ConnectionID, confirmed, ttl); err != nil {
		t.Fatal(err)
	}
	remaining, err = store.client.PTTL(ctx, route).Result()
	if err != nil || remaining <= 0 || remaining > 1100*time.Millisecond {
		t.Fatalf("renewal added processing delay: ttl=%s err=%v", remaining, err)
	}
	actual, err := store.Get(ctx, session.ConnectionID)
	if err != nil || !actual.LastSeenAt.Equal(confirmed) {
		t.Fatal("stored heartbeat timestamp was replaced", err)
	}
	if err := store.Touch(ctx, session.ConnectionID, time.Now().Add(-6*time.Second), ttl); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("stale pong renewed live lease", err)
	}
	expired := presenceSession(time.Now().Add(-6 * time.Second))
	if err := store.Register(ctx, expired, ttl); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("expired initial connection was registered", err)
	}
	if err := store.client.PExpireAt(ctx, route, time.Now().Add(-time.Second)).Err(); err != nil {
		t.Fatal(err)
	}
	if err := store.Touch(ctx, session.ConnectionID, time.Now(), ttl); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("late pong resurrected expired connection", err)
	}
}

// TestPresenceDisconnectKeepsOtherTab проверяет отключение физической вкладки,
// атомарное истечение оставшейся аренды и доставку событий присутствия.
//
// @args
//   - t: контекст интеграционного теста настоящего Redis Pub/Sub.
func TestPresenceDisconnectKeepsOtherTab(t *testing.T) {
	store := presenceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	sub, err := store.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	a, b := presenceSession(time.Now().UTC()), presenceSession(time.Now().UTC())
	for _, session := range []domain.Session{a, b} {
		if err := store.Register(ctx, session, 5*time.Second); err != nil {
			t.Fatal(err)
		}
		bus, err := sub.Receive(ctx)
		if err != nil || bus.Kind != "connected" || bus.Session == nil || bus.Session.ConnectionID != session.ConnectionID {
			t.Fatal("missing connect event", err)
		}
	}
	if err := store.Unregister(ctx, a.ConnectionID); err != nil {
		t.Fatal(err)
	}
	bus, err := sub.Receive(ctx)
	if err != nil || bus.Kind != "disconnected" || bus.Session == nil || bus.Session.ConnectionID != a.ConnectionID {
		t.Fatal("missing disconnect event", err)
	}
	active, err := store.Active(ctx, "meeting")
	if err != nil || len(active) != 1 || active[0].ConnectionID != b.ConnectionID {
		t.Fatal("closed tab removed another active tab", err)
	}
	// Двигаем только границу своей тестовой аренды, не системные часы Redis.
	if err := store.client.ZAdd(ctx, store.prefix+":expiry", goredis.Z{Score: float64(time.Now().Add(-time.Second).UnixMilli()), Member: b.ConnectionID}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := store.Prune(ctx); err != nil {
		t.Fatal(err)
	}
	bus, err = sub.Receive(ctx)
	if err != nil || bus.Kind != "expired" || bus.Session == nil || bus.Session.ConnectionID != b.ConnectionID {
		t.Fatal("missing expiry event", err)
	}
	active, err = store.Active(ctx, "meeting")
	if err != nil || len(active) != 0 {
		t.Fatal("expired final tab left participant online", err)
	}
	if err := store.Touch(ctx, b.ConnectionID, time.Now(), 5*time.Second); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("expired tab returned after a late pong", err)
	}
}
