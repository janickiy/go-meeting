package redis

import (
	"context"
	"encoding/json"

	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	goredis "github.com/redis/go-redis/v9"
)

type NotificationBus struct {
	client *goredis.Client
	prefix string
}

func NewNotificationBus(client *goredis.Client, namespace string) *NotificationBus {
	return &NotificationBus{client: client, prefix: namespace + ":notification:"}
}
func (b *NotificationBus) Publish(ctx context.Context, userID string, event domain.Envelope) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.prefix+userID, data).Err()
}
func (b *NotificationBus) Subscribe(ctx context.Context, userID string) (*goredis.PubSub, error) {
	sub := b.client.Subscribe(ctx, b.prefix+userID)
	if _, err := sub.Receive(ctx); err != nil {
		_ = sub.Close()
		return nil, err
	}
	return sub, nil
}
