package auth

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

type userRepository interface {
	Create(context.Context, users.User) (users.User, error)
	GetByID(context.Context, string) (users.User, error)
	GetByEmail(context.Context, string) (users.User, error)
}

type passwordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, error)
}

type tokenIssuer interface{ Issue(string) (string, error) }

type Service struct {
	repository userRepository
	passwords  passwordHasher
	tokens     tokenIssuer
	dummyHash  string
}

func NewService(repository userRepository, passwords passwordHasher, tokens tokenIssuer) (*Service, error) {
	dummy, err := passwords.Hash("dummy-password-for-login-timing")
	if err != nil {
		return nil, fmt.Errorf("initialize authentication: %w", err)
	}
	return &Service{repository: repository, passwords: passwords, tokens: tokens, dummyHash: dummy}, nil
}

func (s *Service) Register(ctx context.Context, request users.RegisterRequest) (users.View, error) {
	request, err := users.NormalizeRegister(request)
	if err != nil {
		return users.View{}, err
	}
	hash, err := s.passwords.Hash(request.Password)
	if err != nil {
		return users.View{}, err
	}
	if err := ctx.Err(); err != nil {
		return users.View{}, err
	}
	user, err := s.repository.Create(ctx, users.User{ID: uuid.NewString(), Email: request.Email, PasswordHash: hash, DisplayName: request.DisplayName})
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

func (s *Service) Login(ctx context.Context, request users.LoginRequest) (users.LoginResponse, error) {
	request.Email = users.NormalizeEmail(request.Email)
	// Existing accounts may have fewer than eight characters under the old byte-based policy.
	// Apply the new minimum only at registration; never reject a valid existing password.
	if users.ValidateEmail(request.Email) != nil || request.Password == "" || !utf8.ValidString(request.Password) || utf8.RuneCountInString(request.Password) > users.MaxPasswordCharacters {
		return users.LoginResponse{}, invalidCredentials()
	}
	user, err := s.repository.GetByEmail(ctx, request.Email)
	missing := errors.Is(err, apperrors.ErrNotFound)
	if err != nil && !missing {
		return users.LoginResponse{}, err
	}
	hash := user.PasswordHash
	if missing {
		hash = s.dummyHash
	}
	valid, err := s.passwords.Verify(request.Password, hash)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if missing || !valid {
		return users.LoginResponse{}, invalidCredentials()
	}
	if err := ctx.Err(); err != nil {
		return users.LoginResponse{}, err
	}
	token, err := s.tokens.Issue(user.ID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View()}, nil
}

func (s *Service) Me(ctx context.Context, userID string) (users.View, error) {
	user, err := s.repository.GetByID(ctx, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.View{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.View{}, err
	}
	return user.View(), nil
}

func invalidCredentials() error {
	return apperrors.New(apperrors.ErrUnauthorized, "invalid email or password")
}
