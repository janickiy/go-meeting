package rabbitmq

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
	amqp "github.com/rabbitmq/amqp091-go"
	"testing"
)

// drainAck фиксирует подтверждения без реального брокера.
type drainAck struct{ ack, requeue int }

// Ack учитывает обработанную команду; параметры доставки в проверке не используются.
func (a *drainAck) Ack(uint64, bool) error { a.ack++; return nil }

// Nack учитывает только безопасный возврат отложенного старта в очередь.
func (a *drainAck) Nack(_ uint64, _ bool, requeue bool) error {
	if requeue {
		a.requeue++
	}
	return nil
}

// Reject предоставляет полный контракт подтверждений AMQP.
func (a *drainAck) Reject(uint64, bool) error { return nil }

// TestRecorderDrainDefersStartsAndAllowsStops не помещает отложенные старты в карантин при повторной доставке.
// @args t — контекст проверки очереди и числа незавершённых команд.
func TestRecorderDrainDefersStartsAndAllowsStops(t *testing.T) {
	c := &Consumer{}
	c.BeginDrain()
	ack := &drainAck{}
	handled := 0
	for _, kind := range []string{"record.start", "record.stop"} {
		body, _ := json.Marshal(records.Command{Type: kind, RecordID: uuid.NewString()})
		c.handleDelivery(context.Background(), amqp.Delivery{Body: body, Acknowledger: ack, Redelivered: true}, func(context.Context, records.Command) error {
			handled++
			if c.Active() != 1 {
				t.Error("in-flight command missing")
			}
			return nil
		})
	}
	if handled != 1 || ack.ack != 1 || ack.requeue != 1 || c.Active() != 0 {
		t.Fatal("incorrect drain disposition", handled, ack)
	}
}
