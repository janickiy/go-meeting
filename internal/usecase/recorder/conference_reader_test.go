package recorder

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

type compositePrivacyRepo struct {
	apiRepository
	record records.Record
}

func (r compositePrivacyRepo) FindByUUID(context.Context, string) (records.Record, error) {
	return r.record, nil
}
func (r compositePrivacyRepo) FindDetailsByUUID(context.Context, string) (records.RecordDetails, error) {
	return records.RecordDetails{Record: r.record}, nil
}
func (r compositePrivacyRepo) ListDetails(context.Context, int, int) ([]records.RecordDetails, error) {
	return []records.RecordDetails{{Record: r.record}}, nil
}
func (r compositePrivacyRepo) ListSummaryDetailsByConferenceIDs(context.Context, []string, string) ([]records.RecordDetails, error) {
	return []records.RecordDetails{{Record: r.record}}, nil
}

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
