package recorder

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// compositePrivacyRepo хранит изолированное состояние тестового компонента «общая запись Privacy Repo».
// Состав:
//   - apiRepository: встроенный тип, добавляющий свой контракт или данные.
//   - record: задача записи с её сохранённым состоянием.
type compositePrivacyRepo struct {
	apiRepository
	record records.Record
}

// FindByUUID читает задачу записи по её внешнему UUID.
//
// @parameters:
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r compositePrivacyRepo) FindByUUID(context.Context, string) (records.Record, error) {
	return r.record, nil
}

// FindDetailsByUUID читает задачу записи и связанные сведения по её UUID.
//
// @parameters:
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
//
// @return:
//   - результат 1 (records.RecordDetails): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r compositePrivacyRepo) FindDetailsByUUID(context.Context, string) (records.RecordDetails, error) {
	return records.RecordDetails{Record: r.record}, nil
}

// ListDetails читает записи вместе со связанными артефактами и событиями.
//
// @parameters:
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 (int): предел количества обрабатываемых элементов.
//   - аргумент 3 (int): число элементов, пропускаемых перед началом страницы.
//
// @return:
//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r compositePrivacyRepo) ListDetails(context.Context, int, int) ([]records.RecordDetails, error) {
	return []records.RecordDetails{{Record: r.record}}, nil
}

// ListSummaryDetailsByConferenceIDs пакетно читает краткие сведения записей нескольких конференций без запроса для каждой записи.
//
// @parameters:
//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - аргумент 2 ([]string): идентификаторы конференций для пакетной выборки.
//   - аргумент 3 (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 ([]records.RecordDetails): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r compositePrivacyRepo) ListSummaryDetailsByConferenceIDs(context.Context, []string, string) ([]records.RecordDetails, error) {
	return []records.RecordDetails{{Record: r.record}}, nil
}

// TestCompositeIsNotExposedByAnonymousLegacyService проверяет сценарий «общая запись является не Exposed By Anonymous Legacy сервис», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestCompositeIsNotExposedByAnonymousLegacyService(t *testing.T) {
	for _, record := range []records.Record{{Mode: records.ModeComposite, UUID: "record", ConferenceID: "room"}, {SourceType: "conference", UUID: "record", ConferenceID: "room"}} {
		s := NewService(compositePrivacyRepo{record: record}, nil, nil, nil)
		if _, err := s.Read(context.Background(), "record"); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("anonymous Read = %v", err)
		}
		if err := s.Stop(context.Background(), records.EndRequest{RecordID: "record"}); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("anonymous Stop = %v", err)
		}
		items, err := s.List(context.Background(), 20, 0)
		if err != nil || len(items) != 0 {
			t.Fatalf("anonymous List = %+v, %v", items, err)
		}
		summary, err := s.CountByConference(context.Background(), []string{"room"}, "")
		if err != nil || len(summary) != 1 || summary[0].RecordsCount != 0 {
			t.Fatalf("anonymous Count = %+v, %v", summary, err)
		}
		if _, err = s.ReadComposite(context.Background(), "record"); err != nil {
			t.Fatal(err)
		}
	}
}
