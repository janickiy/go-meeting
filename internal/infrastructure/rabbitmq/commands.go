package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/operations"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Options собирает зависимости и настройки создания компонента.
//   - URL: значение URL типа string, используемое согласно назначению этой операции.
//   - Exchange: значение Exchange типа string, используемое согласно назначению этой операции.
//   - Queue: значение Queue типа string, используемое согласно назначению этой операции.
//   - RoutingKey: значение RoutingKey типа string, используемое согласно назначению этой операции.
//   - ConsumerTag: значение ConsumerTag типа string, используемое согласно назначению этой операции.
//   - Logger: значение Logger типа *log.Logger, используемое согласно назначению этой операции.
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

// Publisher задаёт согласованное представление данных «Publisher» для доставке внутренних команд воркеру записи.
//   - conn: действующее сетевое соединение операции.
//   - channel: значение channel типа *amqp.Channel, используемое согласно назначению этой операции.
//   - exchange: значение exchange типа string, используемое согласно назначению этой операции.
//   - routingKey: значение routingKey типа string, используемое согласно назначению этой операции.
//   - options: зависимости и настройки создаваемого компонента.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
type Publisher struct {
	conn       *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	routingKey string
	options    Options
	mu         sync.Mutex
	alive      atomic.Bool
	closed     bool
}

// Consumer задаёт согласованное представление данных «Consumer» для доставке внутренних команд воркеру записи.
//   - conn: действующее сетевое соединение операции.
//   - channel: значение channel типа *amqp.Channel, используемое согласно назначению этой операции.
//   - queue: значение queue типа string, используемое согласно назначению этой операции.
//   - consumerTag: значение consumerTag типа string, используемое согласно назначению этой операции.
//   - logger: значение logger типа *log.Logger, используемое согласно назначению этой операции.
type Consumer struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	queue       string
	consumerTag string
	logger      *log.Logger
	options     Options
	mu          sync.Mutex
	alive       atomic.Bool
	closed      bool
}

// NewPublisher создает RabbitMQ publisher и объявляет exchange/queue/binding.
// @parameters:
// - ctx: контекст bootstrap-а.
// - options: URL, exchange, queue и routing key.
// @return готовый Publisher или ошибку подключения.
func NewPublisher(ctx context.Context, options Options) (*Publisher, error) {
	options = normalizeOptions(options)
	conn, channel, err := openDeclaredChannel(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("rabbitmq publisher confirms: %w", err)
	}

	p := &Publisher{
		conn:       conn,
		channel:    channel,
		exchange:   options.Exchange,
		routingKey: options.RoutingKey,
		options:    options,
	}
	p.watch(conn)
	return p, nil
}

// StartRecord публикует команду подготовки WebRTC ingest.
// @parameters:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента.
// @return ошибку публикации.
func (p *Publisher) StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error {
	return p.publish(ctx, records.Command{
		Type:               "record.start",
		RecordID:           recordID,
		SegmentDurationSec: segmentDurationSec,
	})
}

// StopRecord публикует команду остановки записи, финализации, preview и загрузки в MinIO.
// @parameters:
// - ctx: контекст HTTP-запроса API.
// - recordID: UUID записи.
// - reason: причина остановки.
// @return ошибку публикации.
func (p *Publisher) StopRecord(ctx context.Context, recordID string, reason string) error {
	return p.publish(ctx, records.Command{
		Type:     "record.stop",
		RecordID: recordID,
		Reason:   reason,
	})
}

// Close закрывает RabbitMQ channel и connection.
// @parameters: нет.
// @return ничего.
func (p *Publisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.alive.Store(false)
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.conn != nil {
		_ = p.conn.Close()
	}
}

// publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *Publisher) publish(ctx context.Context, command records.Command) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if p == nil {
		return fmt.Errorf("rabbitmq publisher is not configured")
	}
	body, err := json.Marshal(command)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("rabbitmq publisher closed")
	}

	if err := p.publishLocked(ctx, body, command.Type); err == nil {
		return nil
	}
	if err := p.reconnectLocked(ctx); err != nil {
		return err
	}

	return p.publishLocked(ctx, body, command.Type)
}

// publishLocked публикует команду через канал RabbitMQ при удерживаемой блокировке транспорта.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - body ([]byte): тело входящего запроса или сериализованные данные передачи.
//   - commandType (string): значение commandType типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (p *Publisher) publishLocked(ctx context.Context, body []byte, commandType string) error {
	if p.channel == nil {
		return fmt.Errorf("rabbitmq channel is not configured")
	}

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(ctx, p.exchange, p.routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Type:         commandType,
		Headers:      amqp.Table{"request_id": operations.ID(ctx)},
		Body:         body,
	})
	if err != nil {
		return err
	}
	if confirmation == nil {
		return fmt.Errorf("rabbitmq publisher confirmation unavailable")
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !acked {
		return fmt.Errorf("rabbitmq broker rejected command")
	}
	return nil
}

