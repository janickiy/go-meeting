package recorder

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// ReadComposite читает карточку общей записи после проверки доступа на уровне сценария конференции.
//
// @args
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

// ReadComposites builds an already-authorized page while retaining the single
// record availability and mode checks. Older repository adapters keep working.
func (s *Service) ReadComposites(ctx context.Context, ids []string) ([]records.RecordCard, error) {
	batch, ok := s.repository.(interface {
		FindDetailsByUUIDs(context.Context, []string) ([]records.RecordDetails, error)
	})
	if !ok {
		cards := make([]records.RecordCard, 0, len(ids))
		for _, id := range ids {
			card, err := s.ReadComposite(ctx, id)
			if err != nil {
				return nil, err
			}
			cards = append(cards, card)
		}
		return cards, nil
	}
	rows, err := batch.FindDetailsByUUIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	cards := make([]records.RecordCard, 0, len(rows))
	for _, row := range rows {
		if !records.IsComposite(row.Record) {
			return nil, apperrors.ErrNotFound
		}
		card, err := s.recordCard(ctx, row)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}
