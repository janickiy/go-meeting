package recorder

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// Only the authenticated conference recording usecase calls this after checking
// membership and the conference/recording relation.
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
