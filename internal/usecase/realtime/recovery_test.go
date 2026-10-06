package realtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

type recoverySubscription struct {
	failed chan struct{}
	closed chan struct{}
	once   sync.Once
}

func newRecoverySubscription() *recoverySubscription {
	return &recoverySubscription{failed: make(chan struct{}), closed: make(chan struct{})}
}

func (s *recoverySubscription) Receive(ctx context.Context) (domain.Bus, error) {
	select {
	case <-ctx.Done():
		return domain.Bus{}, ctx.Err()
	case <-s.failed:
		return domain.Bus{}, errors.New("subscription disconnected")
	case <-s.closed:
		return domain.Bus{}, errors.New("subscription closed")
	}
}

func (s *recoverySubscription) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

type recoveryStore struct {
	Store
	subscribe func(context.Context) (domain.Subscription, error)
}

func (s *recoveryStore) Subscribe(ctx context.Context) (domain.Subscription, error) {
	return s.subscribe(ctx)
}

func (s *recoveryStore) Prune(context.Context) error { return nil }

type recoveryRepository struct{ Repository }

func (recoveryRepository) Stale(context.Context, time.Time, string) ([]domain.Session, error) {
	return nil, nil
}

func waitRecovery(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal(message)
	}
}

func TestHubShutdownWinsLateSuccessfulResubscribe(t *testing.T) {
	first, late := newRecoverySubscription(), newRecoverySubscription()
	started, release, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls int
	store := &recoveryStore{subscribe: func(ctx context.Context) (domain.Subscription, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return late, nil // Simulate completion concurrent with shutdown cancellation.
	}}
	hub, err := NewHub(recoveryRepository{}, store, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	close(first.failed)
	waitRecovery(t, started, "recovery did not begin")
	if !errors.Is(hub.Check(context.Background()), apperrors.ErrUnavailable) {
		t.Fatal("Hub remained ready while subscription recovery was blocked")
	}
	stopped := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		stopped <- hub.ShutdownContext(ctx)
	}()
	waitRecovery(t, cancelled, "shutdown did not cancel Subscribe")
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	waitRecovery(t, late.closed, "late successful subscription leaked after shutdown")
	if !errors.Is(hub.Check(context.Background()), apperrors.ErrUnavailable) {
		t.Fatal("successful Subscribe reopened a shutdown Hub")
	}
}

func TestHubStalePruneFailureCannotCloseRecoveredGeneration(t *testing.T) {
	first, second := newRecoverySubscription(), newRecoverySubscription()
	var calls int
	store := &recoveryStore{subscribe: func(context.Context) (domain.Subscription, error) {
		calls++
		if calls == 1 {
			return first, nil
		}
		return second, nil
	}}
	hub, err := NewHub(recoveryRepository{}, store, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Shutdown()
	hub.mu.Lock()
	oldGeneration := hub.generation
	hub.mu.Unlock()
	close(first.failed)
	deadline := time.Now().Add(2 * time.Second)
	for {
		hub.mu.Lock()
		recovered := hub.generation > oldGeneration && !hub.unavailable
		hub.mu.Unlock()
		if recovered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription did not recover")
		}
		time.Sleep(time.Millisecond)
	}
	hub.breakBroker(oldGeneration)
	if err := hub.Check(context.Background()); err != nil {
		t.Fatal("stale Prune error made recovered Hub unavailable", err)
	}
	select {
	case <-second.closed:
		t.Fatal("stale Prune error closed the new subscription")
	default:
	}
}