// Check проверяет действующий publisher connection без блокировки отправки.
// ctx задаёт дедлайн health-пробы; метод не создаёт новую очередь или публикацию.
func (p *Publisher) Check(ctx context.Context) error {
	if !p.alive.Load() {
		if !p.mu.TryLock() {
			return fmt.Errorf("broker reconnect in progress")
		}
		defer p.mu.Unlock()
		if p.closed {
			return fmt.Errorf("publisher closed")
		}
		return p.reconnectLocked(ctx)
	}
	return ctx.Err()
}

// reconnectLocked восстанавливает RabbitMQ-соединение и канал публикации при удерживаемой блокировке.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return fmt.Errorf("rabbitmq publisher confirms: %w", err)
	}
	p.conn = conn
	p.channel = channel
	p.watch(conn)

	operations.Event("rabbitmq_reconnect")
	return nil
}

// watch следит за закрытием конкретного соединения. conn — новый AMQP connection;
// закрытие старого connection не переводит новый publisher в failed.
func (p *Publisher) watch(conn *amqp.Connection) {
	p.alive.Store(true)
	go func() {
		<-conn.NotifyClose(make(chan *amqp.Error, 1))
		p.mu.Lock()
		if p.conn == conn {
			p.alive.Store(false)
		}
		p.mu.Unlock()
	}()
}

// NewConsumer создает RabbitMQ consumer и объявляет exchange/queue/binding.
// @parameters:
// - ctx: контекст bootstrap-а.
// - options: URL, exchange, queue, routing key и consumer tag.
// @return готовый Consumer или ошибку подключения.
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
	if err := channel.Confirm(false); err != nil {
		_ = conn.Close()
		return nil, err
	}

	consumer := &Consumer{
		conn:        conn,
		channel:     channel,
		queue:       options.Queue,
		consumerTag: options.ConsumerTag,
		logger:      options.Logger,
		options:     options,
	}
	return consumer, nil
}

// Consume читает команды из RabbitMQ до отмены ctx.
// @parameters:
// - ctx: общий контекст worker-а.
// - handler: обработчик record.start/record.stop.
// @return ошибку consume loop.
func (c *Consumer) Consume(ctx context.Context, handler CommandHandler) error {
	if c == nil {
		return fmt.Errorf("rabbitmq consumer is not configured")
	}
	for ctx.Err() == nil {
		c.mu.Lock()
		channel := c.channel
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return nil
		}
		var err error
		var deliveries <-chan amqp.Delivery
		if channel == nil {
			err = fmt.Errorf("broker disconnected")
		} else {
			deliveries, err = channel.Consume(c.queue, c.consumerTag, false, false, false, false, nil)
		}
		if err == nil {
			c.alive.Store(true)
		loop:
			for {
				select {
				case <-ctx.Done():
					c.alive.Store(false)
					return nil
				case delivery, ok := <-deliveries:
					if !ok {
						break loop
					}
					c.handleDelivery(ctx, delivery, handler)
				}
			}
		}
		c.alive.Store(false)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
		if err := c.reconnect(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("broker reconnect pending", "event_type", "rabbitmq.reconnect")
		}
	}
	return nil
}

// Check сообщает готовность consumer без сетевого запроса. ctx ограничивает
// health-пробу; готовность снимается при потере канала и восстанавливается после consume.
func (c *Consumer) Check(ctx context.Context) error {
	if !c.alive.Load() {
		return fmt.Errorf("consumer unavailable")
	}
	return ctx.Err()
}

// reconnect восстанавливает QoS и подтверждения после потери брокера. ctx
// ограничивает подключение; старые соединения закрываются, закрытый consumer
// не запускается повторно. Не подтверждённые доставки повторяет RabbitMQ.
func (c *Consumer) reconnect(ctx context.Context) error {
	conn, ch, err := openDeclaredChannel(ctx, c.options)
	if err != nil {
		return err
	}
	if err = errors.Join(ch.Qos(1, 0, false), ch.Confirm(false)); err != nil {
		_ = conn.Close()
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = conn.Close()
		return context.Canceled
	}
	old := c.conn
	c.conn = conn
	c.channel = ch
	c.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	operations.Event("rabbitmq_reconnect")
	return nil
}

