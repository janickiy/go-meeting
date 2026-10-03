package rabbitmq

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// TestNormalizeOptionsLocalBroker проверяет сценарий «нормализация Options локальный Broker», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestNormalizeOptionsLocalBroker(t *testing.T) {
	got := normalizeOptions(Options{})
	if got.URL != "amqp://go_recorder:go_recorder_pass@rabbitmq:5672/%2F" {
		t.Fatalf("default RabbitMQ URL = %q", got.URL)
	}
	if got.Exchange != "go-recorder.commands" || got.Queue != "go-recorder.recording.commands" || got.RoutingKey != "record.commands" {
		t.Fatalf("default topology = %+v", got)
	}
}

// TestCommandRoundTrip проверяет сериализацию команды и восстановление её данных.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCommandRoundTrip(t *testing.T) {
	dsn := os.Getenv("RABBITMQ_TEST_URL")
	if dsn == "" {
		t.Skip("set RABBITMQ_TEST_URL to run the RabbitMQ integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	suffix := uuid.NewString()
	options := Options{
		URL: dsn, Exchange: "go-recorder.test." + suffix,
		Queue: "go-recorder.test." + suffix, RoutingKey: "record.commands",
		ConsumerTag: "go-recorder-test-" + suffix,
	}
	publisher, err := NewPublisher(ctx, options)
	if err != nil {
		t.Fatalf("connect publisher: %v", err)
	}
	defer publisher.Close()
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		if _, err := publisher.channel.QueueDelete(options.Queue, false, false, false); err != nil {
			t.Errorf("delete test queue: %v", err)
		}
		if err := publisher.channel.ExchangeDelete(options.Exchange, false, false); err != nil {
			t.Errorf("delete test exchange: %v", err)
		}
	}()
	consumer, err := NewConsumer(ctx, options)
	if err != nil {
		t.Fatalf("connect consumer: %v", err)
	}
	defer consumer.Close()
	received := make(chan records.Command, 2)
	done := make(chan error, 1)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		done <- consumer.Consume(ctx, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
			  - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.

			@return:
			  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(_ context.Context, command records.Command) error {
				received <- command
				return nil
			})
	}()
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("consume: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("consumer did not stop")
		}
	}()
	if err := publisher.StartRecord(ctx, suffix, 7); err != nil {
		t.Fatalf("publish start: %v", err)
	}
	if err := publisher.StopRecord(ctx, suffix, "test_stop"); err != nil {
		t.Fatalf("publish stop: %v", err)
	}
	want := []records.Command{
		{Type: "record.start", RecordID: suffix, SegmentDurationSec: 7},
		{Type: "record.stop", RecordID: suffix, Reason: "test_stop"},
	}
	for _, expected := range want {
		select {
		case got := <-received:
			if got.Type != expected.Type || got.RecordID != expected.RecordID || got.SegmentDurationSec != expected.SegmentDurationSec || got.Reason != expected.Reason {
				t.Fatalf("command = %+v, want %+v", got, expected)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for command")
		}
	}
}
