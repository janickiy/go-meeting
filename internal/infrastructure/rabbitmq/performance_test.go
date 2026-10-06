package rabbitmq

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
	amqp "github.com/rabbitmq/amqp091-go"
)

func benchmarkPublisher(b *testing.B) (*Publisher, func()) {
	b.Helper()
	address := os.Getenv("RABBITMQ_TEST_URL")
	if address == "" {
		b.Skip("isolated local RabbitMQ required")
	}
	uri, err := amqp.ParseURI(address)
	if err != nil {
		b.Fatal(err)
	}
	if uri.Host != "127.0.0.1" && uri.Host != "localhost" {
		b.Fatal("local broker required")
	}
	id := "p1-bench-" + uuid.NewString()
	p, err := NewPublisher(context.Background(), Options{URL: address, Exchange: id, Queue: id, RoutingKey: "commands"})
	if err != nil {
		b.Fatal(err)
	}
	return p, func() {
		p.mu.Lock()
		_, _ = p.channel.QueueDelete(id, false, false, false)
		_, _ = p.channel.QueueDelete(id+".failed", false, false, false)
		_ = p.channel.ExchangeDelete(id, false, false)
		p.mu.Unlock()
		p.Close()
	}
}

func BenchmarkConfirmedCommandSerial(b *testing.B) {
	p, cleanup := benchmarkPublisher(b)
	defer cleanup()
	id := uuid.NewString()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := p.StartRecord(context.Background(), id, 5); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

func BenchmarkConfirmedCommandParallel(b *testing.B) {
	p, cleanup := benchmarkPublisher(b)
	defer cleanup()
	id := uuid.NewString()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := p.StartRecord(context.Background(), id, 5); err != nil {
				b.Error(err)
				return
			}
		}
	})
	b.StopTimer()
}

// A closed own connection models a broker disconnect; recovery still requires
// topology and publisher confirms. No existing service connection is touched.
func BenchmarkPublisherReconnect(b *testing.B) {
	p, cleanup := benchmarkPublisher(b)
	defer cleanup()
	id := uuid.NewString()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.mu.Lock()
		closeAMQP(p.conn)
		p.mu.Unlock()
		if err := p.StartRecord(context.Background(), id, 5); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}

func TestConsumerReconnectRecoveryLatency(t *testing.T) {
	address := os.Getenv("RABBITMQ_TEST_URL")
	if address == "" {
		t.Skip("isolated local RabbitMQ required")
	}
	uri, err := amqp.ParseURI(address)
	if err != nil || (uri.Host != "127.0.0.1" && uri.Host != "localhost") {
		t.Fatal("local broker required")
	}
	id := "p1-recovery-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	options := Options{URL: address, Exchange: id, Queue: id, RoutingKey: "commands", ConsumerTag: id}
	p, err := NewPublisher(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := NewConsumer(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer func() {
		_, _ = p.channel.QueueDelete(id, false, false, false)
		_, _ = p.channel.QueueDelete(id+".failed", false, false, false)
		_ = p.channel.ExchangeDelete(id, false, false)
	}()
	got := make(chan struct{}, 8)
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(ctx, func(context.Context, records.Command) error { got <- struct{}{}; return nil })
	}()
	defer func() { cancel(); c.Close(); <-done }()
	if err = p.StartRecord(ctx, uuid.NewString(), 5); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal("warmup message unavailable")
	}
	for i := 0; i < 3; i++ {
		c.mu.Lock()
		old := c.conn
		c.mu.Unlock()
		closeAMQP(old)
		started := time.Now()
		if err = p.StartRecord(ctx, uuid.NewString(), 5); err != nil {
			t.Fatal(err)
		}
		select {
		case <-got:
			t.Logf("consumer_reconnect_round=%d delivery_recovery=%s", i+1, time.Since(started))
		case <-ctx.Done():
			t.Fatal("reconnect message unavailable")
		}
	}
}
