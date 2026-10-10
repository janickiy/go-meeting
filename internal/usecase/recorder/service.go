package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/janickiy/meet-space/internal/infrastructure/ffmpeg"
	localstorage "github.com/janickiy/meet-space/internal/infrastructure/storage/local"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
	webrtcingest "github.com/janickiy/meet-space/internal/infrastructure/webrtc"
)

// apiRepository задаёт контракт зависимого компонента apiRepository в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Create: операция создание с контрактом, описанным у метода.
//   - FindByUUID: поиск записи по UUID с контрактом, описанным у метода.
//   - MarkStopping: перевод записи в состояние остановки с контрактом, описанным у метода.
//   - MarkFailed: перевод записи в состояние ошибки с контрактом, описанным у метода.
//   - ListDetails: операция список Details с контрактом, описанным у метода.
//   - ListSummaryDetailsByConferenceIDs: получение кратких сведений о записях по идентификаторам конференций с контрактом, описанным у метода.
//   - FindDetailsByUUID: получение подробных сведений о записи по UUID с контрактом, описанным у метода.
type apiRepository interface {
	// Create создаёт новое состояние задач записи и связанных артефактов по переданным параметрам.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - record (records.Record): задача записи с её сохранённым состоянием.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Create(ctx context.Context, record records.Record) (records.Record, error)
	// FindByUUID читает задачу записи по её внешнему UUID.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordUUID (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	FindByUUID(ctx context.Context, recordUUID string) (records.Record, error)
	// MarkStopping условно переводит задачу записи в состояние «Stopping», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordUUID (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//   - reason (string): причина завершения, отказа или изменения состояния.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkStopping(ctx context.Context, recordUUID string, reason string) error
	// MarkFailed условно переводит задачу записи в состояние «Failed», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordUUID (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkFailed(ctx context.Context, recordUUID string, cause error) error
	// ListDetails читает записи вместе со связанными артефактами и событиями.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - limit (int): максимальное число элементов страницы или порции обработки.
	//   - offset (int): число элементов, пропускаемых перед началом страницы.
	//
	// @return:
	//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ListDetails(ctx context.Context, limit int, offset int) ([]records.RecordDetails, error)
	// ListSummaryDetailsByConferenceIDs пакетно читает краткие сведения записей нескольких конференций без запроса для каждой записи.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - conferenceIDs ([]string): идентификаторы конференций для пакетной выборки.
	//   - status (string): состояние ресурса, ответа или фильтра выборки.
	//
	// @return:
	//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ListSummaryDetailsByConferenceIDs(ctx context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error)
	// FindDetailsByUUID читает задачу записи и связанные сведения по её UUID.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordUUID (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//
	// @return:
	//   - результат 1 (records.RecordDetails): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	FindDetailsByUUID(ctx context.Context, recordUUID string) (records.RecordDetails, error)
}

// workerCommander задаёт контракт зависимого компонента workerCommander в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - StartRecord: операция запуск запись с контрактом, описанным у метода.
//   - StopRecord: операция остановка запись с контрактом, описанным у метода.
type workerCommander interface {
	// StartRecord передаёт команду начала записи выбранному транспорту воркера.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordID (string): внешний UUID задачи записи.
	//   - segmentDurationSec (int): плановая длительность сегмента записи в секундах.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error
	// StopRecord передаёт команду остановки записи выбранному транспорту воркера.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordID (string): внешний UUID задачи записи.
	//   - reason (string): причина завершения, отказа или изменения состояния.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	StopRecord(ctx context.Context, recordID string, reason string) error
}

// conferenceLocker задаёт контракт зависимого компонента conferenceLocker в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Acquire: операция захват с контрактом, описанным у метода.
//   - Release: операция освобождение с контрактом, описанным у метода.
type conferenceLocker interface {
	// Acquire пытается занять блокировку ресурса на ограниченный срок без замены действующего владельца.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
	//   - recordID (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error)
	// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
	//   - recordID (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Release(ctx context.Context, conferenceID string, recordID string) error
}

