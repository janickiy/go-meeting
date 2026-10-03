package realtime

import (
	"context"
	"time"

	"github.com/janickiy/go-recorder/internal/operations"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// HandStore задаёт контракт зависимого компонента HandStore в поднятых руках и временных реакциях участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Raise: операция Raise с контрактом, описанным у метода.
//   - Lower: операция Lower с контрактом, описанным у метода.
//   - List: операция список с контрактом, описанным у метода.
type HandStore interface {
	// Raise сохраняет поднятую руку в Redis с ограничением количества и времени хранения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор членства участника внутри конференции.
	//
	// @return:
	//   - результат 1 (domain.Hand): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Raise(context.Context, string, string) (domain.Hand, bool, error)
	// Lower удаляет активную поднятую руку из Redis.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор членства участника внутри конференции.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Lower(context.Context, string, string) (bool, error)
	// List возвращает ограниченный список поднятых рук и реакций комнаты с принятыми в данном слое фильтрами.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 ([]domain.Hand): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	List(context.Context, string) ([]domain.Hand, error)
}

// Engagement координирует авторизацию, Redis-состояние рук и публикацию временных реакций.
//   - repo: хранилище постоянных данных прикладного сценария.
//   - hands: временное хранилище поднятых рук в Redis.
//   - hub: координатор присутствия и доставки событий комнаты.
type Engagement struct {
	handObserver func(context.Context, string, domain.Hand) error
	repo         Repository
	hands        HandStore
	hub          *Hub
}

// SetHandObserver подключает ограниченную по времени запись технического счётчика успешных поднятий руки.
// @args observer — функция сохранения агрегата, не влияющая на права или состояние Redis.
func (s *Engagement) SetHandObserver(observer func(context.Context, string, domain.Hand) error) {
	s.handObserver = observer
}

// NewEngagement создаёт и связывает зависимости компонента Engagement, используемого в поднятых руках и временных реакциях участников.
//
// @args
//   - repo (Repository): хранилище постоянных данных прикладного сценария.
//   - hands (HandStore): временное хранилище поднятых рук в Redis.
//   - hub (*Hub): координатор присутствия и доставки событий комнаты.
//
// @return:
//   - результат 1 (*Engagement): созданный компонент с переданными зависимостями.
func NewEngagement(repo Repository, hands HandStore, hub *Hub) *Engagement {
	return &Engagement{repo: repo, hands: hands, hub: hub}
}

// Authorize проверяет право пользователя участвовать в операции до работы с защищёнными ресурсами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Engagement) Authorize(ctx context.Context, conferenceID, userID string) error {
	_, _, err := s.authorized(ctx, conferenceID, userID)
	return err
}

// authorized проверяет актуальную активную конференцию и допуск участника, затем возвращает членство и состав комнаты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 ([]conferences.Participant): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Engagement) authorized(ctx context.Context, conferenceID, userID string) (conferences.Participant, []conferences.Participant, error) {
	actor, err := s.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return actor, nil, err
	}
	status, roster, err := s.repo.Roster(ctx, conferenceID)
	if err != nil {
		return actor, nil, err
	}
	if status != conferences.Active {
		return actor, nil, apperrors.New(apperrors.ErrConflict, "conference is not active")
	}
	// Повторно проверяем свежий снимок: вызов Authorize мог совпасть с удалением участника.
	for _, p := range roster {
		if p.ID == actor.ID && p.CanParticipate() {
			return p, roster, nil
		}
	}
	return actor, nil, apperrors.ErrForbidden
}

// List возвращает ограниченный список поднятых рук и реакций комнаты с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 ([]domain.Hand): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Engagement) List(ctx context.Context, conferenceID, userID string) ([]domain.Hand, error) {
	_, roster, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return nil, err
	}
	hands, err := s.hands.List(ctx, conferenceID)
	if err != nil {
		return nil, err
	}
	eligible := map[string]bool{}
	for _, p := range roster {
		eligible[p.ID] = p.CanParticipate()
	}
	items := make([]domain.Hand, 0, len(hands))
	for _, h := range hands {
		if eligible[h.ParticipantID] {
			items = append(items, h)
		}
	}
	return items, nil
}

// Hand меняет состояние руки с проверкой прав участника или модератора.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - participantID (string): идентификатор членства участника внутри конференции.
//   - raised (bool): true поднимает руку, false опускает её.
//
// @return:
//   - результат 1 (*domain.Hand): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Engagement) Hand(ctx context.Context, conferenceID, userID, participantID string, raised bool) (*domain.Hand, error) {
	actor, roster, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return nil, err
	}
	var target *conferences.Participant
	for _, p := range roster {
		if p.ID == participantID {
			copy := p
			target = &copy
			break
		}
	}
	if target == nil || !target.CanParticipate() {
		return nil, apperrors.ErrNotFound
	}
	if actor.ID != participantID {
		if raised || !actor.CanAdmit() || (actor.Role == conferences.CoHost && target.Role != conferences.ParticipantRole) {
			return nil, apperrors.ErrForbidden
		}
	}
	if raised {
		hand, changed, err := s.hands.Raise(ctx, conferenceID, participantID)
		if err != nil {
			return nil, err
		}
		if changed {
			if s.handObserver != nil {
				op, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				err := s.handObserver(op, conferenceID, hand)
				cancel()
				if err != nil {
					operations.Event("analytics_failed")
				}
			}
			_ = s.hub.Broadcast(ctx, domain.Event("hand.raised", conferenceID, hand))
		}
		return &hand, nil
	}
	changed, err := s.hands.Lower(ctx, conferenceID, participantID)
	if err != nil {
		return nil, err
	}
	if changed {
		_ = s.hub.Broadcast(ctx, domain.Event("hand.lowered", conferenceID, map[string]string{"participantId": participantID}))
	}
	return nil, nil
}

// Reaction публикует разрешённую временную реакцию допущенного участника.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - emoji (string): одна из разрешённых временных реакций.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Engagement) Reaction(ctx context.Context, conferenceID, userID, emoji string) error {
	if !domain.AllowedReaction(emoji) {
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported reaction")
	}
	actor, _, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return err
	}
	return s.hub.Broadcast(ctx, domain.Event("reaction.created", conferenceID, map[string]string{"participantId": actor.ID, "emoji": emoji}))
}
