package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

// TestStartReturnsConflictWhenConferenceLockExists проверяет сценарий «запуск Returns Conflict когда конференция Lock Exists», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStartReturnsConflictWhenConferenceLockExists(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: false}
	service := recorder.NewService(repository, worker, nil, locker)

	_, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})

	if !errors.Is(err, records.ErrConferenceAlreadyRecording) {
		t.Fatalf("err = %v, want ErrConferenceAlreadyRecording", err)
	}
	if repository.createCalled {
		t.Fatal("record was created when lock was not acquired")
	}
	if worker.startCalled {
		t.Fatal("worker start was called when lock was not acquired")
	}
}

// TestStartKeepsConferenceLockAfterWorkerPrepare проверяет сценарий «запуск Keeps конференция Lock после воркер Prepare», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStartKeepsConferenceLockAfterWorkerPrepare(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	response, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if response.RecordID == "" {
		t.Fatal("RecordID is empty")
	}
	if !locker.acquireCalled {
		t.Fatal("lock was not acquired")
	}
	if locker.releaseCalled {
		t.Fatal("lock was released after successful start")
	}
	if worker.startRecordID != response.RecordID {
		t.Fatalf("worker start recordId = %q, want %q", worker.startRecordID, response.RecordID)
	}
	if response.WebRTC.Video.Quality != records.DefaultVideoQuality {
		t.Fatalf("response video quality = %q, want %q", response.WebRTC.Video.Quality, records.DefaultVideoQuality)
	}
	if response.WebRTC.Video.Width != records.DefaultVideoWidth || response.WebRTC.Video.Height != records.DefaultVideoHeight {
		t.Fatalf("response video size = %dx%d, want %dx%d", response.WebRTC.Video.Width, response.WebRTC.Video.Height, records.DefaultVideoWidth, records.DefaultVideoHeight)
	}
	if response.WebRTC.Video.MaxBitrateBPS != records.DefaultVideoMaxBitrateBPS {
		t.Fatalf("response max bitrate = %d, want %d", response.WebRTC.Video.MaxBitrateBPS, records.DefaultVideoMaxBitrateBPS)
	}
}

// TestStartReleasesConferenceLockWhenWorkerPrepareFails проверяет сценарий «запуск Releases конференция Lock когда воркер Prepare Fails», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStartReleasesConferenceLockWhenWorkerPrepareFails(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{startErr: errors.New("worker down")}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	_, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})

	if err == nil {
		t.Fatal("Start() error is nil")
	}
	if !locker.releaseCalled {
		t.Fatal("lock was not released after worker prepare error")
	}
	if !repository.markFailedCalled {
		t.Fatal("record was not marked failed after worker prepare error")
	}
}

// TestStopKeepsConferenceLockUntilWorkerStopsMedia проверяет сценарий «остановка Keeps конференция Lock Until воркер Stops медиа», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStopKeepsConferenceLockUntilWorkerStopsMedia(t *testing.T) {
	repository := &fakeRepository{
		record: records.Record{
			UUID:         "22222222-2222-4222-8222-222222222222",
			ConferenceID: "11111111-1111-4111-8111-111111111111",
			Status:       records.StatusRecording,
		},
	}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	err := service.Stop(context.Background(), records.EndRequest{
		RecordID: "22222222-2222-4222-8222-222222222222",
		Reason:   "client_stop",
	})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if !repository.markStoppingCalled {
		t.Fatal("record was not marked stopping")
	}
	if !worker.stopCalled {
		t.Fatal("worker stop was not called")
	}
	if locker.releaseCalled {
		t.Fatal("lock was released before the worker stopped media")
	}
}

// TestStopDoesNotReopenCompletedOrFinalizingRecord проверяет сценарий «остановка выполняет не Reopen Completed Or Finalizing запись», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStopDoesNotReopenCompletedOrFinalizingRecord(t *testing.T) {
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled, records.StatusFinalizing, records.StatusUploading} {
		t.Run(status, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@args
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: status}}
				worker := &fakeWorkerCommander{}
				service := recorder.NewService(repository, worker, nil, nil)
				if err := service.Stop(context.Background(), records.EndRequest{RecordID: "record-1", Reason: "retry"}); err != nil {
					t.Fatal(err)
				}
				if repository.markStoppingCalled || worker.stopCalled {
					t.Fatal("stop reopened a completed or finalizing record")
				}
			})
	}
}

// TestStopRetriesPublishWithoutChangingStopMetadata проверяет сценарий «остановка Retries публикация без Changing остановка Metadata», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStopRetriesPublishWithoutChangingStopMetadata(t *testing.T) {
	repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: records.StatusRecording}}
	wantErr := errors.New("broker unavailable")
	worker := &fakeWorkerCommander{stopErr: wantErr}
	locker := &fakeConferenceLocker{}
	service := recorder.NewService(repository, worker, nil, locker)
	request := records.EndRequest{RecordID: "record-1", Reason: "client_stop"}
	if err := service.Stop(context.Background(), request); !errors.Is(err, wantErr) {
		t.Fatalf("Stop() = %v, want publish error", err)
	}
	if locker.releaseCalled {
		t.Fatal("failed publication released lock")
	}
	repository.markStoppingCalled = false
	worker.stopCalled, worker.stopErr = false, nil
	if err := service.Stop(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if repository.markStoppingCalled || !worker.stopCalled || locker.releaseCalled {
		t.Fatal("retry must only republish the stop command")
	}
}