// workerRepository задаёт контракт зависимого компонента workerRepository в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - ingestFailureRepository: встроенный тип, добавляющий свой контракт или данные.
//   - MarkFinalizing: перевод записи в состояние завершения обработки с контрактом, описанным у метода.
//   - MarkUploading: перевод записи в состояние загрузки с контрактом, описанным у метода.
//   - SaveFinalArtifacts: операция сохранение итоговый артефакты с контрактом, описанным у метода.
type workerRepository interface {
	ingestFailureRepository
	// MarkFinalizing условно переводит задачу записи в состояние «Finalizing», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkFinalizing(context.Context, string) error
	// MarkUploading условно переводит задачу записи в состояние «Uploading», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkUploading(context.Context, string) error
	// SaveFinalArtifacts сохраняет итоговый файл, превью и сегменты после успешной обработки записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//   - аргумент 3 (records.RecordFile): значение finalFile типа records.RecordFile, используемое согласно назначению этой операции.
	//   - аргумент 4 (*records.RecordFile): значение previewFile типа *records.RecordFile, используемое согласно назначению этой операции.
	//   - аргумент 5 ([]records.RecordSegment): доступные сегменты записи для итоговой сборки.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

// mediaIngest задаёт контракт зависимого компонента mediaIngest в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Prepare: операция Prepare с контрактом, описанным у метода.
//   - Stop: операция остановка с контрактом, описанным у метода.
//   - HandleOffer: обработка SDP-предложения с контрактом, описанным у метода.
type mediaIngest interface {
	// Prepare подготавливает состояние WebRTC-приёма конкретной записи до обмена SDP.
	//
	// @args
	//   - аргумент 1 (string): внешний UUID задачи записи.
	//   - аргумент 2 (int): плановая длительность сегмента записи в секундах.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Prepare(string, int) error
	// Stop останавливает активную обработку задач записи и связанных артефактов и освобождает связанные ресурсы.
	//
	// @args
	//   - аргумент 1 (string): внешний UUID задачи записи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Stop(string) error
	// HandleOffer обрабатывает SDP-предложение и возвращает SDP-ответ приёмника записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID задачи записи.
	//   - аргумент 3 (records.WebRTCOfferRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (records.WebRTCAnswerResponse): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Service объединяет зависимости прикладного сценария и координирует его операции.
//   - repository: хранилище постоянных данных прикладного сценария.
//   - workerCommander: транспорт команды API к воркеру записи.
//   - conferenceLocker: Redis-блокировка одной активной записи на встречу.
//   - s3: клиент приватного объектного хранилища MinIO/S3.
type Service struct {
	repository       apiRepository
	workerCommander  workerCommander
	conferenceLocker conferenceLocker
	s3               *s3storage.Client
}

// WorkerService обрабатывает команды записи, входящие медиа, финализацию и загрузку артефактов.
// @params
//   - repository: хранилище постоянных данных прикладного сценария.
//   - postProcessor: компонент FFmpeg для сборки итогового файла и превью.
//   - ingest: значение ingest типа mediaIngest, используемое согласно назначению этой операции.
//   - s3: клиент приватного объектного хранилища MinIO/S3.
//   - storagePath: корневой каталог локального хранения артефактов записи.
//   - workerID: идентификатор воркера-владельца операции.
//   - conferenceLocker: Redis-блокировка одной активной записи на встречу.
//   - commandsMu: значение commandsMu типа sync.Mutex, используемое согласно назначению этой операции.
//   - commands: транспорт доставки управляющих команд записи.
//   - composite: значение composite типа *CompositeService, используемое согласно назначению этой операции.
type WorkerService struct {
	repository       workerRepository
	postProcessor    *ffmpeg.PostProcessor
	ingest           mediaIngest
	s3               *s3storage.Client
	storagePath      string
	workerID         string
	conferenceLocker conferenceReleaser
	commandsMu       sync.Mutex
	commands         map[string]*recordCommandLock
	composite        *CompositeService
}

// SetComposite подключает сценарий общей записи конференции к обработчику команд записи.
//
// @args
//   - service (*CompositeService): значение service типа *CompositeService, используемое согласно назначению этой операции.
func (s *WorkerService) SetComposite(service *CompositeService) { s.composite = service }

// ValidateLegacyRecord запрещает старым неавторизованным маршрутам воркера управлять защищённой записью конференции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) ValidateLegacyRecord(ctx context.Context, id string) error {
	record, err := s.repository.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if records.IsComposite(record) {
		return records.ErrRecordStateChanged
	}
	return nil
}

// recordCommandLock представляет блокировку команды записи для управления задачами и артефактами.
// @params:
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - refs: значение refs типа int, используемое согласно назначению этой операции.
type recordCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewService создаёт прикладной сервис API.
// @args
// - repository: repository записей.
// - workerCommander: транспорт команд recorder-worker; используется издатель RabbitMQ.
// - s3: MinIO/S3-клиент для генерации ссылок на артефакты.
// - conferenceLocker: блокировка Redis, запрещающая параллельную запись одной конференции.
// @return Service.
func NewService(repository apiRepository, workerCommander workerCommander, s3 *s3storage.Client, conferenceLocker conferenceLocker) *Service {
	return &Service{
		repository:       repository,
		workerCommander:  workerCommander,
		conferenceLocker: conferenceLocker,
		s3:               s3,
	}
}

// NewWorkerService создаёт прикладной сервис записывающего воркера.
// @args
// - repository: repository записей.
// - postProcessor: компонент постобработки FFmpeg.
// - s3: MinIO/S3-клиент.
// - storagePath: локальный том хранения.
// - workerID: идентификатор worker-а.
// @return WorkerService.
func NewWorkerService(repository workerRepository, postProcessor *ffmpeg.PostProcessor, ingest mediaIngest, s3 *s3storage.Client, storagePath string, workerID string, locker conferenceReleaser) *WorkerService {
	return &WorkerService{
		repository:       repository,
		postProcessor:    postProcessor,
		ingest:           ingest,
		s3:               s3,
		storagePath:      storagePath,
		workerID:         workerID,
		conferenceLocker: locker,
	}
}

// Start создаёт запись и публикует команду подготовки приёма WebRTC в RabbitMQ.
// @args
// - ctx: контекст HTTP-запроса.
// - request: параметры записи.
// @return response с recordId или ошибку.
func (s *Service) Start(ctx context.Context, request records.StartRequest) (records.StartResponse, error) {
	if guard, ok := s.repository.(interface {
		LegacyConferenceAllowed(context.Context, string) error
	}); ok {
		if err := guard.LegacyConferenceAllowed(ctx, request.ConferenceID); err != nil {
			return records.StartResponse{}, err
		}
	}
	request = records.NormalizeStartRequest(request)
	if request.SegmentDurationSec <= 0 {
		request.SegmentDurationSec = 5
	}
	videoSettings := records.VideoSettingsForQuality(request.Quality)
	recordID := uuid.NewString()
	locked, err := s.acquireConferenceLock(ctx, request.ConferenceID, recordID)
	if err != nil {
		return records.StartResponse{}, err
	}
	if !locked {
		return records.StartResponse{}, records.ErrConferenceAlreadyRecording
	}
	releaseLock := true
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.

	 */func() {
		if releaseLock {
			_ = s.releaseConferenceLock(context.Background(), request.ConferenceID, recordID)
		}
	}()

	record, err := s.repository.Create(ctx, records.Record{
		UUID:               recordID,
		ConferenceID:       request.ConferenceID,
		RequestedBy:        request.RequestedBy,
		SourceType:         "browser",
		TransportType:      "webrtc",
		Status:             records.StatusStarting,
		QualityMode:        request.QualityMode,
		SegmentDurationSec: request.SegmentDurationSec,
		NeedPreview:        true,
	})
	if err != nil {
		return records.StartResponse{}, err
	}

	if err := s.workerCommander.StartRecord(ctx, record.UUID, request.SegmentDurationSec); err != nil {
		_ = s.repository.MarkFailed(ctx, record.UUID, err)
		return records.StartResponse{}, err
	}
	releaseLock = false

	return records.StartResponse{
		Status:       record.Status,
		Message:      "Record job accepted",
		RecordID:     record.UUID,
		ConferenceID: record.ConferenceID,
		WebRTC: records.WebRTCInfo{
			OfferURL: "/api/v1/records/" + record.UUID + "/webrtc/offer",
			ICEServers: []records.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			Video: videoSettings,
		},
	}, nil
}

// Stop помечает запись stopping и публикует в RabbitMQ команду завершения.
// @args
// - ctx: контекст HTTP-запроса.
// - request: recordId и reason.
// @return ошибку БД или публикации RabbitMQ-команды.
func (s *Service) Stop(ctx context.Context, request records.EndRequest) error {
	record, err := s.repository.FindByUUID(ctx, request.RecordID)
	if err != nil {
		return err
	}
	if records.IsComposite(record) {
		return apperrors.ErrNotFound
	}
	if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
		return nil
	}
	if record.Status != records.StatusStopping {
		if err := s.repository.MarkStopping(ctx, request.RecordID, request.Reason); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				return nil
			}
			return err
		}
	}
	// Повтор в состоянии stopping заново публикует команду после прежнего сбоя публикации.
	// Только воркер знает, когда медиа действительно остановлены и можно освободить блокировку.
	return s.workerCommander.StopRecord(ctx, request.RecordID, request.Reason)
}

