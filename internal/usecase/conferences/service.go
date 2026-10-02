package conferences

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

// repository задаёт контракт зависимого компонента repository в жизненном цикле конференций и правах участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Create: операция создание с контрактом, описанным у метода.
//   - Get: операция получение с контрактом, описанным у метода.
//   - GetByInvite: операция получение By Invite с контрактом, описанным у метода.
//   - ListForUser: операция список для пользователь с контрактом, описанным у метода.
//   - Membership: операция Membership с контрактом, описанным у метода.
//   - Participants: операция Participants с контрактом, описанным у метода.
//   - Transition: операция переход с контрактом, описанным у метода.
//   - Join: операция Join с контрактом, описанным у метода.
//   - Leave: операция Leave с контрактом, описанным у метода.
type repository interface {
	// Create создаёт новое состояние конференций и членств участников по переданным параметрам.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Conference): конференция либо её идентификатор, ограничивающий область операции.
	//   - аргумент 3 (domain.Participant): значение owner типа domain.Participant, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Create(context.Context, domain.Conference, domain.Participant) (domain.Conference, error)
	// Get читает состояние конференций и членств участников для дальнейшей обработки или ответа.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Get(context.Context, string) (domain.Conference, error)
	// GetByInvite находит конференцию по действующему коду приглашения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): код приглашения или машинный код результата.
	//
	// @return:
	//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetByInvite(context.Context, string) (domain.Conference, error)
	// ListForUser возвращает конференции, доступные указанному пользователю.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (int): предел количества обрабатываемых элементов.
	//   - аргумент 4 (int): число элементов, пропускаемых перед началом страницы.
	//
	// @return:
	//   - результат 1 ([]domain.Conference): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ListForUser(context.Context, string, int, int) ([]domain.Conference, error)
	// Membership читает членство пользователя в заданной конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (domain.Participant): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Membership(context.Context, string, string) (domain.Participant, error)
	// Participants возвращает разрешённую страницу участников конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (int): предел количества обрабатываемых элементов.
	//   - аргумент 4 (int): число элементов, пропускаемых перед началом страницы.
	//
	// @return:
	//   - результат 1 ([]domain.Participant): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Participants(context.Context, string, int, int) ([]domain.Participant, error)
	// Transition выполняет разрешённый переход состояния конференции или записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 4 (domain.Status): целевой объект, участник или состояние операции.
	//
	// @return:
	//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Transition(context.Context, string, string, domain.Status) (domain.Conference, error)
	// Join создаёт или восстанавливает членство участника, учитывая приглашение, состояние встречи и зал ожидания.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (users.User): пользователь либо его идентификатор, определяющий область доступа.
	//   - аргумент 4 (string): криптографически случайный код приглашения, не заменяющий авторизацию.
	//
	// @return:
	//   - результат 1 (domain.Participant): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Join(context.Context, string, users.User, string) (domain.Participant, error)
	// Leave фиксирует выход участника, сохраняя историю членства и состояние допуска.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (domain.Participant): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Leave(context.Context, string, string) (domain.Participant, error)
}

// userRepository задаёт контракт зависимого компонента userRepository в жизненном цикле конференций и правах участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - GetByID: операция получение By ID с контрактом, описанным у метода.
type userRepository interface {
	// GetByID читает учётную запись по её идентификатору.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetByID(context.Context, string) (users.User, error)
}

// Service объединяет зависимости прикладного сценария и координирует его операции.
// @params
//   - repository: хранилище постоянных данных прикладного сценария.
//   - users: хранилище учётных записей пользователей.
//   - invite: операция invite с контрактом, описанным у метода.
//   - observer: получатель сохранённых изменений конференции или закрытия сессии.
type Service struct {
	repository repository
	users      userRepository
	invite     func() (string, error)
	observer   interface{ ConferenceChanged(context.Context, string) }
}

