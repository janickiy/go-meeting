package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

// TestFailIngestReleasesOnlyFailedRecordLock проверяет сценарий «сбой Ingest Releases только Failed запись Lock», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestFailIngestReleasesOnlyFailedRecordLock(t *testing.T) {
	repo := &failureRepository{}
	lock := &failureLock{}
	err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks"))
	if err != nil {
		t.Fatal(err)
	}
	if !repo.marked || lock.recordID != "record-1" || lock.conferenceID != "conference-1" {
		t.Fatalf("failed record lock was not released: %+v", lock)
	}
	if repo.eventType != "record.ingest.failed" || repo.message != "no tracks" {
		t.Fatal("failure event is missing")
	}
}

// TestFailIngestKeepsLockWhenDatabaseUpdateFails проверяет сценарий «сбой Ingest Keeps Lock когда Database обновление Fails», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestFailIngestKeepsLockWhenDatabaseUpdateFails(t *testing.T) {
	repo := &failureRepository{markErr: errors.New("database unavailable")}
	lock := &failureLock{}
	if err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks")); err == nil {
		t.Fatal("expected database error")
	}
	if lock.recordID != "" {
		t.Fatal("lock released before failure was persisted")
	}
}

// TestFailIngestReleasesLockEvenWhenEventFails проверяет сценарий «сбой Ingest Releases Lock Even когда событие Fails», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestFailIngestReleasesLockEvenWhenEventFails(t *testing.T) {
	wantErr := errors.New("event unavailable")
	repo := &failureRepository{eventErr: wantErr}
	lock := &failureLock{}
	err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks"))
	if !errors.Is(err, wantErr) || lock.recordID != "record-1" {
		t.Fatalf("err = %v, released record = %s", err, lock.recordID)
	}
}

// failureRepository реализует постоянное хранение ресурсов компонента через GORM.
// @params:
//   - markErr: значение markErr типа error, используемое согласно назначению этой операции.
//   - eventErr: значение eventErr типа error, используемое согласно назначению этой операции.
//   - status: состояние ресурса, ответа или фильтра выборки.
//   - marked: логический признак marked, управляющий соответствующей веткой обработки.
//   - eventType: значение eventType типа string, используемое согласно назначению этой операции.
//   - message: сообщение чата или безопасный текст ответа согласно указанному типу.
type failureRepository struct {
	markErr, eventErr  error
	status             string
	marked             bool
	eventType, message string
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
func (r *failureRepository) FindByUUID(context.Context, string) (records.Record, error) {
	return records.Record{UUID: "record-1", ConferenceID: "conference-1", Status: r.status}, nil
}

// TestLateIngestFailureDoesNotOverwriteReadyRecord проверяет сценарий «Late Ingest сбой выполняет не Overwrite готовность запись», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestLateIngestFailureDoesNotOverwriteReadyRecord(t *testing.T) {
	repo := &failureRepository{status: records.StatusReady}
	lock := &failureLock{}
	if err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("late ICE error")); err != nil {
		t.Fatal(err)
	}
	if repo.marked || repo.eventType != "" {
		t.Fatal("late ingest failure changed a ready record")
	}
}

// MarkFailed условно переводит задачу записи в состояние «Failed», соблюдая ограничения жизненного цикла.
//
// @args
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//   - аргумент 3 (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *failureRepository) MarkFailed(context.Context, string, error) error {
	r.marked = r.markErr == nil
	return r.markErr
}

// AddEvent добавляет постоянное диагностическое событие жизненного цикла записи.
//
// @args
//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - eventType (string): значение eventType типа string, используемое согласно назначению этой операции.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//   - message (string): сообщение чата или безопасный текст ответа согласно указанному типу.
//   - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *failureRepository) AddEvent(_ context.Context, _, eventType, _, _, message, _ string) error {
	r.eventType, r.message = eventType, message
	return r.eventErr
}

// failureLock хранит изолированное состояние тестового компонента «сбой Lock».
// @params:
//   - recordID: внешний UUID задачи записи.
//   - conferenceID: идентификатор конференции, ограничивающий область операции.
type failureLock struct{ recordID, conferenceID string }

// Release освобождает ресурс только при совпадении сохранённого владельца или токена.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (l *failureLock) Release(_ context.Context, conferenceID, recordID string) error {
	l.conferenceID, l.recordID = conferenceID, recordID
	return nil
}