// List возвращает список записей с файлами из MinIO и метаданными сегментов.
// @args
// - ctx: контекст HTTP-запроса.
// - limit: количество.
// - offset: смещение.
// @return список карточек записей или ошибку БД/MinIO.
func (s *Service) List(ctx context.Context, limit int, offset int) ([]records.RecordCard, error) {
	details, err := s.repository.ListDetails(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]records.RecordCard, 0, len(details))
	for _, item := range details {
		if records.IsComposite(item.Record) {
			continue
		}
		card, err := s.recordCard(ctx, item)
		if err != nil {
			return nil, err
		}
		result = append(result, card)
	}

	return result, nil
}

// CountByConference возвращает количество записей и краткие карточки записей для каждой переданной конференции.
// @args
// - ctx: контекст HTTP-запроса.
// - conferenceIDs: список UUID конференций.
// - status: optional фильтр по статусу записи.
// @return список conferenceId + recordsCount + records[] или ошибку БД/MinIO.
func (s *Service) CountByConference(ctx context.Context, conferenceIDs []string, status string) ([]records.ConferenceRecordSummary, error) {
	details, err := s.repository.ListSummaryDetailsByConferenceIDs(ctx, conferenceIDs, status)
	if err != nil {
		return nil, err
	}

	summaryIndexesByConference := make(map[string]int, len(conferenceIDs))
	result := make([]records.ConferenceRecordSummary, 0, len(conferenceIDs))
	for _, conferenceID := range conferenceIDs {
		if _, exists := summaryIndexesByConference[conferenceID]; exists {
			continue
		}
		summary := records.ConferenceRecordSummary{
			ConferenceID: conferenceID,
			Records:      []records.ConferenceRecordItem{},
		}
		result = append(result, summary)
		summaryIndexesByConference[conferenceID] = len(result) - 1
	}

	for _, item := range details {
		if records.IsComposite(item.Record) {
			continue
		}
		index, ok := summaryIndexesByConference[item.Record.ConferenceID]
		if !ok {
			continue
		}
		card, err := s.conferenceRecordItem(ctx, item)
		if err != nil {
			return nil, err
		}
		summary := &result[index]
		summary.Records = append(summary.Records, card)
		summary.RecordsCount = int64(len(summary.Records))
	}

	return result, nil
}