// Close закрывает RabbitMQ channel и connection.
// @parameters: нет.
// @return ничего.
func (c *Consumer) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	c.alive.Store(false)
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// handleDelivery разбирает доставленную AMQP-команду и подтверждает либо назначает её повторную обработку.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - delivery (amqp.Delivery): значение delivery типа amqp.Delivery, используемое согласно назначению этой операции.
//   - handler (CommandHandler): обработчик вызываемой команды или маршрута.
func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery, handler CommandHandler) {
	started := time.Now()
	defer func() { operations.Observe("command", time.Since(started).Seconds()) }()
	if !delivery.Timestamp.IsZero() {
		operations.State("rabbitmq_last_queue_latency_seconds", time.Since(delivery.Timestamp).Seconds())
	}
	var command records.Command
	if len(delivery.Body) > 65536 || json.Unmarshal(delivery.Body, &command) != nil {
		c.quarantine(ctx, delivery)
		return
	}
	if _, err := uuid.Parse(command.RecordID); err != nil || (command.Type != "record.start" && command.Type != "record.stop") {
		c.quarantine(ctx, delivery)
		return
	}
	c.logf("received RabbitMQ command %s for record %s", command.Type, command.RecordID)
	messageCtx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	if id, ok := delivery.Headers["request_id"].(string); ok {
		messageCtx = operations.WithID(messageCtx, id)
	}
	slog.Info("recording command received", "event_type", command.Type, "recording_id", command.RecordID, "request_id", operations.ID(messageCtx), "queue_latency_seconds", time.Since(delivery.Timestamp).Seconds())
	defer cancel()
	if err := handler(messageCtx, command); err != nil {
		c.logf("command %s for record %s failed: %v", command.Type, command.RecordID, err)
		if delivery.Redelivered {
			c.quarantine(ctx, delivery)
			return
		}
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}

// quarantine сохраняет poison/repeated-failure сообщение в отдельной durable
// очереди с подтверждением брокера перед ack. ctx ограничивает операцию пятью
// секундами; при ошибке исходная доставка остаётся доступной для повтора.
func (c *Consumer) quarantine(ctx context.Context, d amqp.Delivery) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c.mu.Lock()
	ch := c.channel
	c.mu.Unlock()
	if ch == nil {
		_ = d.Nack(false, true)
		return
	}
	body := d.Body
	if len(body) > 65536 {
		body = body[:65536]
	}
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, "", c.queue+".failed", false, false, amqp.Publishing{ContentType: d.ContentType, DeliveryMode: amqp.Persistent, Timestamp: time.Now().UTC(), Body: body, Headers: amqp.Table{"failure": "poison_or_repeated_error", "original_bytes": int64(len(d.Body))}})
	if err == nil && confirm != nil {
		var ack bool
		ack, err = confirm.WaitContext(ctx)
		if ack && err == nil {
			_ = d.Ack(false)
			operations.Event("rabbitmq_dead_letter")
			slog.Warn("recording command quarantined", "event_type", "rabbitmq.dead_letter")
			return
		}
	}
	_ = d.Nack(false, true)
}

// logf записывает ограниченную диагностику компонента с указанными параметрами.
//
// @parameters:
//   - format (string): формат диагностического сообщения; чувствительные аргументы не должны раскрывать медиа.
//   - args (...any): значения переменного числа аргументов для форматирования или внешней команды.
func (c *Consumer) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// openDeclaredChannel открывает AMQP-канал и проверяет необходимую топологию очередей.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - options (Options): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (*amqp.Connection): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (*amqp.Channel): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// declareTopology объявляет обменники, очереди и привязки доставки команд записи.
//
// @parameters:
//   - channel (*amqp.Channel): значение channel типа *amqp.Channel, используемое согласно назначению этой операции.
//   - options (Options): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func declareTopology(channel *amqp.Channel, options Options) error {
	if err := channel.ExchangeDeclare(options.Exchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare exchange: %w", err)
	}
	if _, err := channel.QueueDeclare(options.Queue, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq declare queue: %w", err)
	}
	if _, err := channel.QueueDeclare(options.Queue+".failed", true, false, false, false, amqp.Table{"x-message-ttl": int32(7 * 24 * 60 * 60 * 1000), "x-max-length": int32(10000), "x-max-length-bytes": int64(64 << 20)}); err != nil {
		return fmt.Errorf("rabbitmq declare failed queue: %w", err)
	}
	if err := channel.QueueBind(options.Queue, options.RoutingKey, options.Exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq bind queue: %w", err)
	}

	return nil
}

// dial устанавливает соединение с RabbitMQ с настроенными ограничениями.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - url (string): адрес вызываемого ресурса.
//
// @return:
//   - результат 1 (*amqp.Connection): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func dial(ctx context.Context, url string) (*amqp.Connection, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
	}
	conn, err := amqp.DialConfig(url, amqp.Config{Heartbeat: 5 * time.Second, Dial: func(network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		socket, err := d.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		deadline, _ := ctx.Deadline()
		_ = socket.SetDeadline(deadline)
		return socket, nil
	}})
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect failed")
	}
	return conn, nil
}

// normalizeOptions заполняет значения по умолчанию и проверяет параметры RabbitMQ-транспорта.
//
// @parameters:
//   - options (Options): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (Options): значение, подготовленное операцией для вызывающей стороны.
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
