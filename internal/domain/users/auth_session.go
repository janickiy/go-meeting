package users

import "time"

// AuthSession retains an account sign-in until that browser explicitly logs out.
// Only a SHA-256 digest of the opaque cookie is stored; no bearer secret is persisted.
type AuthSession struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;not null"`
	TokenHash string    `gorm:"-"`
	CreatedAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
}
