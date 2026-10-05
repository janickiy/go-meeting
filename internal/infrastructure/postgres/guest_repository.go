package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type GuestRepository struct{ db *gorm.DB }

func NewGuestRepository(db *gorm.DB) *GuestRepository {
	return &GuestRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

// JoinGuest commits the identity and membership together. Rejected guests keep
// their existing identity when retrying with the same session.
func (r *GuestRepository) JoinGuest(ctx context.Context, code, name, resumeUserID string) (users.User, conferences.Participant, error) {
	var user users.User
	var participant conferences.Participant
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conference conferences.Conference
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("invite_code = ?", code).Take(&conference).Error; err != nil {
			return mapNotFound(err)
		}
		if conference.Status != conferences.Created && conference.Status != conferences.Active {
			return apperrors.New(apperrors.ErrConflict, "conference is not open for joining")
		}
		if resumeUserID != "" {
			if err := tx.Where("id = ?", resumeUserID).Take(&user).Error; err != nil {
				return mapNotFound(err)
			}
			if user.GuestConferenceID == nil {
				return apperrors.ErrForbidden
			}
			if *user.GuestConferenceID != conference.ID {
				user = users.User{}
			}
		}
		if user.ID == "" {
			user = users.User{ID: uuid.NewString(), DisplayName: &name, GuestConferenceID: &conference.ID, PasswordHash: "!guest"}
			user.Email = "guest-" + user.ID + "@guest.invalid"
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
		}
		var err error
		participant, err = NewConferenceRepository(tx).Join(ctx, conference.ID, user, code)
		if err != nil {
			return err
		}
		if err := tx.Model(&user).Update("display_name", name).Error; err != nil {
			return err
		}
		if err := tx.Model(&participant).Update("display_name", name).Error; err != nil {
			return err
		}
		user.DisplayName, participant.DisplayName = &name, name
		return nil
	})
	return user, participant, err
}
