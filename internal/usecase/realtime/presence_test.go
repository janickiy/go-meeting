package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// presenceRepository заменяет только проверку доступа для изолированного теста
// обновления аренды; остальные методы хранилища здесь не вызываются.
type presenceRepository struct{ Repository }

// Authorize разрешает тестовому участнику обновить присутствие.
//
// @args
//   - ctx: контекст операции.
//   - conferenceID: идентификатор тестовой конференции.
//   - userID: идентификатор тестового пользователя.
//
// @return: допущенный участник без обращения к базе данных.
func (r presenceRepository) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return conferences.Participant{ID: "member"}, nil
}

// presenceStore фиксирует параметры обновления, не используя настоящий Redis.
type presenceStore struct {
	Store
	called      bool
	connection  string
	confirmedAt time.Time
	ttl         time.Duration
}

// Touch сохраняет переданный дедлайн для проверки сохранения исходного pong.
//
// @args
//   - ctx: контекст операции.
//   - id: идентификатор физической сессии.
//   - confirmedAt: время приёма подтверждённого pong.
//   - ttl: разрешённое окно присутствия после pong.
//
// @return: nil — параметры успешно записаны тестовым хранилищем.
func (s *presenceStore) Touch(ctx context.Context, id string, confirmedAt time.Time, ttl time.Duration) error {
	s.called, s.connection, s.confirmedAt, s.ttl = true, id, confirmedAt, ttl
	return nil
}

// TestPresenceRenewalKeepsConfirmedDeadline проверяет, что задержка обработки
// не заменяет исходный момент подтверждения текущим временем, а просроченное
// или отсутствующее подтверждение не продлевает аренду.
//
// @args
//   - t: контекст теста без внешних сервисов.
func TestPresenceRenewalKeepsConfirmedDeadline(t *testing.T) {
	store := &presenceStore{}
	hub := &Hub{repo: presenceRepository{}, store: store, ttl: 5 * time.Second}
	confirmed := time.Now().Add(-4 * time.Second)
	session := domain.Session{ConnectionID: "tab-1", LastSeenAt: confirmed}
	if err := hub.Touch(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if !store.called || store.connection != "tab-1" || !store.confirmedAt.Equal(confirmed) || store.ttl != 5*time.Second {
		t.Fatal("renewal lost the original heartbeat deadline")
	}
	for _, seen := range []time.Time{time.Time{}, time.Now().Add(-6 * time.Second), time.Now().Add(time.Second)} {
		store.called = false
		session.LastSeenAt = seen
		if err := hub.Touch(context.Background(), session); !errors.Is(err, apperrors.ErrNotFound) || store.called {
			t.Fatalf("unconfirmed/expired heartbeat renewed lease: seen=%s err=%v", seen, err)
		}
	}
}
