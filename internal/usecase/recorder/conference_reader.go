package recorder

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// ReadComposite читает карточку общей записи после проверки доступа на уровне сценария конференции.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) ReadComposite(ctx context.Context, id string) (records.RecordCard, error) {
	details, err := s.repository.FindDetailsByUUID(ctx, id)
	if err != nil {
		return records.RecordCard{}, err
	}
	if !records.IsComposite(details.Record) {
		return records.RecordCard{}, apperrors.ErrNotFound
	}
	return s.recordCard(ctx, details)
}