// TestStopIgnoresStaleState проверяет сценарий «остановка Ignores устаревший состояние», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStopIgnoresStaleState(t *testing.T) {
	repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: records.StatusRecording}, markStoppingErr: records.ErrRecordStateChanged}
	worker := &fakeWorkerCommander{}
	service := recorder.NewService(repository, worker, nil, nil)
	if err := service.Stop(context.Background(), records.EndRequest{RecordID: "record-1"}); err != nil {
		t.Fatal(err)
	}
	if worker.stopCalled {
		t.Fatal("stale request published a stop command")
	}
}

// TestCountByConferenceReturnsRecordsWithTimeFields проверяет сценарий «количество By конференция Returns Records с время Fields», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCountByConferenceReturnsRecordsWithTimeFields(t *testing.T) {
	duration := 37
	startedAt := time.Date(2026, 5, 29, 9, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 29, 9, 29, 0, 0, time.UTC)
	repository := &fakeRepository{
		listDetailsByConferenceResponse: []records.RecordDetails{
			{
				Record: records.Record{
					UUID:         "22222222-2222-4222-8222-222222222222",
					ConferenceID: "11111111-1111-4111-8111-111111111111",
					Status:       records.StatusReady,
					DurationSec:  &duration,
					StartedAt:    &startedAt,
					CreatedAt:    createdAt,
				},
				Files: []records.RecordFile{
					{FileType: records.FileTypeFinalMP4, ObjectKey: "records/222/final.mp4"},
					{FileType: records.FileTypePreviewJPG, ObjectKey: "records/222/preview.jpg"},
				},
			},
		},
	}
	service := recorder.NewService(repository, &fakeWorkerCommander{}, nil, nil)

	result, err := service.CountByConference(context.Background(), []string{"11111111-1111-4111-8111-111111111111"}, records.StatusReady)
	if err != nil {
		t.Fatalf("CountByConference() error = %v", err)
	}

	if !repository.listDetailsByConferenceCalled {
		t.Fatal("repository ListSummaryDetailsByConferenceIDs() was not called")
	}
	if repository.listDetailsByConferenceStatus != records.StatusReady {
		t.Fatalf("status = %q, want %q", repository.listDetailsByConferenceStatus, records.StatusReady)
	}
	if got, want := len(result), 1; got != want {
		t.Fatalf("result len = %d, want %d", got, want)
	}
	if result[0].RecordsCount != 1 {
		t.Fatalf("recordsCount = %d, want 1", result[0].RecordsCount)
	}
	if got, want := len(result[0].Records), 1; got != want {
		t.Fatalf("records len = %d, want %d", got, want)
	}
	if result[0].Records[0].RecordID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("recordId = %q", result[0].Records[0].RecordID)
	}
	if result[0].Records[0].DurationSec == nil || *result[0].Records[0].DurationSec != duration {
		t.Fatalf("durationSec = %v, want %d", result[0].Records[0].DurationSec, duration)
	}
}

// fakeRepository реализует постоянное хранение ресурсов компонента через GORM.
// @params:
//   - createCalled: логический признак createCalled, управляющий соответствующей веткой обработки.
//   - markFailedCalled: логический признак markFailedCalled, управляющий соответствующей веткой обработки.
//   - markStoppingCalled: логический признак markStoppingCalled, управляющий соответствующей веткой обработки.
//   - markStoppingErr: значение markStoppingErr типа error, используемое согласно назначению этой операции.
//   - listDetailsByConferenceCalled: логический признак listDetailsByConferenceCalled, управляющий соответствующей веткой обработки.
//   - listDetailsByConferenceIDs: идентификаторы связанных ресурсов для пакетной операции.
//   - listDetailsByConferenceStatus: значение listDetailsByConferenceStatus типа string, используемое согласно назначению этой операции.
//   - listDetailsByConferenceResponse: набор значений listDetailsByConferenceResponse для последовательной или пакетной обработки.
//   - listDetailsByConferenceErr: значение listDetailsByConferenceErr типа error, используемое согласно назначению этой операции.
//   - record: задача записи с её сохранённым состоянием.
type fakeRepository struct {
	createCalled                    bool
	markFailedCalled                bool
	markStoppingCalled              bool
	markStoppingErr                 error
	listDetailsByConferenceCalled   bool
	listDetailsByConferenceIDs      []string
	listDetailsByConferenceStatus   string
	listDetailsByConferenceResponse []records.RecordDetails
	listDetailsByConferenceErr      error
	record                          records.Record
}