// Read возвращает карточку записи по UUID.
// @args
// - ctx: контекст HTTP-запроса.
// - uuid: UUID записи.
// @return карточку записи или ошибку БД/MinIO.
func (s *Service) Read(ctx context.Context, uuid string) (records.RecordCard, error) {
	details, err := s.repository.FindDetailsByUUID(ctx, uuid)
	if err != nil {
		return records.RecordCard{}, err
	}
	if records.IsComposite(details.Record) {
		return records.RecordCard{}, apperrors.ErrNotFound
	}

	return s.recordCard(ctx, details)
}

// acquireConferenceLock ставит lock на активную запись конференции.
// @args
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID создаваемой записи.
// @return true, если блокировка получена.
func (s *Service) acquireConferenceLock(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if s.conferenceLocker == nil {
		return true, nil
	}

	return s.conferenceLocker.Acquire(ctx, conferenceID, recordID)
}

// releaseConferenceLock снимает lock активной записи конференции.
// @args
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// @return ошибку Redis.
func (s *Service) releaseConferenceLock(ctx context.Context, conferenceID string, recordID string) error {
	if s.conferenceLocker == nil {
		return nil
	}

	return s.conferenceLocker.Release(ctx, conferenceID, recordID)
}

// HandleCommand исполняет внутреннюю команду worker-а.
// @args
// - ctx: контекст worker-а.
// - command: record.start или record.stop.
// @return ошибку обработки команды.
func (s *WorkerService) HandleCommand(ctx context.Context, command records.Command) error {
	// Команды поступают к воркеру как напрямую, так и через RabbitMQ.
	// Проверяем recordId до его использования в любом пути файловой системы.
	id, err := uuid.Parse(command.RecordID)
	if err != nil {
		return fmt.Errorf("recordId must be valid UUID: %w", err)
	}
	command.RecordID = id.String()
	if s.composite != nil {
		record, err := s.repository.FindByUUID(ctx, command.RecordID)
		if err != nil {
			return err
		}
		if records.IsComposite(record) {
			return s.composite.HandleCommand(ctx, command, record)
		}
	}
	unlock := s.lockCommand(command.RecordID)
	defer unlock()
	var commandErr error
	switch command.Type {
	case "record.start":
		commandErr = s.handleStart(ctx, command)
	case "record.stop":
		commandErr = s.handleStop(ctx, command)
	default:
		return fmt.Errorf("unknown command type %q", command.Type)
	}
	if errors.Is(commandErr, records.ErrRecordStateChanged) {
		return nil // Более новый переход жизненного цикла уже отменил актуальность этой команды.
	}
	return commandErr
}

