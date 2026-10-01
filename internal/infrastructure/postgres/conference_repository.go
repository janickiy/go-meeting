package postgres

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ConferenceRepository struct{ db *gorm.DB }

func NewConferenceRepository(db *gorm.DB) *ConferenceRepository { return &ConferenceRepository{db: db} }

func (r *ConferenceRepository) Create(ctx context.Context, conference conferences.Conference, owner conferences.Participant) (conferences.Conference, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conference).Error; err != nil {
			return err
		}
		return tx.Create(&owner).Error
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "conferences_invite_code_key" {
		return conferences.Conference{}, conferences.ErrInviteCollision
	}
	return conference, err
}

func (r *ConferenceRepository) Get(ctx context.Context, id string) (conferences.Conference, error) {
	return findConference(r.db.WithContext(ctx), id, false)
}

func (r *ConferenceRepository) GetByInvite(ctx context.Context, code string) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Where("invite_code = ?", code).Take(&conference).Error
	return conference, mapNotFound(err)
}

func (r *ConferenceRepository) ListForUser(ctx context.Context, userID string, limit, offset int) ([]conferences.Conference, error) {
	items := []conferences.Conference{}
	err := r.db.WithContext(ctx).Model(&conferences.Conference{}).
		Joins("JOIN conference_participants p ON p.conference_id = conferences.id").
		Where("p.user_id = ? AND p.admission_state IN ('admitted','waiting')", userID).Order("conferences.created_at DESC, conferences.id").
		Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

func (r *ConferenceRepository) Membership(ctx context.Context, id, userID string) (conferences.Participant, error) {
	return findMembership(r.db.WithContext(ctx), id, userID)
}

func (r *ConferenceRepository) Participants(ctx context.Context, id string, limit, offset int) ([]conferences.Participant, error) {
	items := []conferences.Participant{}
	err := r.db.WithContext(ctx).Where("conference_id = ?", id).
		Order("created_at ASC, id").Limit(limit).Offset(offset).Find(&items).Error
	return items, err
}

func (r *ConferenceRepository) Transition(ctx context.Context, id, userID string, target conferences.Status) (conferences.Conference, error) {
	var conference conferences.Conference
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		conference, err = findConference(tx, id, true)
		if err != nil {
			return err
		}
		if conference.OwnerID != userID {
			return apperrors.ErrForbidden
		}
		if !conferences.CanTransition(conference.Status, target) {
			return apperrors.New(apperrors.ErrConflict, "conference status does not allow this transition")
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": target}
		if target == conferences.Active {
			updates["started_at"] = now
		} else {
			updates["finished_at"] = now
		}
		if err := tx.Model(&conference).Updates(updates).Error; err != nil {
			return err
		}
		if target == conferences.Finished || target == conferences.Cancelled {
			if err := stopConferenceRecordings(tx, id); err != nil {
				return err
			}
			if err := tx.Model(&conferences.Participant{}).
				Where("conference_id = ? AND status = ?", id, conferences.Joined).
				Updates(map[string]any{"status": conferences.Left, "left_at": now, "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
				return err
			}
		}
		conference, err = findConference(tx, id, false)
		return err
	})
	return conference, err
}

func (r *ConferenceRepository) Join(ctx context.Context, id string, user users.User, inviteCode string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conference, err := findConference(tx, id, true)
		if err != nil {
			return err
		}
		if conference.Status != conferences.Created && conference.Status != conferences.Active && conference.Status != conferences.Scheduled {
			return apperrors.New(apperrors.ErrConflict, "conference is closed")
		}
		participant, err = findMembership(tx, id, user.ID)
		missing := errors.Is(err, apperrors.ErrNotFound)
		if err != nil && !missing {
			return err
		}
		if missing && subtle.ConstantTimeCompare([]byte(inviteCode), []byte(conference.InviteCode)) != 1 {
			return apperrors.New(apperrors.ErrForbidden, "inviteCode is required for a new membership")
		}
		if !missing && participant.CanParticipate() {
			return nil
		}
		if !missing && (participant.Status == conferences.Kicked || participant.Status == conferences.Rejected || participant.AdmissionState == conferences.AdmissionKicked || participant.AdmissionState == conferences.AdmissionRejected) {
			return apperrors.New(apperrors.ErrForbidden, "this membership cannot rejoin the conference")
		}
		now := time.Now().UTC()
		if missing {
			userID := user.ID
			participant = conferences.Participant{ID: uuid.NewString(), ConferenceID: id, UserID: &userID,
				DisplayName: user.ParticipantName(), Role: conferences.ParticipantRole, Status: conferences.Joined, JoinedAt: &now, AdmissionState: conferences.AdmissionAdmitted}
			if conference.WaitingRoomEnabled {
				participant.Status, participant.AdmissionState, participant.JoinedAt = conferences.Waiting, conferences.AdmissionWaiting, nil
			} else if conference.Status == conferences.Scheduled {
				participant.Status, participant.JoinedAt = conferences.Left, nil
			}
			return tx.Create(&participant).Error
		}
		// Repeated invites cannot change admission; scheduled joins are enrollment.
		if participant.AdmissionState == conferences.AdmissionWaiting {
			if participant.Status == conferences.Waiting {
				return nil
			}
			return tx.Model(&participant).Update("status", conferences.Waiting).Error
		}
		if conference.Status == conferences.Scheduled {
			return nil
		}
		if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Joined, "joined_at": now, "left_at": nil, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
			return err
		}
		participant, err = findMembership(tx, id, user.ID)
		return err
	})
	return participant, err
}

func (r *ConferenceRepository) Leave(ctx context.Context, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := findConference(tx, id, true); err != nil {
			return err
		}
		var err error
		participant, err = findMembership(tx, id, userID)
		if errors.Is(err, apperrors.ErrNotFound) {
			return apperrors.ErrForbidden
		}
		if err != nil {
			return err
		}
		if participant.Status == conferences.Waiting {
			return tx.Model(&participant).Update("status", conferences.Left).Error
		}
		if participant.Status != conferences.Joined {
			return nil
		}
		if err := tx.Model(&participant).Updates(map[string]any{"status": conferences.Left, "left_at": time.Now().UTC(), "microphone_enabled": false, "camera_enabled": false, "screen_sharing": false, "media_policy_version": gorm.Expr("media_policy_version + 1")}).Error; err != nil {
			return err
		}
		participant, err = findMembership(tx, id, userID)
		return err
	})
	return participant, err
}

func findConference(db *gorm.DB, id string, locked bool) (conferences.Conference, error) {
	if locked {
		db = db.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var conference conferences.Conference
	err := db.Where("id = ?", id).Take(&conference).Error
	return conference, mapNotFound(err)
}

func findMembership(db *gorm.DB, id, userID string) (conferences.Participant, error) {
	var participant conferences.Participant
	err := db.Where("conference_id = ? AND user_id = ?", id, userID).Take(&participant).Error
	return participant, mapNotFound(err)
}
