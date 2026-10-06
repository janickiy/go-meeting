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

type sessionClaims struct {
	jwt.RegisteredClaims
	GuestConferenceID string `json:"guestConferenceId,omitempty"`
	SessionID         string `json:"sid,omitempty"`
}

// TokenService выпускает и проверяет JWT учётной записи с настроенным сроком жизни.
//   - secret: секрет подписи или внутренней авторизации компонента.
//   - now: операция now с контрактом, описанным у метода.
type TokenService struct {
	secret []byte
	now    func() time.Time
}

// NewTokenService создаёт и связывает зависимости компонента TokenService, используемого в проверке учётных данных и ограниченных разрешений.
//
// @args
//   - secret (string): секрет подписи или внутренней авторизации компонента.
//
// @return:
//   - результат 1 (*TokenService): созданный компонент с переданными зависимостями.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NewTokenService(secret string) (*TokenService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	return &TokenService{secret: []byte(secret), now: time.Now}, nil
}

// Issue выпускает подписанный JWT пользователя с настроенным сроком действия.
//
// @args
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *TokenService) Issue(userID string) (string, error) {
	return s.issue(userID, "", "")
}

// IssueSession binds a short-lived access token to a revocable account session.
func (s *TokenService) IssueSession(userID, sessionID string) (string, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid account session")
	}
	return s.issue(userID, "", id.String())
}

// IssueGuest grants only the invited meeting; it never grants account access.
func (s *TokenService) IssueGuest(userID, conferenceID string) (string, error) {
	id, err := uuid.Parse(conferenceID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid guest conference")
	}
	return s.issue(userID, id.String(), "")
}

func (s *TokenService) issue(userID, conferenceID, sessionID string) (string, error) {
	id, err := uuid.Parse(userID)
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("invalid token subject")
	}
	now := s.now().UTC()
	claims := sessionClaims{GuestConferenceID: conferenceID, SessionID: sessionID, RegisteredClaims: jwt.RegisteredClaims{Subject: id.String(), Issuer: tokenIssuer,
		Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Verify проверяет подпись и срок JWT и извлекает идентификатор пользователя.
//
// @args
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *TokenService) Verify(raw string) (string, error) {
	id, _, err := s.VerifyWithExpiry(raw)
	return id, err
}

// VerifyWithExpiry проверяет подпись и содержимое JWT и возвращает идентификатор пользователя вместе со сроком действия.
//
// @args
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (time.Time): временная отметка результата или окончания действия разрешения.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *TokenService) VerifyWithExpiry(raw string) (string, time.Time, error) {
	id, _, expiry, err := s.VerifySession(raw)
	return id, expiry, err
}

// VerifySession validates the signature and returns the optional meeting scope.
func (s *TokenService) VerifySession(raw string) (string, string, time.Time, error) {
	id, conferenceID, _, expiry, err := s.VerifyAuthorization(raw)
	return id, conferenceID, expiry, err
}

// VerifyAuthorization validates JWT claims; the application also checks session revocation.
func (s *TokenService) VerifyAuthorization(raw string) (string, string, string, time.Time, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	claims := &sessionClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, /* Вложенный обработчик выполняет выделенный шаг обработки в проверке учётных данных и ограниченных разрешений, используя состояние окружающей функции.

		@args
		  - _ (*jwt.Token): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.

		@return:
		  - результат 1 (any): значение, подготовленное операцией для вызывающей стороны.
		  - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return "", "", "", time.Time{}, apperrors.ErrUnauthorized
	}
	if claims.GuestConferenceID != "" {
		conferenceID, err := uuid.Parse(claims.GuestConferenceID)
		if err != nil || conferenceID == uuid.Nil || conferenceID.String() != claims.GuestConferenceID {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	if claims.SessionID != "" {
		sessionID, err := uuid.Parse(claims.SessionID)
		if err != nil || sessionID == uuid.Nil || sessionID.String() != claims.SessionID || claims.GuestConferenceID != "" {
			return "", "", "", time.Time{}, apperrors.ErrUnauthorized
		}
	}
	return id.String(), claims.GuestConferenceID, claims.SessionID, claims.ExpiresAt.Time, nil
}