// lockCommand сериализует команды одной записи; удаляет блокировку после завершения всех использующих её вызовов.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (func()): функция продолжения или освобождения ресурса с указанным контрактом.
func (s *WorkerService) lockCommand(recordID string) func() {
	s.commandsMu.Lock()
	if s.commands == nil {
		s.commands = make(map[string]*recordCommandLock)
	}
	lock := s.commands[recordID]
	if lock == nil {
		lock = &recordCommandLock{}
		s.commands[recordID] = lock
	}
	lock.refs++
	s.commandsMu.Unlock()
	lock.mu.Lock()
	// Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.
	// Синхронизирует доступ к разделяемому состоянию блокировкой.
	//
	return func() {
		lock.mu.Unlock()
		s.commandsMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.commands, recordID)
		}
		s.commandsMu.Unlock()
	}
}

// handleStart идемпотентно подготавливает приём медиа по команде начала записи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) handleStart(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if record.Status != records.StatusStarting {
		if records.IsTerminalStatus(record.Status) {
			return releaseRecordLock(ctx, s.conferenceLocker, record)
		}
		return nil
	}
	recordDir := s.recordDir(command.RecordID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
	}
	if s.ingest != nil {
		if err := s.ingest.Prepare(command.RecordID, command.SegmentDurationSec); err != nil {
			return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
		}
	}
	if err := s.repository.AddEvent(ctx, command.RecordID, "record.worker.ready", "worker", "info", "worker prepared WebRTC ingest", s.workerID); err != nil {
		// Rabbit may quarantine a repeated error instead of retrying again.
		// Roll back the local allocation before returning that error so an
		// unstarted recording cannot permanently occupy an ingest slot.
		if s.ingest != nil {
			if stopErr := s.ingest.Stop(command.RecordID); stopErr != nil && !errors.Is(stopErr, webrtcingest.ErrNoMedia) {
				return errors.Join(err, stopErr)
			}
		}
		return err
	}

	return nil
}

// failWorkerRecord сохраняет ошибку обработки записи и освобождает принадлежащие ей ресурсы.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordID (string): внешний UUID задачи записи.
//   - eventType (string): значение eventType типа string, используемое согласно назначению этой операции.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) failWorkerRecord(ctx context.Context, recordID string, eventType string, cause error) error {
	err := failRecord(ctx, s.repository, s.conferenceLocker, recordID, s.workerID, eventType, cause)
	s.cleanupEmptyLocalStorageAfterFailure(ctx, recordID)
	return err
}

