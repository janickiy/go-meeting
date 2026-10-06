package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

func TestGroupRemovalSerializesWithBoundedDelivery(t *testing.T) {
	f := stageTwo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo := pg.NewPersonalRepository(f.db)
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Delivery lock", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	defer unlock()
	delivery := make(chan error, 1)
	go func() {
		allowed, err := repo.WithConversationDelivery(ctx, group.ID, f.member.ID, func() error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		if !allowed && err == nil {
			err = context.Canceled
		}
		delivery <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("delivery did not enter", ctx.Err())
	}
	removal := make(chan error, 1)
	go func() { _, _, err := repo.RemoveGroupMember(ctx, f.owner.ID, group.ID, f.member.ID); removal <- err }()
	// Observe a real PostgreSQL lock wait, rather than infer blocking from a sleep.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := false
	for !waiting {
		select {
		case err := <-removal:
			t.Fatal("removal committed before authorized delivery ended", err)
		case <-ctx.Done():
			t.Fatal("removal lock was not observed", ctx.Err())
		case <-ticker.C:
			var n int64
			if err := f.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&n).Error; err != nil {
				t.Fatal(err)
			}
			waiting = n > 0
		}
	}
	unlock()
	if err := <-delivery; err != nil {
		t.Fatal(err)
	}
	if err := <-removal; err != nil {
		t.Fatal(err)
	}
	called := false
	allowed, err := repo.WithConversationDelivery(ctx, group.ID, f.member.ID, func() error { called = true; return nil })
	if err != nil || allowed || called {
		t.Fatal("completed removal allowed a queued delivery", allowed, called, err)
	}
}
