package security

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

const AccessTokenTTL = time.Hour
const tokenIssuer = "go-recorder"
const tokenAudience = "go-recorder-api"

type TokenService struct {
	secret []byte
	now    func() time.Time
}

func NewTokenService(secret string) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &TokenService{secret: []byte(secret), now: time.Now}, nil
}

func (s *TokenService) Issue(userID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid token subject")
	}
	now := s.now().UTC()
	claims := jwt.RegisteredClaims{Subject: id.String(), Issuer: tokenIssuer,
		Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL))}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *TokenService) Verify(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return "", apperrors.ErrUnauthorized
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return "", apperrors.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return "", apperrors.ErrUnauthorized
	}
	return id.String(), nil
}
