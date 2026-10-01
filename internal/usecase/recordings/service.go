package recordings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

type Repository interface {
	Start(context.Context, string, string, int) (records.Record, bool, error)
	Stop(context.Context, string, string, string) (records.Record, error)
	Accessible(context.Context, string, string, string) (records.Record, error)
	List(context.Context, string, string, int, int) ([]records.Record, error)
	ClaimCommand(context.Context) (records.OutboxCommand, records.Record, error)
	CompleteCommand(context.Context, records.OutboxCommand) error
	RetryCommand(context.Context, records.OutboxCommand) error
}
type Reader interface {
	ReadComposite(context.Context, string) (records.RecordCard, error)
}
type Commander interface {
	StartRecord(context.Context, string, int) error
	StopRecord(context.Context, string, string) error
}
type Locker interface {
	Acquire(context.Context, string, string) (bool, error)
	Release(context.Context, string, string) error
}
type Events interface {
	Broadcast(context.Context, realtime.Envelope) error
}
type Service struct {
	repo     Repository
	reader   Reader
	commands Commander
	locker   Locker
	events   Events
}

func NewConferenceService(repo Repository, reader Reader, commands Commander, locker Locker, events Events) *Service {
	return &Service{repo, reader, commands, locker, events}
}

func (s *Service) Start(ctx context.Context, userID, conferenceID string, request records.ConferenceStartRequest) (records.RecordCard, error) {
	seconds := request.SegmentDurationSec
	if seconds == 0 {
		seconds = 5
	}
	if seconds < 2 || seconds > 30 {
		return records.RecordCard{}, apperrors.New(apperrors.ErrInvalidInput, "segmentDurationSec must be between 2 and 30")
	}
	record, created, err := s.repo.Start(ctx, userID, conferenceID, seconds)
	if err != nil {
		return records.RecordCard{}, err
	}
	if created {
		s.publish(ctx, record, "recording.starting")
	}
	return s.card(ctx, record.UUID)
}
func (s *Service) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Stop(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	if record.Status == records.StatusStopping {
		s.publish(ctx, record, "recording.stopping")
	}
	return s.card(ctx, record.UUID)
}
func (s *Service) Read(ctx context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	record, err := s.repo.Accessible(ctx, userID, conferenceID, recordID)
	if err != nil {
		return records.RecordCard{}, err
	}
	return s.card(ctx, record.UUID)
}
func (s *Service) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.RecordCard, error) {
	rows, err := s.repo.List(ctx, userID, conferenceID, limit, offset)
	if err != nil {
		return nil, err
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
func (s *Service) card(ctx context.Context, id string) (records.RecordCard, error) {
	card, err := s.reader.ReadComposite(ctx, id)
	card.Status = records.PublicStatus(card.Status)
	return card, err
}
func (s *Service) publish(ctx context.Context, record records.Record, kind string) {
	if s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": records.PublicStatus(record.Status), "mode": records.ModeComposite}))
	}
}

// A durable transactional outbox bridges PostgreSQL and RabbitMQ. Claims are
// fenced and expire; per-record ordering prevents stop overtaking start.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 16 {
			op, cancel := context.WithTimeout(ctx, 5*time.Second)
			command, record, err := s.repo.ClaimCommand(op)
			if err != nil {
				cancel()
				break
			}
			if records.IsTerminalStatus(record.Status) {
				err = s.repo.CompleteCommand(op, command)
				cancel()
				if err != nil {
					break
				}
				continue
			}
			if command.CommandType == "record.start" {
				if s.locker != nil {
					var acquired bool
					acquired, err = s.locker.Acquire(op, record.ConferenceID, record.UUID)
					if err == nil && !acquired {
						err = apperrors.ErrConflict
					}
				}
				if err == nil {
					err = s.commands.StartRecord(op, record.UUID, record.SegmentDurationSec)
				}
				if err == nil && record.Status == records.StatusStarting {
					s.publish(op, record, "recording.starting")
				}
			} else {
				err = s.commands.StopRecord(op, record.UUID, command.Reason)
				if err == nil {
					s.publish(op, record, "recording.stopping")
				}
			}
			if err == nil {
				err = s.repo.CompleteCommand(op, command)
			}
			if err != nil {
				_ = s.repo.RetryCommand(op, command)
				if !errors.Is(err, context.Canceled) {
					slog.Warn("recording command retry", "recording_id", record.UUID, "command", command.CommandType)
				}
			}
			cancel()
			if err != nil {
				break
			}
		}
	}
}