// handleStop останавливает приём медиа, финализирует и сохраняет артефакты по команде завершения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) handleStop(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		// Подтверждаем повторную доставку без изменения артефактов и окончательного состояния.
		return releaseRecordLock(ctx, s.conferenceLocker, record)
	}
	if s.ingest != nil {
		if err := s.ingest.Stop(command.RecordID); err != nil && !errors.Is(err, webrtcingest.ErrNoMedia) {
			return s.failWorkerRecord(ctx, command.RecordID, "record.ingest.stop.failed", err)
		}
	}
	if err := releaseRecordLock(ctx, s.conferenceLocker, record); err != nil {
		return err
	}
	if err := s.repository.MarkFinalizing(ctx, command.RecordID); err != nil {
		return err
	}
	result, err := s.postProcessor.Finalize(ctx, s.recordDir(command.RecordID))
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.finalize.failed", err)
	}

	if err := s.repository.MarkUploading(ctx, command.RecordID); err != nil {
		return err
	}
	finalKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "final.mp4"))
	previewKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "preview.jpg"))
	finalUpload, err := s.s3.UploadFile(ctx, finalKey, result.FinalPath, "video/mp4")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	previewUpload, err := s.s3.UploadFile(ctx, previewKey, result.PreviewPath, "image/jpeg")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	if err := s.s3.RemovePrefix(ctx, filepath.ToSlash(filepath.Join("records", command.RecordID, "segments"))+"/"); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.segments.cleanup.failed", err)
	}
	segments := segmentMetadata(result.Segments)

	duration := result.DurationSec
	finalSize := result.FinalSizeBytes
	finalChecksum := result.FinalChecksum
	previewSize := result.PreviewSizeBytes
	previewChecksum := result.PreviewChecksum
	previewFile := &records.RecordFile{
		FileType:       records.FileTypePreviewJPG,
		Bucket:         previewUpload.Bucket,
		ObjectKey:      previewUpload.ObjectKey,
		FileName:       "preview.jpg",
		MimeType:       "image/jpeg",
		SizeBytes:      &previewSize,
		ChecksumSHA256: &previewChecksum,
		IsPrimary:      false,
		IsPublic:       false,
	}
	finalFile := records.RecordFile{
		FileType:       records.FileTypeFinalMP4,
		Bucket:         finalUpload.Bucket,
		ObjectKey:      finalUpload.ObjectKey,
		FileName:       "final.mp4",
		MimeType:       "video/mp4",
		SizeBytes:      &finalSize,
		DurationSec:    &duration,
		ChecksumSHA256: &finalChecksum,
		IsPrimary:      true,
		IsPublic:       false,
	}
	if err := s.repository.SaveFinalArtifacts(ctx, command.RecordID, finalFile, previewFile, segments); err != nil {
		return err
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.artifacts.uploaded", "worker", "info", "final.mp4 and preview.jpg uploaded to MinIO", s.workerID)
	if err := s.cleanupLocalStorage(command.RecordID); err != nil {
		_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleanup_failed", "worker", "warning", err.Error(), s.workerID)
		return nil
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleaned", "worker", "info", "local storage cleaned after successful MinIO upload", s.workerID)

	return nil
}

// recordDir строит локальный каталог конкретной задачи записи.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (s *WorkerService) recordDir(recordID string) string {
	return filepath.Join(s.storagePath, "records", recordID)
}

// recordCard собирает карточку записи, связанных файлов и временных ссылок чтения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - details (records.RecordDetails): запись вместе со связанными файлами, сегментами и событиями.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) recordCard(ctx context.Context, details records.RecordDetails) (records.RecordCard, error) {
	card := records.RecordCard{
		Record:   details.Record,
		Files:    make([]records.RecordFileView, 0, len(details.Files)),
		Segments: make([]records.RecordSegmentView, 0, len(details.Segments)),
		Events:   details.Events,
	}
	for _, file := range details.Files {
		view := records.RecordFileView{RecordFile: file}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.RecordCard{}, err
		}
		view.URL = url
		card.Files = append(card.Files, view)
	}

	for _, segment := range details.Segments {
		view := records.RecordSegmentView{RecordSegment: segment}
		if segment.ObjectKey != nil {
			if details.Record.StorageBucket != nil {
				view.Bucket = *details.Record.StorageBucket
			} else if s.s3 != nil {
				view.Bucket = s.s3.Bucket()
			}
			url, err := s.presignedURL(ctx, *segment.ObjectKey)
			if err != nil {
				return records.RecordCard{}, err
			}
			view.URL = url
		}
		card.Segments = append(card.Segments, view)
	}

	return card, nil
}

