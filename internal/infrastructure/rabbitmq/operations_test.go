package rabbitmq

import (
	"context"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/operations"
	amqp "github.com/rabbitmq/amqp091-go"
	"os"
	"testing"
	"time"
)

// TestReconnectCorrelationAndQuarantine проверяет разрыв собственных AMQP
// соединений, повторное подключение, correlation header и durable poison queue.
// t получает ошибки; требуется явно заданный локальный RABBITMQ_TEST_URL.
func TestReconnectCorrelationAndQuarantine(t *testing.T) {
	address := os.Getenv("RABBITMQ_TEST_URL")
	if address == "" {
		t.Skip("isolated RabbitMQ required")
	}
	parsed, err := amqp.ParseURI(address)
	if err != nil || (parsed.Host != "127.0.0.1" && parsed.Host != "localhost") {
		t.Fatal("local broker required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	id := uuid.NewString()
	opts := Options{URL: address, Exchange: "stage6-" + id, Queue: "stage6-" + id, RoutingKey: "commands"}
	p, err := NewPublisher(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := NewConsumer(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer func() {
		_, _ = p.channel.QueueDelete(opts.Queue, false, false, false)
		_, _ = p.channel.QueueDelete(opts.Queue+".failed", false, false, false)
		_ = p.channel.ExchangeDelete(opts.Exchange, false, false)
	}()
	got := make(chan string, 4)
	done := make(chan error, 1)
	go func() {
		done <- c.Consume(ctx, func(ctx context.Context, _ records.Command) error { got <- operations.ID(ctx); return nil })
	}()
	requestID := uuid.NewString()
	messageCtx := operations.WithID(ctx, requestID)
	if err := p.StartRecord(messageCtx, id, 5); err != nil {
		t.Fatal(err)
	}
	select {
	case actual := <-got:
		if actual != requestID {
			t.Fatal("correlation lost")
		}
	case <-ctx.Done():
		t.Fatal("first command lost")
	}
	c.mu.Lock()
	old := c.conn
	c.mu.Unlock()
	_ = old.Close()
	p.mu.Lock()
	_ = p.conn.Close()
	p.mu.Unlock()
	if err := p.StopRecord(messageCtx, id, "test"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-ctx.Done():
		t.Fatal("reconnect command lost")
	}
	err = p.channel.PublishWithContext(ctx, opts.Exchange, opts.RoutingKey, false, false, amqp.Publishing{Body: []byte("invalid-json"), DeliveryMode: amqp.Persistent})
	if err != nil {
		t.Fatal(err)
	}
	for {
		q, err := p.channel.QueueDeclarePassive(opts.Queue+".failed", false, false, false, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if q.Messages == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("poison message not quarantined")
		case <-time.After(50 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop")
	}
}
