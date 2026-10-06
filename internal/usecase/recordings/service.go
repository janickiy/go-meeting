package recordings

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// Repository is the atomic conference recording API boundary. Delivery of its
// durable commands belongs to CommandRepository, not the request service.
type Repository interface {
	Start(context.Context, string, string, int) (records.Record, bool, error)
	Stop(context.Context, string, string, string) (records.Record, error)
	Accessible(context.Context, string, string, string) (records.Record, error)
	List(context.Context, string, string, int, int) ([]records.Record, error)
}

// Reader loads recording cards after the repository has checked access.
type Reader interface {
	// ReadComposite loads one recording and its artifacts.
	ReadComposite(context.Context, string) (records.RecordCard, error)
}

// Events publishes recording changes to conference participants.
type Events interface {
	// Broadcast publishes a trusted conference event.
	Broadcast(context.Context, realtime.Envelope) error
}

// Service handles authorized recording requests and reads. It owns no polling
// goroutine, queue connection or distributed lock.
type Service struct {
	repo   Repository
	reader Reader
	events Events
}

func NewConferenceService(repo Repository, reader Reader, events Events) *Service {
	return &Service{repo: repo, reader: reader, events: events}
}

// Start commits a recording request; the dispatcher delivers its durable command.
func (s *Service) Start(ctx context.Context, userID, conferenceID string, request records.ConferenceStartRequest) (records.RecordCard, error) {
	if request.Mode == "" {
		request.Mode = records.ModeComposite
	}
	if !records.ValidConferenceMode(request.Mode) {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	segmentDurationSec := request.SegmentDurationSec
	if segmentDurationSec == 0 {
		segmentDurationSec = 5
	}
	if segmentDurationSec < 2 || segmentDurationSec > 30 {
		return records.RecordCard{}, apperrors.New(apperrors.ErrInvalidInput, "segmentDurationSec must be between 2 and 30")
	}
	var record records.Record
	var created bool
	var err error
	if modes, ok := s.repo.(interface {
		StartMode(context.Context, string, string, int, string) (records.Record, bool, error)
	}); ok {
		record, created, err = modes.StartMode(ctx, userID, conferenceID, segmentDurationSec, request.Mode)
	} else if request.Mode == records.ModeComposite {
		record, created, err = s.repo.Start(ctx, userID, conferenceID, segmentDurationSec)
	} else {
		return records.RecordCard{}, apperrors.ErrInvalidInput
	}
	if err != nil {
		return records.RecordCard{}, err
	}
	if created {
		publish(s.events, ctx, record, "recording.starting")
	}
	return s.card(ctx, record.UUID)
}

// Stop requests a stop; the capture worker releases the recording resources.
func (s *Service) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Stop(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	if record.Status == records.StatusStopping {
		publish(s.events, ctx, record, "recording.stopping")
	}
	return s.card(ctx, record.UUID)
}

// Read checks access before loading the recording card.
func (s *Service) Read(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Accessible(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	return s.card(ctx, record.UUID)
}

// List checks access and uses batched reads when the reader supports them.
func (s *Service) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.RecordCard, error) {
	rows, err := s.repo.List(ctx, userID, conferenceID, limit, offset)
	if err != nil {
		return nil, err
	}
	if batch, ok := s.reader.(interface {
		ReadComposites(context.Context, []string) ([]records.RecordCard, error)
	}); ok {
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UUID)
		}
		items, err := batch.ReadComposites(ctx, ids)
		if err != nil {
			return nil, err
		}
		for i := range items {
			items[i].Status = records.PublicStatus(items[i].Status)
		}
		return items, nil
	}
	items := make([]records.RecordCard, 0, len(rows))
	for _, record := range rows {
		card, err := s.card(ctx, record.UUID)
		if err != nil {
			return nil, err
		}
		items = append(items, card)
	}
	return items, nil
}

// card maps internal finalization states to the public recording status.
func (s *Service) card(ctx context.Context, recordID string) (records.RecordCard, error) {
	card, err := s.reader.ReadComposite(ctx, recordID)
	card.Status = records.PublicStatus(card.Status)
	return card, err
}

// publish sends a best-effort notification; delivery failure does not undo the request.
func publish(events Events, ctx context.Context, record records.Record, eventType string) {
	if events != nil {
		_ = events.Broadcast(ctx, realtime.Event(eventType, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": record.Mode, "requestedBy": record.RequestedBy}))
	}
}
