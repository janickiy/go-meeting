package usecase_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

const workerTestRecordID = "22222222-2222-4222-8222-222222222222"

// TestWorkerIgnoresCommandsForTerminalRecords проверяет игнорирование команд для записей с окончательным состоянием.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestWorkerIgnoresCommandsForTerminalRecords(t *testing.T) {
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		for _, commandType := range []string{"record.start", "record.stop"} {
			t.Run(status+"/"+commandType, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

				@args
				  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
				*/func(t *testing.T) {
					repo := newWorkerRepository(status)
					ingest := &fakeIngest{}
					storage := t.TempDir()
					locker := &failureLock{}
					service := recorder.NewWorkerService(repo, nil, ingest, nil, storage, "worker", locker)
					if err := service.HandleCommand(context.Background(), records.Command{Type: commandType, RecordID: workerTestRecordID}); err != nil {
						t.Fatal(err)
					}
					if ingest.prepared || ingest.stopped || repo.marked || repo.finalizing || repo.eventType != "" {
						t.Fatal("terminal record was processed again")
					}
					entries, err := os.ReadDir(storage)
					if err != nil || len(entries) != 0 {
						t.Fatalf("duplicate command touched storage: %v", err)
					}
					if locker.recordID != workerTestRecordID {
						t.Fatal("retry did not release residual owner lock")
					}
				})
		}
	}
}

// TestWorkerReleasesLockAfterMediaStopsBeforeFinalization проверяет освобождение блокировки после остановки медиа до финализации.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestWorkerReleasesLockAfterMediaStopsBeforeFinalization(t *testing.T) {
	repo := newWorkerRepository(records.StatusStopping)
	ingest := &fakeIngest{}
	lock := &orderedRelease{t: t, ingest: ingest}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	repo.onFinalizing = func() {
		if !lock.released {
			t.Error("finalization started before the closed media lock was released")
		}
	}
	// Пустой каталог вызывает ошибку Finalize без запуска FFmpeg.
	service := recorder.NewWorkerService(repo, ffmpeg.NewPostProcessor("not-used"), ingest, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.stop", RecordID: workerTestRecordID}); err != nil {
		t.Fatal(err)
	}
	if !lock.released || !repo.marked || repo.eventType != "record.finalize.failed" {
		t.Fatal("failed finalization did not persist failure and release lock")
	}
}

// TestWorkerPrepareFailureReleasesLock проверяет освобождение блокировки при сбое подготовки воркера.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestWorkerPrepareFailureReleasesLock(t *testing.T) {
	repo := newWorkerRepository(records.StatusStarting)
	ingest := &fakeIngest{prepareErr: errors.New("prepare failed")}
	lock := &failureLock{}
	service := recorder.NewWorkerService(repo, nil, ingest, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: workerTestRecordID}); err != nil {
		t.Fatal(err)
	}
	if !repo.marked || lock.recordID != workerTestRecordID || repo.eventType != "record.worker.prepare.failed" {
		t.Fatal("prepare failure left an active lock or missed failure event")
	}
}

// TestWorkerDoesNotAcknowledgeDatabaseFailure проверяет отсутствие подтверждения команды при сбое БД.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestWorkerDoesNotAcknowledgeDatabaseFailure(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repo := newWorkerRepository(records.StatusStarting)
	repo.markErr = wantErr
	lock := &failureLock{}
	service := recorder.NewWorkerService(repo, nil, &fakeIngest{prepareErr: errors.New("prepare failed")}, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: workerTestRecordID}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want database failure", err)
	}
	if lock.recordID != "" {
		t.Fatal("failure persistence did not succeed but lock was released")
	}
}

// TestWorkerRejectsUnsafeIDBeforeTouchingStorage проверяет отклонение небезопасного ID до обращения к хранилищу.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestWorkerRejectsUnsafeIDBeforeTouchingStorage(t *testing.T) {
	storage := t.TempDir()
	service := recorder.NewWorkerService(nil, nil, nil, nil, storage, "worker", nil)
	for _, recordID := range []string{"", "../escape", filepath.Join(storage, "escape"), "not-a-uuid"} {
		if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: recordID}); err == nil {
			t.Fatalf("accepted unsafe ID %q", recordID)
		}
	}
	entries, err := os.ReadDir(storage)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid command created files")
	}
}