// Create создаёт новое состояние ресурсов компонента по переданным параметрам.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) Create(_ context.Context, record records.Record) (records.Record, error) {
	r.createCalled = true
	if record.UUID == "" {
		record.UUID = "22222222-2222-4222-8222-222222222222"
	}
	r.record = record

	return record, nil
}

// FindByUUID читает задачу записи по её внешнему UUID.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) FindByUUID(_ context.Context, _ string) (records.Record, error) {
	if r.record.UUID == "" {
		return records.Record{}, errors.New("not found")
	}

	return r.record, nil
}

// MarkStopping условно переводит задачу записи в состояние «Stopping», соблюдая ограничения жизненного цикла.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) MarkStopping(_ context.Context, _ string, _ string) error {
	r.markStoppingCalled = true
	if r.markStoppingErr == nil {
		r.record.Status = records.StatusStopping
	}
	return r.markStoppingErr
}

// MarkFailed условно переводит задачу записи в состояние «Failed», соблюдая ограничения жизненного цикла.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (error): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) MarkFailed(_ context.Context, _ string, _ error) error {
	r.markFailedCalled = true

	return nil
}

// ListDetails читает записи вместе со связанными артефактами и событиями.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (int): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (int): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) ListDetails(_ context.Context, _ int, _ int) ([]records.RecordDetails, error) {
	return nil, nil
}

// ListSummaryDetailsByConferenceIDs пакетно читает краткие сведения записей нескольких конференций без запроса для каждой записи.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceIDs ([]string): идентификаторы конференций для пакетной выборки.
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) ListSummaryDetailsByConferenceIDs(_ context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error) {
	r.listDetailsByConferenceCalled = true
	r.listDetailsByConferenceIDs = conferenceIDs
	r.listDetailsByConferenceStatus = status

	return r.listDetailsByConferenceResponse, r.listDetailsByConferenceErr
}

// FindDetailsByUUID читает задачу записи и связанные сведения по её UUID.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (records.RecordDetails): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeRepository) FindDetailsByUUID(_ context.Context, _ string) (records.RecordDetails, error) {
	return records.RecordDetails{}, nil
}

// fakeWorkerCommander хранит изолированное состояние тестового компонента «fake воркер Commander».
// @params:
//   - startCalled: логический признак startCalled, управляющий соответствующей веткой обработки.
//   - startRecordID: идентификатор связанного ресурса, заданного параметром startRecordID.
//   - startDurationSec: значение startDurationSec типа int, используемое согласно назначению этой операции.
//   - startErr: значение startErr типа error, используемое согласно назначению этой операции.
//   - stopCalled: логический признак stopCalled, управляющий соответствующей веткой обработки.
//   - stopRecordID: идентификатор связанного ресурса, заданного параметром stopRecordID.
//   - stopReason: значение stopReason типа string, используемое согласно назначению этой операции.
//   - stopErr: значение stopErr типа error, используемое согласно назначению этой операции.
type fakeWorkerCommander struct {
	startCalled      bool
	startRecordID    string
	startDurationSec int
	startErr         error
	stopCalled       bool
	stopRecordID     string
	stopReason       string
	stopErr          error
}

// StartRecord передаёт команду начала записи выбранному транспорту воркера.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordID (string): внешний UUID задачи записи.
//   - segmentDurationSec (int): плановая длительность сегмента записи в секундах.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *fakeWorkerCommander) StartRecord(_ context.Context, recordID string, segmentDurationSec int) error {
	c.startCalled = true
	c.startRecordID = recordID
	c.startDurationSec = segmentDurationSec

	return c.startErr
}

// StopRecord передаёт команду остановки записи выбранному транспорту воркера.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - recordID (string): внешний UUID задачи записи.
//   - reason (string): причина завершения, отказа или изменения состояния.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (c *fakeWorkerCommander) StopRecord(_ context.Context, recordID string, reason string) error {
	c.stopCalled = true
	c.stopRecordID = recordID
	c.stopReason = reason

	return c.stopErr
}

// fakeConferenceLocker хранит изолированное состояние тестового компонента «fake конференция Locker».
// @params:
//   - acquireOK: логический признак acquireOK, управляющий соответствующей веткой обработки.
//   - acquireCalled: логический признак acquireCalled, управляющий соответствующей веткой обработки.
//   - releaseCalled: логический признак releaseCalled, управляющий соответствующей веткой обработки.
type fakeConferenceLocker struct {
	acquireOK     bool
	acquireCalled bool
	releaseCalled bool
}

// Acquire пытается занять блокировку ресурса на ограниченный срок без замены действующего владельца.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (l *fakeConferenceLocker) Acquire(_ context.Context, _ string, _ string) (bool, error) {
	l.acquireCalled = true

	return l.acquireOK, nil
}

// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (l *fakeConferenceLocker) Release(_ context.Context, _ string, _ string) error {
	l.releaseCalled = true

	return nil
}
