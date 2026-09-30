package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type UserRepository struct{ db *gorm.DB }

func NewUserRepository(db *gorm.DB) *UserRepository {
	// An INSERT failure must never cause GORM to log password_hash parameters.
	return &UserRepository{db: db.Session(&gorm.Session{Logger: logger.Discard})}
}

func (r *UserRepository) Create(ctx context.Context, user users.User) (users.User, error) {
	err := r.db.WithContext(ctx).Create(&user).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return users.User{}, apperrors.New(apperrors.ErrConflict, "email is already registered")
	}
	return user, err
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&user).Error
	return user, mapNotFound(err)
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (users.User, error) {
	var user users.User
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&user).Error
	return user, mapNotFound(err)
}

func mapNotFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperrors.ErrNotFound
	}
	return err
}
