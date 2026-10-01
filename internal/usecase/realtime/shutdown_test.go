package realtime

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// shutdownSubscription — тестовая PubSub-подписка со счётчиком закрытий.
// Счётчик нужен для проверки однократной очистки при нескольких ShutdownContext.
type shutdownSubscription struct{ closed atomic.Int64 }

// Receive имитирует ожидание события до отмены ctx; возвращает причину отмены.
func (s *shutdownSubscription) Receive(ctx context.Context) (domain.Bus, error) {
	<-ctx.Done()
	return domain.Bus{}, ctx.Err()
}

// Close отмечает освобождение подписки; аргументов нет, ошибка отсутствует.
func (s *shutdownSubscription) Close() error { s.closed.Add(1); return nil }

// TestShutdownContextBoundsAndJoins проверяет ограниченное ожидание зависшего
// сокета, отмену Hub и продолжение той же очистки после освобождения сокета.
// t задаёт фиктивный незавершённый socket через WaitGroup, без сетевых сервисов.
func TestShutdownContextBoundsAndJoins(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := &shutdownSubscription{}
	h := &Hub{ctx: ctx, cancel: cancel, sub: sub, local: map[string]*localSocket{}}
	h.sockets.Add(1)
	deadline, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err := h.ShutdownContext(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected bounded deadline, got %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("Hub cancellation missing")
	}
	h.sockets.Done()
	join, done := context.WithTimeout(context.Background(), time.Second)
	defer done()
	for range 2 {
		if err := h.ShutdownContext(join); err != nil {
			t.Fatal(err)
		}
	}
	if sub.closed.Load() != 1 {
		t.Fatal("subscription closed repeatedly")
	}
}
