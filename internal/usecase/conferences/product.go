package conferences

import (
	"context"
	"errors"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
)

// productRepository задаёт контракт зависимого компонента productRepository в жизненном цикле конференций и правах участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - DecideAdmission: операция Decide допуск с контрактом, описанным у метода.
//   - UpdateSchedule: операция обновление расписание с контрактом, описанным у метода.
//   - Timeline: операция Timeline с контрактом, описанным у метода.
//   - History: операция история с контрактом, описанным у метода.
type productRepository interface {
	// DecideAdmission сериализует решение допуска блокировкой конференции, сохраняет состояние и версию решения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 4 (string): идентификатор членства участника внутри конференции.
	//   - аргумент 5 (domain.AdmissionRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (domain.Participant): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	DecideAdmission(context.Context, string, string, string, domain.AdmissionRequest) (domain.Participant, error)
	// UpdateSchedule обновляет расписание только запланированной встречи под той же блокировкой, что используется при старте.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 4 (domain.ScheduleRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (domain.Conference): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	UpdateSchedule(context.Context, string, string, domain.ScheduleRequest) (domain.Conference, error)
	// Timeline возвращает страницу встреч текущего пользователя с фильтрами будущих, активных и прошедших встреч.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (domain.TimelineQuery): параметры выборки либо SQL-текст выполняемого запроса.
	//
	// @return:
	//   - результат 1 ([]domain.Conference): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Timeline(context.Context, string, domain.TimelineQuery) ([]domain.Conference, error)
	// History собирает сведения завершённой встречи, историю участников и сводку записей с проверкой доступа.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//
	// @return:
	//   - результат 1 (domain.HistoryView): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	History(context.Context, string, string) (domain.HistoryView, error)
}

// Self возвращает собственное членство пользователя, включая состояние ожидания и решение о допуске.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Self(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	return p.View(), err
}

// Admission обрабатывает решение о допуске или отказе с проверкой полномочий организатора.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - participantID (string): идентификатор членства участника внутри конференции.
//   - request (domain.AdmissionRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Admission(ctx context.Context, userID, id, participantID string, request domain.AdmissionRequest) (domain.ParticipantView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.ParticipantView{}, apperrors.ErrUnavailable
	}
	p, err := repo.DecideAdmission(ctx, id, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	kind := "participant.admitted"
	if request.Decision == "reject" {
		kind = "participant.rejected"
	}
	s.event(ctx, kind, id, p)
	return p.View(), nil
}

// Schedule обновляет расписание запланированной встречи с проверкой полномочий владельца.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - request (domain.ScheduleRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.View): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Schedule(ctx context.Context, userID, id string, request domain.ScheduleRequest) (domain.View, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.View{}, apperrors.ErrUnavailable
	}
	c, err := repo.UpdateSchedule(ctx, id, userID, request)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return c.View(), nil
}

// Timeline возвращает страницу встреч текущего пользователя с фильтрами будущих, активных и прошедших встреч.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - query (domain.TimelineQuery): параметры выборки либо SQL-текст выполняемого запроса.
//
// @return:
//   - результат 1 (domain.TimelinePage): страница элементов и метаданные продолжения.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Timeline(ctx context.Context, userID string, query domain.TimelineQuery) (domain.TimelinePage, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.TimelinePage{}, apperrors.ErrUnavailable
	}
	rows, err := repo.Timeline(ctx, userID, query)
	if err != nil {
		return domain.TimelinePage{}, err
	}
	page := domain.TimelinePage{Items: []domain.View{}}
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
		cursor := domain.EncodeTimelineCursor(rows[len(rows)-1])
		page.NextCursor = &cursor
	}
	for _, c := range rows {
		// Приглашения не нужны для просмотра списка. Сохраняем приватность ожидающего членства
		// без отдельного запроса членства для каждой конференции.
		view := c.View()
		view.InviteCode, view.InviteURL = "", ""
		page.Items = append(page.Items, view)
	}
	return page, nil
}

// History собирает сведения завершённой встречи, историю участников и сводку записей с проверкой доступа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.HistoryView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) History(ctx context.Context, userID, id string) (domain.HistoryView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.HistoryView{}, apperrors.ErrUnavailable
	}
	return repo.History(ctx, id, userID)
}
