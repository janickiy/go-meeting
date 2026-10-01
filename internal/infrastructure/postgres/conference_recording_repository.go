package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConferenceRecordingRepository struct{ db *gorm.DB }

func NewConferenceRecordingRepository(db *gorm.DB) *ConferenceRecordingRepository {
	return &ConferenceRecordingRepository{db: db}
}

type RecordingOutbox = records.OutboxCommand

func (r *ConferenceRecordingRepository) Start(ctx context.Context, userID, conferenceID string, segmentDuration int) (records.Record, bool, error) {
	var record records.Record
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conference, err := findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		actor, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			return membershipError(err)
		}
		if conference.OwnerID != userID || actor.Role != conferences.Owner || actor.Status != conferences.Joined {
			return apperrors.ErrForbidden
		}
		if conference.Status != conferences.Active {
			return apperrors.New(apperrors.ErrConflict, "conference is not active")
		}
		err = tx.Where("platform_conference_id = ? AND mode = 'composite' AND status IN ?", conferenceID, activeRecordingStatuses()).Take(&record).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record = records.Record{UUID: uuid.NewString(), ConferenceID: conferenceID, PlatformConferenceID: &conferenceID, RequestedBy: &userID, Mode: records.ModeComposite, SourceType: "conference", TransportType: "sfu", Status: records.StatusStarting, QualityMode: "auto", SegmentDurationSec: segmentDuration, NeedPreview: true}
		if err = tx.Create(&record).Error; err != nil {
			return err
		}
		if err = enqueueRecording(tx, record.UUID, "record.start", ""); err != nil {
			return err
		}
		created = true
		return nil
	})
	return record, created, err
}

func (r *ConferenceRecordingRepository) Stop(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conference, err := findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		actor, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			return membershipError(err)
		}
		if conference.OwnerID != userID || actor.Role != conferences.Owner {
			return apperrors.ErrForbidden
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND platform_conference_id = ? AND mode = 'composite'", recordID, conferenceID).Take(&record).Error; err != nil {
			return mapNotFound(err)
		}
		if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
			return nil
		}
		if record.Status != records.StatusStopping {
			if err = tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "owner_requested"}).Error; err != nil {
				return err
			}
		}
		return enqueueRecording(tx, record.UUID, "record.stop", "owner_requested")
	})
	return record, err
}

func (r *ConferenceRecordingRepository) Accessible(ctx context.Context, userID, conferenceID, recordID string) (records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return records.Record{}, err
	}
	var record records.Record
	err := r.db.WithContext(ctx).Where("uuid = ? AND platform_conference_id = ? AND mode = 'composite'", recordID, conferenceID).Take(&record).Error
	return record, mapNotFound(err)
}

func (r *ConferenceRecordingRepository) List(ctx context.Context, userID, conferenceID string, limit, offset int) ([]records.Record, error) {
	if err := r.authorizeRead(ctx, userID, conferenceID); err != nil {
		return nil, err
	}
	items := []records.Record{}
	err := r.db.WithContext(ctx).Where("platform_conference_id = ? AND mode = 'composite'", conferenceID).Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

func (r *ConferenceRecordingRepository) authorizeRead(ctx context.Context, userID, conferenceID string) error {
	participant, err := findMembership(r.db.WithContext(ctx), conferenceID, userID)
	if err != nil {
		return membershipError(err)
	}
	if participant.Status == conferences.Kicked || participant.Status == conferences.Rejected {
		return apperrors.ErrForbidden
	}
	return nil
}

func activeRecordingStatuses() []string {
	return []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
}
func enqueueRecording(tx *gorm.DB, recordID, command, reason string) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&RecordingOutbox{RecordID: recordID, CommandType: command, Reason: reason}).Error
}

// The conference row is already locked by the caller, serializing finish with
// recording starts. Commands become durable in the same transaction as finish.
func stopConferenceRecordings(tx *gorm.DB, conferenceID string) error {
	var rows []records.Record
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("platform_conference_id = ? AND mode = 'composite' AND status IN ?", conferenceID, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping}).Find(&rows).Error; err != nil {
		return err
	}
	for _, record := range rows {
		if record.Status != records.StatusStopping {
			if err := tx.Model(&record).Updates(map[string]any{"status": records.StatusStopping, "stopped_at": time.Now().UTC(), "ended_reason": "conference_finished"}).Error; err != nil {
				return err
			}
		}
		if err := enqueueRecording(tx, record.UUID, "record.stop", "conference_finished"); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConferenceRecordingRepository) ClaimCommand(ctx context.Context) (RecordingOutbox, records.Record, error) {
	var out RecordingOutbox
	var record records.Record
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Raw(`SELECT o.* FROM recording_outbox o WHERE o.published_at IS NULL
		AND (o.claimed_until IS NULL OR o.claimed_until < now())
		AND NOT EXISTS (SELECT 1 FROM recording_outbox prev WHERE prev.record_id=o.record_id AND prev.id<o.id AND prev.published_at IS NULL)
		ORDER BY o.id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&out).Error
		if err != nil {
			return err
		}
		if out.ID == 0 {
			return apperrors.ErrNotFound
		}
		token := uuid.NewString()
		until := time.Now().UTC().Add(15 * time.Second)
		out.ClaimToken = &token
		out.ClaimedUntil = &until
		if err = tx.Model(&out).Updates(map[string]any{"claim_token": token, "claimed_until": until}).Error; err != nil {
			return err
		}
		return tx.Where("uuid = ?", out.RecordID).Take(&record).Error
	})
	return out, record, err
}

func (r *ConferenceRecordingRepository) CompleteCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"published_at": time.Now().UTC(), "claimed_until": nil}).Error
}

func (r *ConferenceRecordingRepository) RetryCommand(ctx context.Context, command RecordingOutbox) error {
	return r.db.WithContext(ctx).Model(&RecordingOutbox{}).Where("id = ? AND claim_token = ?", command.ID, command.ClaimToken).Updates(map[string]any{"claimed_until": time.Now().UTC().Add(time.Second)}).Error
}
