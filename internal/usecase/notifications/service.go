package notifications

import (
	"context"
	"log/slog"
	"time"

	domain "github.com/janickiy/go-recorder/internal/domain/notifications"
	realtime "github.com/janickiy/go-recorder/internal/domain/realtime"
)

type Repository interface {
	List(context.Context, string, string, int) (domain.Page, error)
	Read(context.Context, string, string) (domain.Notification, error)
	Generate(context.Context) error
	Pending(context.Context) ([]domain.Notification, error)
	Published(context.Context, string) error
}
type Publisher interface {
	Publish(context.Context, string, realtime.Envelope) error
}
type Service struct {
	repo Repository
	bus  Publisher
}

func NewService(repo Repository, bus Publisher) *Service { return &Service{repo: repo, bus: bus} }
func (s *Service) List(ctx context.Context, userID, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, userID, cursor, limit)
}
func (s *Service) Read(ctx context.Context, userID, id string) (domain.Notification, error) {
	item, err := s.repo.Read(ctx, userID, id)
	if err != nil {
		return item, err
	}
	// Read state is durable before publication. Poll/reconnect repairs missed
	// Redis events; no sensitive content is placed on a conference broadcast.
	event := realtime.Event("notification.read", "", map[string]any{"id": item.ID})
	_ = s.bus.Publish(ctx, userID, event)
	return item, nil
}
func (s *Service) Tick(ctx context.Context) error {
	if err := s.repo.Generate(ctx); err != nil {
		return err
	}
	pending, err := s.repo.Pending(ctx)
	if err != nil {
		return err
	}
	for _, item := range pending {
		event := realtime.Event("notification.created", "", map[string]any{"notification": item})
		event.ID = item.ID // At-least-once deliveries have a stable identity.
		if err = s.bus.Publish(ctx, item.UserID, event); err != nil {
			return err
		}
		if err = s.repo.Published(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		op, cancel := context.WithTimeout(ctx, 4*time.Second)
		err := s.Tick(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("notification job failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
