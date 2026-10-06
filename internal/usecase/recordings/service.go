package recordings

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// Repository is the atomic conference recording API boundary. Delivery of its
// durable commands belongs to CommandRepository, not the request service.
type Repository interface {
	Start(context.Context, string, string, int) (records.Record, bool, error)
	Stop(context.Context, string, string, string) (records.Record, error)
	Accessible(context.Context, string, string, string) (records.Record, error)
	List(context.Context, string, string, int, int) ([]records.Record, error)
}

// Reader задаёт контракт зависимого компонента Reader в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - ReadComposite: операция чтение общая запись с контрактом, описанным у метода.
type Reader interface {
	// ReadComposite читает карточку общей записи после проверки доступа на уровне сценария конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ReadComposite(context.Context, string) (records.RecordCard, error)
}

// Events задаёт контракт зависимого компонента Events в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Broadcast: операция Broadcast с контрактом, описанным у метода.
type Events interface {
	// Broadcast публикует доверенное событие для разрешённых получателей конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (realtime.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Broadcast(context.Context, realtime.Envelope) error
}

// Service handles authorized recording requests and reads. It owns no polling
// goroutine, queue connection or distributed lock.
type Service struct {
	repo   Repository
	reader Reader
	events Events
}

func NewConferenceService(repo Repository, reader Reader, events Events) *Service {
	return &Service{repo: repo, reader: reader, events: events}
}

// Start запускает обработку задач записи и связанных артефактов и подготавливает связанные ресурсы.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - request (records.ConferenceStartRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Start(ctx context.Context, userID, conferenceID string, request records.ConferenceStartRequest) (records.RecordCard, error) {
	if request.Mode == "" {
		request.Mode = records.ModeComposite
	}
	if !records.ValidConferenceMode(request.Mode) {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	seconds := request.SegmentDurationSec
	if seconds == 0 {
		seconds = 5
	}
	if seconds < 2 || seconds > 30 {
		return records.RecordCard{}, apperrors.New(apperrors.ErrInvalidInput, "segmentDurationSec must be between 2 and 30")
	}
	var record records.Record
	var created bool
	var err error
	if modes, ok := s.repo.(interface {
		StartMode(context.Context, string, string, int, string) (records.Record, bool, error)
	}); ok {
		record, created, err = modes.StartMode(ctx, userID, conferenceID, seconds, request.Mode)
	} else if request.Mode == records.ModeComposite {
		record, created, err = s.repo.Start(ctx, userID, conferenceID, seconds)
	} else {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	if err != nil {
		return records.RecordCard{}, err
	}
	if created {
		publish(s.events, ctx, record, "recording.starting")
	}
	return s.card(ctx, record.UUID)
}

// Stop останавливает активную обработку задач записи и связанных артефактов и освобождает связанные ресурсы.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Stop(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	if record.Status == records.StatusStopping {
		publish(s.events, ctx, record, "recording.stopping")
	}
	return s.card(ctx, record.UUID)
}

// Read читает состояние задач записи и связанных артефактов для дальнейшей обработки или ответа.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Read(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Accessible(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	return s.card(ctx, record.UUID)
}

// List возвращает ограниченный список задач записи и связанных артефактов с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//   - offset (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]records.RecordCard): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.RecordCard, error) {
	rows, err := s.repo.List(ctx, userID, conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	if batch, ok := s.reader.(interface {
		ReadComposites(context.Context, []string) ([]records.RecordCard, error)
	}); ok {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UUID)
		}
		items, err := batch.ReadComposites(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].Status = records.PublicStatus(items[i].Status)
		}
		return items, nil
	}
	items := make([]records.RecordCard, 0, len(rows))
	for _, record := range rows {
		card, err := s.card(ctx, record.UUID)
		if err != nil {
			return nil, err
		}
		items = append(items, card)
	}
	return items, nil
}

// card читает и собирает разрешённую карточку записи конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) card(ctx context.Context, id string) (records.RecordCard, error) {
	card, err := s.reader.ReadComposite(ctx, id)
	card.Status = records.PublicStatus(card.Status)
	return card, err
}

// publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
func publish(events Events, ctx context.Context, record records.Record, kind string) {
	if events != nil {
		_ = events.Broadcast(ctx, realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode, "requestedBy": record.RequestedBy}))
	}
}
