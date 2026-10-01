package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"gorm.io/gorm"
)

type SessionRepository struct{ db *gorm.DB }

func NewSessionRepository(db *gorm.DB) *SessionRepository { return &SessionRepository{db: db} }

func authorizeSession(db *gorm.DB, conferenceID, userID string, locked bool) (conferences.Participant, error) {
	c, err := findConference(db, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	p, err := findMembership(db, conferenceID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return p, apperrors.ErrForbidden
	}
	if err != nil {
		return p, err
	}
	if c.Status == conferences.Finished || c.Status == conferences.Cancelled {
		return p, apperrors.New(apperrors.ErrConflict, "conference is closed")
	}
	if p.Status != conferences.Joined {
		return p, apperrors.New(apperrors.ErrForbidden, "join the conference before connecting")
	}
	return p, nil
}
func (r *SessionRepository) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return authorizeSession(r.db.WithContext(ctx), conferenceID, userID, false)
}
func (r *SessionRepository) Open(ctx context.Context, session realtime.Session) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := authorizeSession(tx, session.ConferenceID, session.UserID, true)
		if err != nil {
			return err
		}
		if p.ID != session.ParticipantID {
			return apperrors.ErrForbidden
		}
		return tx.Create(&session).Error
	})
}
func (r *SessionRepository) Close(ctx context.Context, connectionID string, seen time.Time) error {
	// Idempotent; PostgreSQL is updated only on open/close, not on every pong.
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session realtime.Session
		if err := tx.Where("connection_id = ?", connectionID).Take(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		// Same lock order as media state updates, Open, kick and finish.
		if _, err := findConference(tx, session.ConferenceID, true); err != nil {
			return err
		}
		return tx.Model(&realtime.Session{}).Where("connection_id = ? AND status = 'connected'", connectionID).
			Updates(map[string]any{"status": "disconnected", "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "last_seen_at": gorm.Expr("GREATEST(connected_at, ?)", seen), "disconnected_at": gorm.Expr("GREATEST(connected_at, ?, NOW())", seen)}).Error
	})
}
func (r *SessionRepository) Stale(ctx context.Context, before time.Time, cursor string) ([]realtime.Session, error) {
	var rows []realtime.Session
	query := r.db.WithContext(ctx).Where("status = 'connected' AND connected_at < ?", before)
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}
	err := query.Order("id").Limit(500).Find(&rows).Error
	return rows, err
}
func (r *SessionRepository) Roster(ctx context.Context, conferenceID string) (conferences.Status, []conferences.Participant, error) {
	c, err := findConference(r.db.WithContext(ctx), conferenceID, false)
	if err != nil {
		return "", nil, err
	}
	rows := []conferences.Participant{}
	err = r.db.WithContext(ctx).Where("conference_id = ?", conferenceID).Order("created_at, id").Find(&rows).Error
	return c.Status, rows, err
}
