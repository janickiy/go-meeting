package realtime

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// Engagement проверяет допуск участника и публикует временные emoji-реакции.
// @params:
//   - repo: постоянное хранилище членства и состояния конференции.
//   - hub: координатор доставки событий текущим участникам.
type Engagement struct {
	repo Repository
	hub  *Hub
}

// NewEngagement связывает авторизацию реакций с доставкой событий комнаты.
// @args repo — хранилище членства; hub — координатор событий.
// @return сервис временных реакций.
func NewEngagement(repo Repository, hub *Hub) *Engagement {
	return &Engagement{repo: repo, hub: hub}
}

// Authorize проверяет право отправлять реакции до расходования общего лимита комнаты.
// @args ctx — контекст запроса; conferenceID — конференция; userID — пользователь.
// @return ошибка доступа или nil для допущенного участника активной встречи.
func (s *Engagement) Authorize(ctx context.Context, conferenceID, userID string) error {
	_, err := s.authorized(ctx, conferenceID, userID)
	return err
}

// authorized сверяет сохранённое членство с актуальным составом активной встречи.
// Повторная проверка не позволяет использовать членство, отозванное после первого запроса.
// @args ctx — контекст запроса; conferenceID — конференция; userID — пользователь.
// @return актуальное членство участника и ошибка проверки доступа.
func (s *Engagement) authorized(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	actor, err := s.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return actor, err
	}
	status, roster, err := s.repo.Roster(ctx, conferenceID)
	if err != nil {
		return actor, err
	}
	if status != conferences.Active {
		return actor, apperrors.New(apperrors.ErrConflict, "conference is not active")
	}
	for _, p := range roster {
		if p.ID == actor.ID && p.CanParticipate() {
			return p, nil
		}
	}
	return actor, apperrors.ErrForbidden
}

// Reaction публикует одну из разрешённых реакций от имени текущего участника.
// @args ctx — контекст запроса; conferenceID — конференция; userID — пользователь;
// emoji — одна из разрешённых реакций 👍, 👏, ❤️ или 😂.
// @return ошибка проверки или доставки события; nil означает успех.
func (s *Engagement) Reaction(ctx context.Context, conferenceID, userID, emoji string) error {
	if !domain.AllowedReaction(emoji) {
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported reaction")
	}
	actor, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return err
	}
	return s.hub.Broadcast(ctx, domain.Event("reaction.created", conferenceID, map[string]string{"participantId": actor.ID, "emoji": emoji}))
}
