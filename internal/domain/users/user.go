package users

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

type User struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"-"`
	Email        string    `json:"-"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	DisplayName  *string   `gorm:"column:display_name" json:"-"`
	CreatedAt    time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

const (
	MinPasswordCharacters = 8
	MaxPasswordCharacters = 128
)

func (User) TableName() string { return "users" }

type View struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName *string   `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (u User) View() View {
	return View{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

func (u User) ParticipantName() string {
	if u.DisplayName != nil && *u.DisplayName != "" {
		return *u.DisplayName
	}
	return "Participant"
}

type RegisterRequest struct {
	Email       string  `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"displayName"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Status      string `json:"status"`
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	ExpiresIn   int    `json:"expiresIn"`
	User        View   `json:"user"`
}

func NormalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func ValidateEmail(value string) error {
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 || !strings.Contains(value, "@") {
		return apperrors.New(apperrors.ErrInvalidInput, "email must be a valid address, up to 254 bytes")
	}
	return nil
}

func NormalizeRegister(request RegisterRequest) (RegisterRequest, error) {
	request.Email = NormalizeEmail(request.Email)
	if err := ValidateEmail(request.Email); err != nil {
		return request, err
	}
	length := utf8.RuneCountInString(request.Password)
	if !utf8.ValidString(request.Password) || length < MinPasswordCharacters || length > MaxPasswordCharacters {
		return request, apperrors.New(apperrors.ErrInvalidInput, "password must contain 8 to 128 characters")
	}
	if request.DisplayName != nil {
		name := strings.TrimSpace(*request.DisplayName)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			return request, apperrors.New(apperrors.ErrInvalidInput, "displayName must be at most 100 characters")
		}
		request.DisplayName = nil
		if name != "" {
			request.DisplayName = &name
		}
	}
	return request, nil
}
