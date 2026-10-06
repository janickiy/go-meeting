package rabbitmq

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// blockedAMQPWrite reproduces TCP write backpressure after a successful broker
// handshake. net.Pipe preserves real net.Conn deadline and Close behavior.
type blockedAMQPWrite struct {
	net.Conn
	blocked   atomic.Bool
	entered   chan struct{}
	once      sync.Once
	writeSide net.Conn
	readSide  net.Conn
}

func (c *blockedAMQPWrite) Write(p []byte) (int, error) {
	if !c.blocked.Load() {
		return c.Conn.Write(p)
	}
	c.once.Do(func() { close(c.entered) })
	return c.writeSide.Write(p)
}

func (c *blockedAMQPWrite) SetDeadline(deadline time.Time) error {
	_ = c.writeSide.SetDeadline(deadline)
	return c.Conn.SetDeadline(deadline)
}

func (c *blockedAMQPWrite) SetWriteDeadline(deadline time.Time) error {
	_ = c.writeSide.SetWriteDeadline(deadline)
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *blockedAMQPWrite) Close() error {
	_ = c.readSide.Close()
	_ = c.writeSide.Close()
	return c.Conn.Close()
}

func TestPublisherCancelsBlockedSocketWrite(t *testing.T) {
	address := os.Getenv("RABBITMQ_TEST_URL")
	if address == "" {
		t.Skip("isolated local RabbitMQ required")
	}
	uri, err := amqp.ParseURI(address)
	if err != nil || (uri.Host != "localhost" && uri.Host != "127.0.0.1") {
		t.Fatal("local broker required")
	}
	var raw *blockedAMQPWrite
	connection, err := amqp.DialConfig(address, amqp.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, address, time.Second)
		if err != nil {
			return nil, err
		}
		a, b := net.Pipe()
		raw = &blockedAMQPWrite{Conn: conn, entered: make(chan struct{}), writeSide: a, readSide: b}
		_ = raw.SetDeadline(time.Now().Add(3 * time.Second))
		return raw, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	channel, err := connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := channel.Confirm(false); err != nil {
		t.Fatal(err)
	}
	p := &Publisher{conn: connection, channel: channel, exchange: "", routingKey: "p0-audit-unrouted", options: Options{URL: address}}
	p.watch(connection)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	raw.blocked.Store(true)
	go func() { finished <- p.StartRecord(ctx, "11111111-1111-4111-8111-111111111111", 2) }()
	select {
	case <-raw.entered:
	case <-time.After(time.Second):
		t.Fatal("publish did not reach socket write")
	}
	<-ctx.Done()
	select {
	case err := <-finished:
		if err == nil {
			t.Error("cancelled blocked publish succeeded")
		}
	case <-time.After(300 * time.Millisecond):
		buf := make([]byte, 128<<10)
		n := runtime.Stack(buf, true)
		t.Logf("blocked publication after context cancellation:\n%s", buf[:n])
		t.Error("publisher ignored cancellation while holding its transport mutex")
	}
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(300 * time.Millisecond):
		t.Error("publisher Close cannot interrupt blocked write")
	}
	// Force the underlying test socket closed so the regression itself cannot
	// retain AMQP goroutines when run against the known broken implementation.
	_ = raw.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("socket cleanup did not release Close")
	}
	select {
	case <-finished:
	default:
	}
}

func blockedBrokerChannel(t *testing.T) (*amqp.Connection, *amqp.Channel, *blockedAMQPWrite) {
	t.Helper()
	address := os.Getenv("RABBITMQ_TEST_URL")
	if address == "" {
		t.Skip("isolated local RabbitMQ required")
	}
	uri, err := amqp.ParseURI(address)
	if err != nil || (uri.Host != "localhost" && uri.Host != "127.0.0.1") {
		t.Fatal("local broker required")
	}
	var raw *blockedAMQPWrite
	connection, err := amqp.DialConfig(address, amqp.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		conn, err := net.DialTimeout(network, address, time.Second)
		if err != nil {
			return nil, err
		}
		a, b := net.Pipe()
		raw = &blockedAMQPWrite{Conn: conn, entered: make(chan struct{}), writeSide: a, readSide: b}
		_ = raw.SetDeadline(time.Now().Add(3 * time.Second))
		return raw, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close(); closeAMQP(connection) })
	channel, err := connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	return connection, channel, raw
}

func TestAMQPRPCCancelsBlockedSocketWrite(t *testing.T) {
	for _, operation := range []string{"confirm", "qos", "consume", "ack", "nack"} {
		t.Run(operation, func(t *testing.T) {
			conn, ch, raw := blockedBrokerChannel(t)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			raw.blocked.Store(true)
			started := time.Now()
			err := boundedAMQPRPC(ctx, conn, func() error {
				switch operation {
				case "confirm":
					return ch.Confirm(false)
				case "qos":
					return ch.Qos(1, 0, false)
				case "consume":
					_, err := ch.Consume("unused-p0-audit", "", false, false, false, false, nil)
					return err
				case "ack":
					return ch.Ack(1, false)
				default:
					return ch.Nack(1, false, true)
				}
			})
			if err == nil || time.Since(started) > 400*time.Millisecond || !conn.IsClosed() {
				t.Fatalf("unbounded %s: elapsed=%s error=%v closed=%v", operation, time.Since(started), err, conn.IsClosed())
			}
		})
	}
}

func TestPublisherCloseInterruptsSocketWrite(t *testing.T) {
	conn, ch, raw := blockedBrokerChannel(t)
	if err := ch.Confirm(false); err != nil {
		t.Fatal(err)
	}
	p := &Publisher{conn: conn, channel: ch, exchange: "", routingKey: "p0-audit-unrouted"}
	p.watch(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw.blocked.Store(true)
	finished := make(chan error, 1)
	go func() { finished <- p.StartRecord(ctx, "11111111-1111-4111-8111-111111111111", 2) }()
	select {
	case <-raw.entered:
	case <-time.After(time.Second):
		t.Fatal("publish did not reach write")
	}
	started := time.Now()
	p.Close()
	if time.Since(started) > 1500*time.Millisecond {
		t.Fatalf("Close unbounded: %s", time.Since(started))
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("blocked publication accepted after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("Close retained publisher goroutine")
	}
	if p.Check(context.Background()) == nil {
		t.Fatal("closed publisher reports healthy")
	}
}

func TestAMQPHandshakeCancelsWithoutDeadline(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		_, _ = io.Copy(io.Discard, conn)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := dial(ctx, fmt.Sprintf("amqp://guest:guest@%s/", listener.Addr())); finished <- err }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("handshake not connected")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled handshake accepted")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handshake ignored context cancellation")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("handshake left socket open")
	}
}