// NewService создаёт и связывает зависимости компонента Service, используемого в жизненном цикле конференций и правах участников.
//
// @args
//   - repository (repository): хранилище постоянных данных прикладного сценария.
//   - users (userRepository): хранилище учётных записей пользователей.
//   - invite (func() (string, error)): вызываемый обработчик «invite» с контрактом, указанным в типе.
//
// @return:
//   - результат 1 (*Service): созданный компонент с переданными зависимостями.
func NewService(repository repository, users userRepository, invite func() (string, error)) *Service {
	return &Service{repository: repository, users: users, invite: invite}
}

// SetObserver подключает обработчик изменений конференции при сборке приложения.
//
// @args
//   - observer (interface{ ConferenceChanged(context.Context, string) }): получатель сохранённых изменений конференции или закрытия сессии.
func (s *Service) SetObserver(observer interface{ ConferenceChanged(context.Context, string) }) {
	s.observer = observer
}

// changed сообщает зависимым обработчикам об изменении локального или сохранённого состояния.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
func (s *Service) changed(ctx context.Context, id string) {
	if s.observer != nil {
		s.observer.ConferenceChanged(ctx, id)
	}
}

// Create создаёт новое состояние конференций и членств участников по переданным параметрам.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - request (domain.CreateRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.View): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Create(ctx context.Context, userID string, request domain.CreateRequest) (domain.View, error) {
	if err := domain.ValidateSchedule(request.ScheduledAt, request.PlannedDurationMin, time.Now()); err != nil {
		return domain.View{}, err
	}
	title, err := domain.NormalizeTitle(request.Title)
	if err != nil {
		return domain.View{}, err
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.View{}, err
	}
	for range 5 {
		code, err := s.invite()
		if err != nil {
			return domain.View{}, err
		}
		if !validInvite(code) {
			return domain.View{}, fmt.Errorf("invalid generated invite code")
		}
		conference := domain.Conference{ID: uuid.NewString(), OwnerID: user.ID, Title: title, InviteCode: code, Status: domain.Created,
			WaitingRoomEnabled: request.WaitingRoomEnabled, ScheduledAt: request.ScheduledAt, PlannedDurationMin: request.PlannedDurationMin}
		if conference.ScheduledAt != nil {
			at := conference.ScheduledAt.UTC()
			conference.ScheduledAt = &at
			conference.Status = domain.Scheduled
		}
		ownerID := user.ID
		owner := domain.Participant{ID: uuid.NewString(), ConferenceID: conference.ID, UserID: &ownerID,
			DisplayName: user.ParticipantName(), Role: domain.Owner, Status: domain.Left, AdmissionState: domain.AdmissionAdmitted}
		created, err := s.repository.Create(ctx, conference, owner)
		if errors.Is(err, domain.ErrInviteCollision) {
			continue
		}
		if err != nil {
			return domain.View{}, err
		}
		return created.View(), nil
	}
	return domain.View{}, fmt.Errorf("unable to generate a unique invite code")
}

// List возвращает ограниченный список конференций и членств участников с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]domain.View): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]domain.View, error) {
	items, err := s.repository.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]domain.View, 0, len(items))
	for _, item := range items {
		view := item.View()
		view.InviteCode, view.InviteURL = "", ""
		views = append(views, view)
	}
	return views, nil
}

// Read читает состояние конференций и членств участников для дальнейшей обработки или ответа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.View): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Read(ctx context.Context, userID, id string) (domain.View, error) {
	conference, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.View{}, err
	}
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return domain.View{}, err
	}
	view := conference.View()
	if !p.CanReadHistory() {
		view.InviteCode, view.InviteURL = "", ""
	}
	return view, nil
}

// Transition выполняет разрешённый переход состояния конференции или записи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - target (domain.Status): целевой объект, участник или состояние операции.
//
// @return:
//   - результат 1 (domain.View): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Transition(ctx context.Context, userID, id string, target domain.Status) (domain.View, error) {
	conference, err := s.repository.Transition(ctx, id, userID, target)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return conference.View(), nil
}

