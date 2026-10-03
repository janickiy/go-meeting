package recordings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// Repository задаёт контракт зависимого компонента Repository в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Start: операция запуск с контрактом, описанным у метода.
//   - Stop: операция остановка с контрактом, описанным у метода.
//   - Accessible: операция Accessible с контрактом, описанным у метода.
//   - List: операция список с контрактом, описанным у метода.
//   - ClaimCommand: получение права на выполнение команды с контрактом, описанным у метода.
//   - CompleteCommand: завершение выполнения команды с контрактом, описанным у метода.
//   - RetryCommand: операция повторная попытка Command с контрактом, описанным у метода.
type Repository interface {
	// Start запускает обработку задач записи и связанных артефактов и подготавливает связанные ресурсы.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (int): значение segmentDuration типа int, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Start(context.Context, string, string, int) (records.Record, bool, error)
	// Stop останавливает активную обработку задач записи и связанных артефактов и освобождает связанные ресурсы.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Stop(context.Context, string, string, string) (records.Record, error)
	// Accessible проверяет связь записи с конференцией и право пользователя читать её.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Accessible(context.Context, string, string, string) (records.Record, error)
	// List возвращает ограниченный список задач записи и связанных артефактов с принятыми в данном слое фильтрами.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (int): предел количества обрабатываемых элементов.
	//   - аргумент 5 (int): число элементов, пропускаемых перед началом страницы.
	//
	// @return:
	//   - результат 1 ([]records.Record): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	List(context.Context, string, string, int, int) ([]records.Record, error)
	// ClaimCommand захватывает очередную команду записи с ограниченным сроком обработки и соблюдением порядка.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (records.OutboxCommand): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ClaimCommand(context.Context) (records.OutboxCommand, records.Record, error)
	// CompleteCommand подтверждает обработку команды только для её действующего владельца.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (records.OutboxCommand): значение для проверки, нормализации или преобразования.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CompleteCommand(context.Context, records.OutboxCommand) error
	// RetryCommand назначает повторную попытку доставки команды после временной ошибки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (records.OutboxCommand): значение для проверки, нормализации или преобразования.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	RetryCommand(context.Context, records.OutboxCommand) error
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

// Commander задаёт контракт зависимого компонента Commander в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - StartRecord: операция запуск запись с контрактом, описанным у метода.
//   - StopRecord: операция остановка запись с контрактом, описанным у метода.
type Commander interface {
	// StartRecord передаёт команду начала записи выбранному транспорту воркера.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID задачи записи.
	//   - аргумент 3 (int): плановая длительность сегмента записи в секундах.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	StartRecord(context.Context, string, int) error
	// StopRecord передаёт команду остановки записи выбранному транспорту воркера.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID задачи записи.
	//   - аргумент 3 (string): причина завершения, отказа или изменения состояния.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	StopRecord(context.Context, string, string) error
}

// Locker задаёт контракт зависимого компонента Locker в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Acquire: операция захват с контрактом, описанным у метода.
//   - Release: операция освобождение с контрактом, описанным у метода.
type Locker interface {
	// Acquire пытается занять блокировку ресурса на ограниченный срок без замены действующего владельца.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Acquire(context.Context, string, string) (bool, error)
	// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Release(context.Context, string, string) error
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

// Service объединяет зависимости прикладного сценария и координирует его операции.
// @params
//   - repo: хранилище постоянных данных прикладного сценария.
//   - reader: источник содержимого либо читатель карточек записи согласно типу.
//   - commands: транспорт доставки управляющих команд записи.
//   - locker: механизм взаимного исключения по ресурсу.
//   - events: получатель или издатель событий прикладного сценария.
type Service struct {
	repo     Repository
	reader   Reader
	commands Commander
	locker   Locker
	events   Events
}

// NewConferenceService создаёт и связывает зависимости компонента ConferenceService, используемого в управлении задачами записи и её артефактами.
//
// @args
//   - repo (Repository): хранилище постоянных данных прикладного сценария.
//   - reader (Reader): источник содержимого либо читатель карточек записи согласно типу.
//   - commands (Commander): транспорт доставки управляющих команд записи.
//   - locker (Locker): механизм взаимного исключения по ресурсу.
//   - events (Events): получатель или издатель событий прикладного сценария.
//
// @return:
//   - результат 1 (*Service): созданный компонент с переданными зависимостями.
func NewConferenceService(repo Repository, reader Reader, commands Commander, locker Locker, events Events) *Service {
	return &Service{repo, reader, commands, locker, events}
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
		s.publish(ctx, record, "recording.starting")
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
		s.publish(ctx, record, "recording.stopping")
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
func (s *Service) publish(ctx context.Context, record records.Record, kind string) {
	if s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode}))
	}
}

// Run выполняет основной цикл компонента до завершения работы или отмены контекста.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 16 {
			op, cancel := context.WithTimeout(ctx, 5*time.Second)
			command, record, err := s.repo.ClaimCommand(op)
			if err != nil {
				cancel()
				break
			}
			if records.IsTerminalStatus(record.Status) {
				err = s.repo.CompleteCommand(op, command)
				cancel()
				if err != nil {
					break
				}
				continue
			}
			if command.CommandType == "record.start" {
				if s.locker != nil {
					var acquired bool
					acquired, err = s.locker.Acquire(op, record.ConferenceID, record.UUID)
					if err == nil && !acquired {
						err = apperrors.ErrConflict
					}
				}
				if err == nil {
					err = s.commands.StartRecord(op, record.UUID, record.SegmentDurationSec)
				}
				if err == nil && record.Status == records.StatusStarting {
					s.publish(op, record, "recording.starting")
				}
			} else {
				err = s.commands.StopRecord(op, record.UUID, command.Reason)
				if err == nil {
					s.publish(op, record, "recording.stopping")
				}
			}
			if err == nil {
				err = s.repo.CompleteCommand(op, command)
			}
			if err != nil {
				_ = s.repo.RetryCommand(op, command)
				if !errors.Is(err, context.Canceled) {
					slog.Warn("recording command retry", "recording_id", record.UUID, "command", command.CommandType)
				}
			}
			cancel()
			if err != nil {
				break
			}
		}
	}
}
