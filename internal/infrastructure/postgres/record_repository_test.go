package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testRepository подготавливает или проверяет часть тестового сценария «проверка Repository».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*RecordRepository): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (*gorm.DB): значение, подготовленное операцией для вызывающей стороны.
func testRepository(t *testing.T) (*RecordRepository, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("RECORDER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RECORDER_TEST_POSTGRES_DSN to a local PostgreSQL URL")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("repository tests require a local PostgreSQL URL")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = tx.Rollback().Error })
	return NewRecordRepository(tx), tx
}

// createTestRecord подготавливает или проверяет часть тестового сценария «создание проверка запись».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - repo (*RecordRepository): хранилище постоянных данных прикладного сценария.
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
func createTestRecord(t *testing.T, repo *RecordRepository, status string) records.Record {
	t.Helper()
	record, err := repo.Create(context.Background(), records.Record{
		UUID: uuid.NewString(), ConferenceID: uuid.NewString(), Status: status,
		SourceType: "browser", TransportType: "webrtc", QualityMode: "auto", SegmentDurationSec: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// TestTerminalRecordCannotBeOverwritten проверяет сценарий «Terminal запись Cannot Be Overwritten», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestTerminalRecordCannotBeOverwritten(t *testing.T) {
	repo, _ := testRepository(t)
	ctx := context.Background()
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		t.Run(status, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@parameters:
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				record := createTestRecord(t, repo, status)
				for name, transition := range map[string]func() error{
					"recording":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.MarkRecording(ctx, record.UUID, "late-worker") },
					"stopping":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.MarkStopping(ctx, record.UUID, "duplicate") },
					"finalizing":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.MarkFinalizing(ctx, record.UUID) },
					"uploading":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.MarkUploading(ctx, record.UUID) },
					"failed":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.MarkFailed(ctx, record.UUID, errors.New("late error")) },
					"ready":/* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


					@return:
					  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error { return repo.SaveFinalArtifacts(ctx, record.UUID, records.RecordFile{}, nil, nil) },
				} {
					if err := transition(); !errors.Is(err, records.ErrRecordStateChanged) {
						t.Fatalf("%s returned %v", name, err)
					}
				}
				got, err := repo.FindByUUID(ctx, record.UUID)
				if err != nil {
					t.Fatal(err)
				}
				if got.Status != status || !got.UpdatedAt.Equal(record.UpdatedAt) || got.EndedAt != nil {
					t.Fatalf("terminal record changed: %+v", got)
				}
			})
	}
}

// TestStopMetadataSurvivesLateStartAndDuplicateStop проверяет сценарий «остановка Metadata Survives Late запуск и повторный остановка», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStopMetadataSurvivesLateStartAndDuplicateStop(t *testing.T) {
	repo, _ := testRepository(t)
	ctx := context.Background()
	record := createTestRecord(t, repo, records.StatusStarting)
	if err := repo.MarkStopping(ctx, record.UUID, "first reason"); err != nil {
		t.Fatal(err)
	}
	stopping, err := repo.FindByUUID(ctx, record.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRecording(ctx, record.UUID, "late-worker"); !errors.Is(err, records.ErrRecordStateChanged) {
		t.Fatal(err)
	}
	if err := repo.MarkStopping(ctx, record.UUID, "second reason"); !errors.Is(err, records.ErrRecordStateChanged) {
		t.Fatal(err)
	}
	got, err := repo.FindByUUID(ctx, record.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != records.StatusStopping || got.StartedAt != nil || got.EndedReason == nil || *got.EndedReason != "first reason" || !got.StoppedAt.Equal(*stopping.StoppedAt) {
		t.Fatalf("stop metadata was overwritten: %+v", got)
	}
	if err := repo.MarkFinalizing(ctx, record.UUID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkUploading(ctx, record.UUID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveFinalArtifacts(ctx, record.UUID, records.RecordFile{FileType: records.FileTypeFinalMP4, Bucket: "test", ObjectKey: record.UUID + "/final.mp4", MimeType: "video/mp4", IsPrimary: true}, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByUUID(ctx, record.UUID)
	if err != nil || got.Status != records.StatusReady {
		t.Fatalf("record did not become ready: %v", err)
	}
}

// TestSummaryLoadsOnlyFinalFilesWithTwoQueries проверяет сценарий «Summary Loads только итоговый файлы с два Queries», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestSummaryLoadsOnlyFinalFilesWithTwoQueries(t *testing.T) {
	repo, tx := testRepository(t)
	ctx := context.Background()
	record := createTestRecord(t, repo, records.StatusReady)
	for _, fileType := range []string{records.FileTypeFinalMP4, records.FileTypePreviewJPG, records.FileTypeDebugLog} {
		file := records.RecordFile{RecordID: record.ID, FileType: fileType, Bucket: "test", ObjectKey: record.UUID + "/" + fileType, MimeType: "application/octet-stream"}
		if err := tx.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Create(&records.RecordSegment{RecordID: record.ID, SeqNo: 1, Status: "closed"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.AddEvent(ctx, record.UUID, "test", "worker", "info", "test event", "worker"); err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	countedRepo := NewRecordRepository(tx.Session(&gorm.Session{Logger: counter}))
	details, err := countedRepo.ListSummaryDetailsByConferenceIDs(ctx, []string{record.ConferenceID}, records.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.queries.Load(); got != 2 {
		t.Fatalf("query count = %d, want 2", got)
	}
	if len(details) != 1 || len(details[0].Files) != 2 || len(details[0].Segments) != 0 || len(details[0].Events) != 0 {
		t.Fatalf("summary loaded unnecessary relations: %+v", details)
	}
	for _, file := range details[0].Files {
		if file.FileType == records.FileTypeDebugLog {
			t.Fatal("summary fetched debug logs")
		}
	}
	full, err := repo.FindDetailsByUUID(ctx, record.UUID)
	if err != nil || len(full.Files) != 3 || len(full.Segments) != 1 || len(full.Events) != 1 {
		t.Fatalf("full detail endpoint lost relations: %+v, %v", full, err)
	}
	filtered, err := countedRepo.ListSummaryDetailsByConferenceIDs(ctx, []string{record.ConferenceID}, records.StatusFailed)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("status filter failed: %v", err)
	}
}

// queryCounter хранит изолированное состояние тестового компонента «query Counter».
// Состав:
//   - logger.Interface: встроенный тип, добавляющий свой контракт или данные.
//   - queries: значение queries типа atomic.Int32, используемое согласно назначению этой операции.
type queryCounter struct {
	logger.Interface
	queries atomic.Int32
}

// Trace подготавливает или проверяет часть тестового сценария «Trace».
//
// @parameters:
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (time.Time): значение для проверки, нормализации или преобразования.
//   - аргумент 3 (func() (string, int64)): значение для проверки, нормализации или преобразования.
//   - аргумент 4 (error): значение для проверки, нормализации или преобразования.
func (l *queryCounter) Trace(context.Context, time.Time, func() (string, int64), error) {
	l.queries.Add(1)
}