// conferenceRecordItem формирует краткую карточку записи для маршрута count-by-conference.
// @args
// - details: запись с итоговым файлом и превью, без событий и сегментов.
// @return recordId, ссылки на final/preview и временные поля записи.
func (s *Service) conferenceRecordItem(ctx context.Context, details records.RecordDetails) (records.ConferenceRecordItem, error) {
	record := details.Record
	result := records.ConferenceRecordItem{
		RecordID:    record.UUID,
		Status:      record.Status,
		DurationSec: record.DurationSec,
		StartedAt:   record.StartedAt,
		StoppedAt:   record.StoppedAt,
		EndedAt:     record.EndedAt,
		CreatedAt:   record.CreatedAt,
	}
	for _, file := range details.Files {
		if file.FileType != records.FileTypeFinalMP4 && file.FileType != records.FileTypeFinalAudio && file.FileType != records.FileTypePreviewJPG {
			continue
		}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.ConferenceRecordItem{}, err
		}
		switch file.FileType {
		case records.FileTypeFinalMP4, records.FileTypeFinalAudio:
			result.FinalURL = url
		case records.FileTypePreviewJPG:
			result.PreviewURL = url
		}
	}

	return result, nil
}

// presignedURL формирует временную ссылку на приватный артефакт записи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - objectKey (string): серверный ключ объекта внутри приватного бакета.
//
// @return:
//   - результат 1 (string): адрес разрешённого чтения или целевого ресурса.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) presignedURL(ctx context.Context, objectKey string) (string, error) {
	if s.s3 == nil || objectKey == "" {
		return "", nil
	}

	return s.s3.PresignedGetURL(ctx, objectKey, 24*time.Hour)
}

// segmentMetadata преобразует сведения локального сегмента в сохраняемые метаданные.
//
// @args
//   - segments ([]ffmpeg.Segment): доступные сегменты записи для итоговой сборки.
//
// @return:
//   - результат 1 ([]records.RecordSegment): собранные элементы результата; состав ограничивается параметрами операции.
func segmentMetadata(segments []ffmpeg.Segment) []records.RecordSegment {
	result := make([]records.RecordSegment, 0, len(segments))
	for _, segment := range segments {
		fileName := segment.FileName
		sizeBytes := segment.SizeBytes
		checksum := segment.ChecksumSHA256
		result = append(result, records.RecordSegment{
			SeqNo:          segment.SeqNo,
			Status:         "closed",
			FileName:       &fileName,
			SizeBytes:      &sizeBytes,
			ChecksumSHA256: &checksum,
		})
	}

	return result
}

// cleanupLocalStorage удаляет локальные артефакты завершённой записи после принятой последовательности сохранения.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) cleanupLocalStorage(recordID string) error {
	if err := os.RemoveAll(s.recordDir(recordID)); err != nil {
		return fmt.Errorf("cleanup record storage: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(s.storagePath, "tmp", recordID)); err != nil {
		return fmt.Errorf("cleanup tmp storage: %w", err)
	}

	return nil
}

// cleanupEmptyLocalStorage удаляет пустые каталоги локального хранения после завершения обработки.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *WorkerService) cleanupEmptyLocalStorage(recordID string) error {
	return localstorage.RemoveEmptyTrees(
		s.recordDir(recordID),
		filepath.Join(s.storagePath, "tmp", recordID),
	)
}

// cleanupEmptyLocalStorageAfterFailure очищает пустые каталоги после сбоя, не удаляя оставшиеся артефакты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordID (string): внешний UUID задачи записи.
func (s *WorkerService) cleanupEmptyLocalStorageAfterFailure(ctx context.Context, recordID string) {
	if err := s.cleanupEmptyLocalStorage(recordID); err != nil {
		_ = s.repository.AddEvent(ctx, recordID, "record.storage.empty_cleanup_failed", "worker", "warning", err.Error(), s.workerID)
	}
}

// HandleOffer передаёт SDP-предложение браузера в менеджер приёма WebRTC.
// @args
// - ctx: контекст HTTP-запроса воркера.
// - recordID: UUID записи.
// - request: SDP-предложение.
// @return SDP answer или ошибку signaling.
func (s *WorkerService) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	record, err := s.repository.FindByUUID(ctx, recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}
	if records.IsComposite(record) {
		return records.WebRTCAnswerResponse{}, records.ErrRecordStateChanged
	}
	if s.ingest == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("webrtc ingest is not configured")
	}

	return s.ingest.HandleOffer(ctx, recordID, request)
}
