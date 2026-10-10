package recorder

import (
	"context"
	"errors"
	"fmt"

	"github.com/janickiy/meet-space/internal/domain/records"
)

// ingestFailureRepository задаёт контракт зависимого компонента ingestFailureRepository в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - FindByUUID: поиск записи по UUID с контрактом, описанным у метода.
//   - MarkFailed: перевод записи в состояние ошибки с контрактом, описанным у метода.
//   - AddEvent: операция Add событие с контрактом, описанным у метода.
type ingestFailureRepository interface {
	// FindByUUID читает задачу записи по её внешнему UUID.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	FindByUUID(context.Context, string) (records.Record, error)
	// MarkFailed условно переводит задачу записи в состояние «Failed», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//   - аргумент 3 (error): значение cause типа error, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkFailed(context.Context, string, error) error
	// AddEvent добавляет постоянное диагностическое событие жизненного цикла записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//   - аргумент 3 (string): значение eventType типа string, используемое согласно назначению этой операции.
	//   - аргумент 4 (string): семантический источник медиа либо входной источник данных.
	//   - аргумент 5 (string): значение severity типа string, используемое согласно назначению этой операции.
	//   - аргумент 6 (string): сообщение чата или безопасный текст ответа согласно указанному типу.
	//   - аргумент 7 (string): идентификатор воркера-владельца операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// conferenceReleaser задаёт контракт зависимого компонента conferenceReleaser в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Release: операция освобождение с контрактом, описанным у метода.
type conferenceReleaser interface {
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

// FailIngest сохраняет ошибку уже закрытого WebRTC-приёма и освобождает только принадлежащую записи блокировку.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - repository (ingestFailureRepository): хранилище постоянных данных прикладного сценария.
//   - locker (conferenceReleaser): механизм взаимного исключения по ресурсу.
//   - recordID (string): внешний UUID задачи записи.
//   - workerID (string): идентификатор воркера-владельца операции.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func FailIngest(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID string, cause error) error {
	return failRecord(ctx, repository, locker, recordID, workerID, "record.ingest.failed", cause)
}

// failRecord сохраняет ошибку задачи записи с защитой от устаревшего состояния.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - repository (ingestFailureRepository): хранилище постоянных данных прикладного сценария.
//   - locker (conferenceReleaser): механизм взаимного исключения по ресурсу.
//   - recordID (string): внешний UUID задачи записи.
//   - workerID (string): идентификатор воркера-владельца операции.
//   - eventType (string): значение eventType типа string, используемое согласно назначению этой операции.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func failRecord(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID, eventType string, cause error) error {
	record, err := repository.FindByUUID(ctx, recordID)
	if err != nil {
		return fmt.Errorf("find failed record: %w", err)
	}
	if records.IsTerminalStatus(record.Status) {
		return releaseRecordLock(ctx, locker, record)
	}
	if err := repository.MarkFailed(ctx, recordID, cause); err != nil {
		return fmt.Errorf("mark ingest failed: %w", err)
	}
	releaseErr := releaseRecordLock(ctx, locker, record)
	message := "WebRTC ingest failed"
	if cause != nil {
		message = cause.Error()
	}
	eventErr := repository.AddEvent(ctx, recordID, eventType, "worker", "error", message, workerID)
	return errors.Join(releaseErr, eventErr)
}

// releaseRecordLock освобождает блокировку только для указанной задачи записи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - locker (conferenceReleaser): механизм взаимного исключения по ресурсу.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func releaseRecordLock(ctx context.Context, locker conferenceReleaser, record records.Record) error {
	if locker == nil {
		return nil
	}
	return locker.Release(ctx, record.ConferenceID, record.UUID)
}
