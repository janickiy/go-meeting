package redis

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	goredis "github.com/redis/go-redis/v9"
)

// mediaRegistryFixture подготавливает или проверяет часть тестового сценария «медиа Registry тестовое окружение».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*MediaRegistry): значение, подготовленное операцией для вызывающей стороны.
func mediaRegistryFixture(t *testing.T) *MediaRegistry {
	t.Helper()
	addr := os.Getenv("RECORDER_STAGE2_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RECORDER_STAGE2_TEST_REDIS_ADDR for isolated real Redis media ownership tests")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		t.Fatal("media integration tests require local Redis")
	}
	client := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	s := NewMediaRegistry(client, "test:media-registry:"+uuid.NewString())
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			keys, _ := client.Keys(context.Background(), s.prefix+":*").Result()
			if len(keys) > 0 {
				_ = client.Del(context.Background(), keys...).Err()
			}
			_ = client.Close()
		})
	return s
}

// TestMediaOwnershipAtomicClaimAndFencing проверяет атомарный захват владения медиа и защиту версии аренды.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaOwnershipAtomicClaimAndFencing(t *testing.T) {
	s := mediaRegistryFixture(t)
	ctx := context.Background()
	workers := []media.Worker{{ID: "w-a", Endpoint: "http://worker-a:8091"}, {ID: "w-b", Endpoint: "http://worker-b:8091"}}
	for _, w := range workers {
		if err := s.RegisterWorker(ctx, w, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	conferenceID := uuid.NewString()
	var wg sync.WaitGroup
	routes := make(chan media.Route, 20)
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - index (int): значение index типа int, используемое согласно назначению этой операции.
		*/func(index int) {
			defer wg.Done()
			r, err := s.Claim(ctx, conferenceID, workers[index%2].ID, time.Second)
			if err != nil {
				failures <- err
			} else {
				routes <- r
			}
		}(i)
	}
	wg.Wait()
	close(routes)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var first media.Route
	for route := range routes {
		if first.WorkerID == "" {
			first = route
		}
		if first != route {
			t.Fatal("simultaneous claim produced distinct owners")
		}
	}
	if err := s.Renew(ctx, conferenceID, first, time.Second); err != nil {
		t.Fatal(err)
	}
	wrong := first
	wrong.LeaseID = uuid.NewString()
	if err := s.Renew(ctx, conferenceID, wrong, time.Second); !errors.Is(err, media.ErrOwnership) {
		t.Fatal("foreign lease renewed owner")
	}
	if err := s.Release(ctx, conferenceID, wrong); err != nil {
		t.Fatal(err)
	}
	if actual, err := s.GetOwner(ctx, conferenceID); err != nil || actual != first {
		t.Fatal("foreign lease removed owner", err)
	}
	if err := s.RemoveWorker(ctx, first.WorkerID, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetOwner(ctx, conferenceID); !errors.Is(err, media.ErrOwnership) {
		t.Fatal("dead owner remained routable")
	}
	other := workers[0]
	if other.ID == first.WorkerID {
		other = workers[1]
	}
	second, err := s.Claim(ctx, conferenceID, other.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if first.LeaseID == second.LeaseID {
		t.Fatal("reassignment did not fence old owner")
	}
	_ = s.Release(ctx, conferenceID, first)
	if actual, err := s.GetOwner(ctx, conferenceID); err != nil || actual != second {
		t.Fatal("old release removed reassigned owner", err)
	}
	if err := s.Renew(ctx, conferenceID, first, time.Second); !errors.Is(err, media.ErrOwnership) {
		t.Fatal("old owner renewed reassigned room")
	}
}

// TestMediaWorkerExpiryAndRegistrationBinding проверяет истечение воркера и привязку регистрации.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaWorkerExpiryAndRegistrationBinding(t *testing.T) {
	s := mediaRegistryFixture(t)
	ctx := context.Background()
	w := media.Worker{ID: "expiry-worker", Endpoint: "http://worker:8091"}
	if err := s.RegisterWorker(ctx, w, 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	changed := w
	changed.Endpoint = "http://elsewhere:8091"
	if err := s.RegisterWorker(ctx, changed, time.Second); !errors.Is(err, media.ErrOwnership) {
		t.Fatal("live worker ID hijacked")
	}
	_ = s.RemoveWorker(ctx, w.ID, changed.Endpoint)
	workers, err := s.Workers(ctx)
	if err != nil || len(workers) != 1 {
		t.Fatal("foreign endpoint deleted worker", err)
	}
	conf := uuid.NewString()
	if _, err := s.Claim(ctx, conf, w.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(230 * time.Millisecond)
	if workers, err = s.Workers(ctx); err != nil || len(workers) != 0 {
		t.Fatal("expired worker retained", err)
	}
	if _, err = s.GetOwner(ctx, conf); !errors.Is(err, media.ErrOwnership) {
		t.Fatal("owner retained after heartbeat expired")
	}
	if _, err = s.Claim(ctx, conf, w.ID, time.Second); !errors.Is(err, media.ErrUnavailable) {
		t.Fatal("dead worker claimed conference")
	}
}
