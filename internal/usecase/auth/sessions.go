package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

type SessionRepository interface {
	Create(context.Context, users.AuthSession) error
	GetByHash(context.Context, string) (users.AuthSession, error)
	GetByID(context.Context, string) (users.AuthSession, error)
	RevokeByHash(context.Context, string) error
	RevokeByID(context.Context, string, string) error
	Bootstrap(context.Context, users.AuthSession, string, string) (users.AuthSession, error)
	RevokeLegacy(context.Context, string, string) error
	AuthorizeLegacy(context.Context, string, string) error
}

type sessionTokens interface {
	IssueSession(string, string) (string, error)
	VerifyAuthorization(string) (string, string, string, time.Time, error)
}

// WithSessions enables durable sign-in and revocation checks for the API.
func (s *Service) WithSessions(repository SessionRepository) *Service {
	s.sessions = repository
	return s
}

func (s *Service) signIn(ctx context.Context, user users.User) (users.LoginResponse, error) {
	if s.sessions == nil {
		token, err := s.tokens.Issue(user.ID)
		return loginResponse(user, token, ""), err
	}
	if user.GuestConferenceID != nil {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw := base64.RawURLEncoding.EncodeToString(secret[:])
	session := users.AuthSession{ID: uuid.NewString(), UserID: user.ID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	response, err := s.sessionResponse(user, session, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if err := s.sessions.Create(ctx, session); err != nil {
		return users.LoginResponse{}, err
	}
	return response, nil
}

func loginResponse(user users.User, token, raw string) users.LoginResponse {
	return users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View(), SessionToken: raw}
}

func (s *Service) sessionResponse(user users.User, session users.AuthSession, raw string) (users.LoginResponse, error) {
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	token, err := tokens.IssueSession(user.ID, session.ID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return loginResponse(user, token, raw), nil
}

func sessionHash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}

func validSessionSecret(raw string) bool {
	if len(raw) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == raw
}

func (s *Service) sessionForCookie(ctx context.Context, raw string) (users.AuthSession, error) {
	if s.sessions == nil || !validSessionSecret(raw) {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByHash(ctx, sessionHash(raw))
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && session.RevokedAt != nil {
		return users.AuthSession{}, apperrors.ErrUnauthorized
	}
	return session, err
}

func (s *Service) account(ctx context.Context, id string) (users.User, error) {
	user, err := s.repository.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.User{}, err
	}
	if user.GuestConferenceID != nil {
		return users.User{}, apperrors.ErrForbidden
	}
	return user, nil
}

// Refresh renews the short access token without imposing an idle/session deadline.
func (s *Service) Refresh(ctx context.Context, raw string) (users.LoginResponse, error) {
	session, err := s.sessionForCookie(ctx, raw)
	if err != nil {
		return users.LoginResponse{}, err
	}
	user, err := s.account(ctx, session.UserID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	return s.sessionResponse(user, session, raw)
}

// Bootstrap upgrades a valid existing account token. Repeated tabs reuse its cookie.
func (s *Service) Bootstrap(ctx context.Context, userID, raw, access string) (users.LoginResponse, error) {
	user, err := s.account(ctx, userID)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if s.sessions == nil {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return users.LoginResponse{}, apperrors.ErrUnavailable
	}
	identity, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
	if err != nil {
		return users.LoginResponse{}, err
	}
	if identity != userID || scope != "" {
		return users.LoginResponse{}, apperrors.ErrForbidden
	}
	legacyHash := ""
	if sessionID == "" {
		legacyHash = sessionHash(access)
	}
	var preferredID string
	var existingCookieID, existingCookie string
	if raw != "" {
		session, err := s.sessionForCookie(ctx, raw)
		if err == nil && session.UserID == userID {
			existingCookieID, existingCookie = session.ID, raw
			if sessionID != "" && sessionID == session.ID {
				return s.sessionResponse(user, session, raw)
			}
			if sessionID == "" {
				preferredID = session.ID
			}
		}
		if err != nil && !errors.Is(err, apperrors.ErrUnauthorized) {
			return users.LoginResponse{}, err
		}
	}
	if sessionID != "" {
		// A short-lived JS-readable access token must never mint a durable secret.
		// Matching cookies are reused above; cookie loss requires a fresh login.
		return users.LoginResponse{}, apperrors.ErrUnauthorized
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return users.LoginResponse{}, err
	}
	raw = base64.RawURLEncoding.EncodeToString(secret[:])
	candidate := users.AuthSession{ID: uuid.NewString(), UserID: userID, TokenHash: sessionHash(raw), CreatedAt: time.Now().UTC()}
	session, err := s.sessions.Bootstrap(ctx, candidate, legacyHash, preferredID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrUnauthorized
	}
	if err != nil {
		return users.LoginResponse{}, err
	}
	if session.ID == existingCookieID {
		raw = existingCookie
	}
	return s.sessionResponse(user, session, raw)
}

// Logout revokes both the current browser cookie and a supplied valid SID token.
func (s *Service) Logout(ctx context.Context, raw, access string) error {
	if s.sessions == nil {
		return nil
	}
	if validSessionSecret(raw) {
		if err := s.sessions.RevokeByHash(ctx, sessionHash(raw)); err != nil {
			return err
		}
	}
	if tokens, ok := s.tokens.(sessionTokens); ok && access != "" {
		userID, scope, sessionID, _, err := tokens.VerifyAuthorization(access)
		if err == nil && sessionID != "" {
			return s.sessions.RevokeByID(ctx, sessionID, userID)
		}
		if err == nil {
			if scope == "" {
				return s.sessions.RevokeLegacy(ctx, sessionHash(access), userID)
			}
		}
	}
	return nil
}

// AuthorizeSession also serves long-lived WebSocket/SSE connections after JWT expiry.
func (s *Service) AuthorizeSession(ctx context.Context, userID, sessionID string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if s.sessions == nil || sessionID == "" {
		return apperrors.ErrUnauthorized
	}
	session, err := s.sessions.GetByID(ctx, sessionID)
	if errors.Is(err, apperrors.ErrNotFound) || err == nil && (session.UserID != userID || session.RevokedAt != nil) {
		return apperrors.ErrUnauthorized
	}
	return err
}

func (s *Service) VerifyAuthorization(ctx context.Context, raw string) (string, string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tokens, ok := s.tokens.(sessionTokens)
	if !ok {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	userID, scope, sessionID, expiry, err := tokens.VerifyAuthorization(raw)
	if err == nil && sessionID != "" {
		err = s.AuthorizeSession(ctx, userID, sessionID)
	}
	if err == nil && sessionID == "" && scope == "" && s.sessions != nil {
		err = s.sessions.AuthorizeLegacy(ctx, sessionHash(raw), userID)
	}
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return userID, scope, sessionID, expiry, nil
}

func (s *Service) VerifySession(raw string) (string, string, time.Time, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	userID, scope, _, expiry, err := s.VerifyAuthorization(ctx, raw)
	return userID, scope, expiry, err
}

func (s *Service) VerifyWithExpiry(raw string) (string, time.Time, error) {
	userID, _, expiry, err := s.VerifySession(raw)
	return userID, expiry, err
}

func (s *Service) Verify(raw string) (string, error) {
	userID, _, err := s.VerifyWithExpiry(raw)
	return userID, err
}
