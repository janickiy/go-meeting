package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Options содержит настройки подключения к RabbitMQ проекта.
type Options struct {
	URL         string
	Exchange    string
	Queue       string
	RoutingKey  string
	ConsumerTag string
	Logger      *log.Logger
}

// CommandHandler обрабатывает одну команду записи из RabbitMQ.
type CommandHandler func(context.Context, records.Command) error

// Publisher публикует команды record.start/record.stop в RabbitMQ.
type Publisher struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	routingKey string
	options    Options
	mu         sync.Mutex
}

// Consumer читает команды record.start/record.stop из RabbitMQ.
type Consumer struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	queue       string
	consumerTag string
	logger      *log.Logger
}

// NewPublisher создает RabbitMQ publisher и объявляет exchange/queue/binding.
// Параметры:
// - ctx: контекст bootstrap-а.
// - options: URL, exchange, queue и routing key.
// Возвращает: готовый Publisher или ошибку подключения.
func NewPublisher(ctx context.Context, options Options) (*Publisher, error) {
	options = normalizeOptions(options)
	conn, channel, err := openDeclaredChannel(ctx, options)
	if err != nil {
		return nil, err
	}

	return &Publisher{
		conn:       conn,
		channel:    channel,
		exchange:   options.Exchange,
		routingKey: options.RoutingKey,
		options:    options,
	}, nil
}

// StartRecord публикует команду подготовки WebRTC ingest.
// Параметры:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента.
// Возвращает: ошибку публикации.
func (p *Publisher) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return p.publish(ctx, records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord публикует команду остановки записи, финализации, preview и загрузки в MinIO.
// Параметры:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - reason: причина остановки.
// Возвращает: ошибку публикации.
func (p *Publisher) StopRecord(ctx context.Context, recordID string, reason string) error {
	return p.publish(ctx, records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Close закрывает RabbitMQ channel и connection.
// Параметры: нет.
// Возвращает: ничего.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}

func (p *Publisher) publish(ctx context.Context, command records.Command) error {
	if p == nil || p.channel == nil {
		return fmt.Errorf("rabbitmq publisher is not configured")
	}
	body, err := json.Marshal(command)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.publishLocked(ctx, body, command.Type); err == nil {
		return nil
	}
	if err := p.reconnectLocked(ctx); err != nil {
		return err
	}

	return p.publishLocked(ctx, body, command.Type)
}

func (p *Publisher) publishLocked(ctx context.Context, body []byte, commandType string) error {
	if p.channel == nil {
		return fmt.Errorf("rabbitmq channel is not configured")
	}

	return p.channel.PublishWithContext(ctx, p.exchange, p.routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Type:         commandType,
		Body:         body,
	})
}

func (p *Publisher) reconnectLocked(ctx context.Context) error {
	if p.channel != nil {
		_ = p.channel.Close()
		p.channel = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	conn, channel, err := openDeclaredChannel(ctx, p.options)
	if err != nil {
		return err
	}
	p.conn = conn
	p.channel = channel

	return nil
}

// NewConsumer создает RabbitMQ consumer и объявляет exchange/queue/binding.
// Параметры:
// - ctx: контекст bootstrap-а.
// - options: URL, exchange, queue, routing key и consumer tag.
// Возвращает: готовый Consumer или ошибку подключения.
func NewConsumer(ctx context.Context, options Options) (*Consumer, error) {
	options = normalizeOptions(options)
	conn, channel, err := openDeclaredChannel(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := channel.Qos(1, 0, false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq qos: %w", err)
	}

	return &Consumer{
		conn:        conn,
		channel:     channel,
		queue:       options.Queue,
		consumerTag: options.ConsumerTag,
		logger:      options.Logger,
	}, nil
}

// Consume читает команды из RabbitMQ до отмены ctx.
// Параметры:
// - ctx: общий контекст worker-а.
// - handler: обработчик record.start/record.stop.
// Возвращает: ошибку consume loop.
func (c *Consumer) Consume(ctx context.Context, handler CommandHandler) error {
	if c == nil || c.channel == nil {
		return fmt.Errorf("rabbitmq consumer is not configured")
	}
	deliveries, err := c.channel.Consume(c.queue, c.consumerTag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq consume: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("rabbitmq deliveries channel closed")
			}
			c.handleDelivery(ctx, delivery, handler)
		}
	}
}

// Close закрывает RabbitMQ channel и connection.
// Параметры: нет.
// Возвращает: ничего.
func (c *Consumer) Close() {
	if c == nil {
		return
	}
	if c.channel != nil {
		_ = c.channel.Close()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery, handler CommandHandler) {
	var command records.Command
	if err := json.Unmarshal(delivery.Body, &command); err != nil {
		c.logf("reject invalid RabbitMQ command: %v", err)
		_ = delivery.Reject(false)
		return
	}
	if strings.TrimSpace(command.Type) == "" || strings.TrimSpace(command.RecordID) == "" {
		c.logf("reject incomplete RabbitMQ command: %+v", command)
		_ = delivery.Reject(false)
		return
	}
	c.logf("received RabbitMQ command %s for record %s", command.Type, command.RecordID)
	messageCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	if err := handler(messageCtx, command); err != nil {
		c.logf("command %s for record %s failed: %v", command.Type, command.RecordID, err)
		if delivery.Redelivered {
			_ = delivery.Reject(false)
			return
		}
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}

func (c *Consumer) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

func openDeclaredChannel(ctx context.Context, options Options) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := dial(ctx, options.URL)
	if err != nil {
		return nil, nil, err
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := declareTopology(channel, options); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, nil, err
	}

	return conn, channel, nil
}

func declareTopology(channel *amqp.Channel, options Options) error {
	if err := channel.ExchangeDeclare(options.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(options.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare queue: %w", err)
	}
	if err := channel.QueueBind(options.Queue, options.RoutingKey, options.Exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq bind queue: %w", err)
	}

	return nil
}

func dial(ctx context.Context, url string) (*amqp.Connection, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	type result struct {
		conn *amqp.Connection
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := amqp.Dial(url)
		done <- result{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		if result.err != nil {
			return nil, fmt.Errorf("rabbitmq connect: %w", result.err)
		}

		return result.conn, nil
	}
}

func normalizeOptions(options Options) Options {
	options.URL = strings.TrimSpace(options.URL)
	if options.URL == "" {
		options.URL = "amqp://go_recorder:go_recorder_pass@rabbitmq:5672/%2F"
	}
	options.Exchange = strings.TrimSpace(options.Exchange)
	if options.Exchange == "" {
		options.Exchange = "go-recorder.commands"
	}
	options.Queue = strings.TrimSpace(options.Queue)
	if options.Queue == "" {
		options.Queue = "go-recorder.recording.commands"
	}
	options.RoutingKey = strings.TrimSpace(options.RoutingKey)
	if options.RoutingKey == "" {
		options.RoutingKey = "record.commands"
	}
	options.ConsumerTag = strings.TrimSpace(options.ConsumerTag)
	if options.ConsumerTag == "" {
		options.ConsumerTag = "go-recorder-worker"
	}

	return options
}
