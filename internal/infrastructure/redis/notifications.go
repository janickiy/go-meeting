package redis

import (
	"context"
	"encoding/json"

	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

// NotificationBus изолирует личные уведомления в Redis-каналах пользователей.
// @params
//   - client: клиент внешнего сервиса или транспорта компонента.
//   - prefix: ограниченный префикс объектов, относящихся к одной операции.
type NotificationBus struct {
	client *goredis.Client
	prefix string
}

// NewNotificationBus создаёт и связывает зависимости компонента NotificationBus, используемого в личных уведомлениях и их фоновой доставке.
//
// @args
//   - client (*goredis.Client): клиент внешнего сервиса или транспорта компонента.
//   - namespace (string): изолированное пространство Redis-ключей и каналов приложения или теста.
//
// @return:
//   - результат 1 (*NotificationBus): созданный компонент с переданными зависимостями.
func NewNotificationBus(client *goredis.Client, namespace string) *NotificationBus {
	return &NotificationBus{client: client, prefix: namespace + ":notification:"}
}

// Publish сериализует доверенное событие и публикует его в изолированном Redis-канале.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - event (domain.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (b *NotificationBus) Publish(ctx context.Context, userID string, event domain.Envelope) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.prefix+userID, data).Err()
}

// Subscribe открывает ограниченную по времени подписку на изолированный канал событий.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (*goredis.PubSub): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (b *NotificationBus) Subscribe(ctx context.Context, userID string) (*goredis.PubSub, error) {
	sub := b.client.Subscribe(ctx, b.prefix+userID)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	return sub, nil
}