// Participants возвращает разрешённую страницу участников конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]domain.ParticipantView): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Participants(ctx context.Context, userID, id string, limit, offset int) ([]domain.ParticipantView, error) {
	if _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	actor, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if !actor.CanReadHistory() {
		return nil, apperrors.ErrForbidden
	}
	var items []domain.Participant
	if repo, ok := s.repository.(interface {
		ParticipantsVisible(context.Context, string, string, int, int) ([]domain.Participant, error)
	}); ok {
		items, err = repo.ParticipantsVisible(ctx, id, userID, limit, offset)
	} else {
		items, err = s.repository.Participants(ctx, id, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	views := make([]domain.ParticipantView, 0, len(items))
	for _, item := range items {
		if !item.IsAdmitted() && actor.Role != domain.Owner && actor.Role != domain.CoHost {
			continue
		}
		views = append(views, item.View())
	}
	return views, nil
}

// Join создаёт или восстанавливает членство участника, учитывая приглашение, состояние встречи и зал ожидания.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - request (domain.JoinRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Join(ctx context.Context, userID, id string, request domain.JoinRequest) (domain.ParticipantView, error) {
	if request.InviteCode != "" && !validInvite(request.InviteCode) {
		return domain.ParticipantView{}, apperrors.New(apperrors.ErrInvalidInput, "invalid inviteCode")
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	participant, err := s.repository.Join(ctx, id, user, request.InviteCode)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	if participant.Status == domain.Waiting {
		s.event(ctx, "participant.waiting", id, participant)
	}
	return participant.View(), nil
}

// Leave фиксирует выход участника, сохраняя историю членства и состояние допуска.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Leave(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	participant, err := s.repository.Leave(ctx, id, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	return participant.View(), nil
}

// LookupInvite находит ограниченные сведения о конференции по коду приглашения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - code (string): код приглашения или машинный код результата.
//
// @return:
//   - результат 1 (domain.InviteView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) LookupInvite(ctx context.Context, code string) (domain.InviteView, error) {
	if !validInvite(code) {
		return domain.InviteView{}, apperrors.ErrNotFound
	}
	conference, err := s.repository.GetByInvite(ctx, code)
	if err != nil {
		return domain.InviteView{}, err
	}
	return domain.InviteView{ID: conference.ID, Title: conference.Title, Status: conference.Status, WaitingRoomEnabled: conference.WaitingRoomEnabled, ScheduledAt: conference.ScheduledAt}, nil
}

// JoinInvite присоединяет авторизованного пользователя по коду приглашения с сохранением существующего членства.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - code (string): код приглашения или машинный код результата.
//
// @return:
//   - результат 1 (domain.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) JoinInvite(ctx context.Context, userID, code string) (domain.ParticipantView, error) {
	invite, err := s.LookupInvite(ctx, code)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	return s.Join(ctx, userID, invite.ID, domain.JoinRequest{InviteCode: code})
}

// currentUser читает учётную запись и подготавливает сведения участника текущей операции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (users.User): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) currentUser(ctx context.Context, id string) (users.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	return user, err
}

// event формирует серверное событие сохранённого изменения членства конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - participant (domain.Participant): значение participant типа domain.Participant, используемое согласно назначению этой операции.
func (s *Service) event(ctx context.Context, kind, id string, participant domain.Participant) {
	if events, ok := s.observer.(interface {
		Broadcast(context.Context, realtime.Envelope) error
	}); ok {
		_ = events.Broadcast(ctx, realtime.Event(kind, id, map[string]any{"participant": participant.View()}))
	}
}

// validInvite проверяет форму кода приглашения перед обращением к хранилищу.
//
// @args
//   - code (string): код приглашения или машинный код результата.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func validInvite(code string) bool {
	if len(code) != 32 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(code)
	return err == nil && len(decoded) == 24
}