// fakeWorkerRepository реализует постоянное хранение ресурсов компонента через GORM.
// @params:
//   - failureRepository: встроенный тип, добавляющий свой контракт или данные.
//   - record: задача записи с её сохранённым состоянием.
//   - finalizing: логический признак finalizing, управляющий соответствующей веткой обработки.
//   - onFinalizing: операция onFinalizing с контрактом, описанным у метода.
type fakeWorkerRepository struct {
	failureRepository
	record       records.Record
	finalizing   bool
	onFinalizing func()
}

// TestConcurrentDuplicateStopsRunMediaStopOnce проверяет сценарий «одновременный повторный Stops выполнение медиа остановка Once», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestConcurrentDuplicateStopsRunMediaStopOnce(t *testing.T) {
	repo := newWorkerRepository(records.StatusStopping)
	ingest := &fakeIngest{}
	service := recorder.NewWorkerService(repo, ffmpeg.NewPostProcessor("not-used"), ingest, nil, t.TempDir(), "worker", nil)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			if err := service.HandleCommand(context.Background(), records.Command{Type: "record.stop", RecordID: workerTestRecordID}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ingest.stopCalls != 1 || repo.record.Status != records.StatusFailed {
		t.Fatalf("stop calls = %d, status = %s", ingest.stopCalls, repo.record.Status)
	}
}

// newWorkerRepository подготавливает или проверяет часть тестового сценария «новый воркер Repository».
//
// @args
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 (*fakeWorkerRepository): значение, подготовленное операцией для вызывающей стороны.
func newWorkerRepository(status string) *fakeWorkerRepository {
	return &fakeWorkerRepository{record: records.Record{UUID: workerTestRecordID, ConferenceID: "conference-1", Status: status}}
}

// FindByUUID читает задачу записи по её внешнему UUID.
//
// @args
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeWorkerRepository) FindByUUID(context.Context, string) (records.Record, error) {
	return r.record, nil
}

// MarkFailed условно переводит задачу записи в состояние «Failed», соблюдая ограничения жизненного цикла.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeWorkerRepository) MarkFailed(ctx context.Context, id string, cause error) error {
	if err := r.failureRepository.MarkFailed(ctx, id, cause); err != nil {
		return err
	}
	r.record.Status = records.StatusFailed
	return nil
}

// MarkFinalizing условно переводит задачу записи в состояние «Finalizing», соблюдая ограничения жизненного цикла.
//
// @args
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeWorkerRepository) MarkFinalizing(context.Context, string) error {
	r.finalizing = true
	if r.onFinalizing != nil {
		r.onFinalizing()
	}
	return nil
}

// MarkUploading условно переводит задачу записи в состояние «Uploading», соблюдая ограничения жизненного цикла.
//
// @args
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *fakeWorkerRepository) MarkUploading(context.Context, string) error { return nil }

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
func (r *fakeWorkerRepository) SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error {
	return nil
}

// fakeIngest хранит изолированное состояние подставного компонента приёма медиа.
// @params:
//   - prepared: логический признак prepared, управляющий соответствующей веткой обработки.
//   - stopped: логический признак stopped, управляющий соответствующей веткой обработки.
//   - prepareErr: значение prepareErr типа error, используемое согласно назначению этой операции.
//   - stopCalls: значение stopCalls типа int, используемое согласно назначению этой операции.
type fakeIngest struct {
	prepared, stopped bool
	prepareErr        error
	stopCalls         int
}

// Prepare подготавливает состояние WebRTC-приёма конкретной записи до обмена SDP.
//
// @args
//   - аргумент 1 (string): внешний UUID задачи записи.
//   - аргумент 2 (int): плановая длительность сегмента записи в секундах.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (i *fakeIngest) Prepare(string, int) error { i.prepared = true; return i.prepareErr }

// Stop останавливает активную обработку ресурсов компонента и освобождает связанные ресурсы.
//
// @args
//   - аргумент 1 (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (i *fakeIngest) Stop(string) error { i.stopped = true; i.stopCalls++; return nil }

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
func (i *fakeIngest) HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	return records.WebRTCAnswerResponse{}, nil
}

// orderedRelease хранит изолированное состояние тестового компонента «ordered освобождение».
// @params:
//   - t: контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - ingest: значение ingest типа *fakeIngest, используемое согласно назначению этой операции.
//   - released: логический признак released, управляющий соответствующей веткой обработки.
type orderedRelease struct {
	t        *testing.T
	ingest   *fakeIngest
	released bool
}

// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
//
// @args
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
//   - аргумент 3 (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (l *orderedRelease) Release(context.Context, string, string) error {
	if !l.ingest.stopped {
		l.t.Error("lock released while media is still running")
	}
	l.released = true
	return nil
}
