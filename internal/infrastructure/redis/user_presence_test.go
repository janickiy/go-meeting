package redis

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	goredis "github.com/redis/go-redis/v9"
)

type presenceCommandCounter struct{ commands int }

func (h *presenceCommandCounter) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return next(ctx, network, address)
	}
}
func (h *presenceCommandCounter) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.commands++
		return next(ctx, cmd)
	}
}
func (h *presenceCommandCounter) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func TestUserPresenceOnlineBatchLeases(t *testing.T) {
	fixture := presenceFixture(t)
	presence := NewUserPresence(fixture.client, fixture.prefix, 200*time.Millisecond)
	ctx := context.Background()
	users := make([]string, 100)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}
	for _, tab := range []string{"tab-a", "tab-b"} {
		if _, err := presence.Touch(ctx, users[0], tab); err != nil {
			t.Fatal(err)
		}
	}
	// An expired lease retained in a live sorted set must not make its user online.
	clock, err := fixture.client.Time(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.client.ZAdd(ctx, presence.prefix+users[1], goredis.Z{Score: float64(clock.UnixMilli() - 1), Member: "dead-tab"}).Err(); err != nil {
		t.Fatal(err)
	}
	hook := &presenceCommandCounter{}
	fixture.client.AddHook(hook)
	statuses, err := presence.Online(ctx, users)
	if err != nil || len(statuses) != 100 || !statuses[users[0]] || statuses[users[1]] || statuses[users[99]] {
		t.Fatalf("batch statuses=%v err=%v", statuses, err)
	}
	if hook.commands != 1 {
		t.Fatalf("100-user lookup used %d commands, want one EVAL", hook.commands)
	}
	if _, err = presence.Remove(ctx, users[0], "tab-a"); err != nil {
		t.Fatal(err)
	}
	statuses, err = presence.Online(ctx, users[:1])
	if err != nil || !statuses[users[0]] {
		t.Fatalf("closing one of two tabs removed global presence: %v %v", statuses, err)
	}
	if _, err = presence.Remove(ctx, users[0], "tab-b"); err != nil {
		t.Fatal(err)
	}
	statuses, err = presence.Online(ctx, users[:1])
	if err != nil || statuses[users[0]] {
		t.Fatalf("last tab closed but remained online: %v %v", statuses, err)
	}
	if _, err = presence.Touch(ctx, users[0], "abandoned-tab"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond)
	statuses, err = presence.Online(ctx, users[:1])
	if err != nil || statuses[users[0]] {
		t.Fatalf("abandoned tab survived TTL: %v %v", statuses, err)
	}
	// Reads neither recreate nor renew an expired lease.
	if exists, err := fixture.client.Exists(ctx, presence.prefix+users[0]).Result(); err != nil || exists != 0 {
		t.Fatalf("batch read recreated expired key: %d %v", exists, err)
	}
}

func TestUserPresenceOnlineBoundsAndFailures(t *testing.T) {
	fixture := presenceFixture(t)
	presence := NewUserPresence(fixture.client, fixture.prefix, time.Second)
	ctx := context.Background()
	if statuses, err := presence.Online(ctx, make([]string, 101)); !errors.Is(err, apperrors.ErrInvalidInput) || statuses != nil {
		t.Fatalf("unbounded batch accepted: %v %v", statuses, err)
	}
	if _, err := presence.Touch(ctx, "user", "tab"); err != nil {
		t.Fatal(err)
	}
	if statuses, err := presence.Online(ctx, []string{"", "user", "user"}); err != nil || len(statuses) != 1 || !statuses["user"] {
		t.Fatalf("duplicate/empty identities: %v %v", statuses, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if statuses, err := presence.Online(cancelled, []string{"user"}); err == nil || statuses != nil {
		t.Fatalf("cancelled read returned partial offline status: %v %v", statuses, err)
	}
	// A corrupted Redis key is unavailable, not an offline account.
	if err := fixture.client.Set(ctx, presence.prefix+"broken", "wrong type", time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	if statuses, err := presence.Online(ctx, []string{"user", "broken"}); err == nil || statuses != nil {
		t.Fatalf("Redis error returned partial statuses: %v %v", statuses, err)
	}
}
