package redis

import (
	"context"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"testing"
	"time"
)

// TestDrainingWorkerPreservesOwnership исключает новые назначения, сохраняя продление действующей аренды.
// @args t — интеграционный тест в случайном пространстве имён Redis.
func TestDrainingWorkerPreservesOwnership(t *testing.T) {
	s := mediaRegistryFixture(t)
	ctx := context.Background()
	w := media.Worker{ID: "drain-worker", Endpoint: "http://worker:8091"}
	if err := s.RegisterWorker(ctx, w, time.Minute); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	route, err := s.Claim(ctx, id, w.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	w.Draining = true
	if err := s.RegisterWorker(ctx, w, time.Minute); err != nil {
		t.Fatal(err)
	}
	workers, err := s.Workers(ctx)
	if err != nil || len(workers) != 0 {
		t.Fatal("draining worker advertised", workers, err)
	}
	if _, err := s.Claim(ctx, uuid.NewString(), w.ID, time.Minute); err == nil {
		t.Fatal("new room claimed")
	}
	if err := s.Renew(ctx, id, route, time.Minute); err != nil {
		t.Fatal("existing ownership lost", err)
	}
	if current, err := s.GetOwner(ctx, id); err != nil || current != route {
		t.Fatal("existing route lost", err)
	}
}
