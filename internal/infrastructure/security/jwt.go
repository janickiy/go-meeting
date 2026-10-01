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

// TokenService выпускает и проверяет JWT учётной записи с настроенным сроком жизни.
//   - secret: секрет подписи или внутренней авторизации компонента.
//   - now: операция now с контрактом, описанным у метода.
type TokenService struct {
	secret []byte
	now    func() time.Time
}

// NewTokenService создаёт и связывает зависимости компонента TokenService, используемого в проверке учётных данных и ограниченных разрешений.
//
// @parameters:
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
// @parameters:
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// Verify проверяет подпись и срок JWT и извлекает идентификатор пользователя.
//
// @parameters:
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
// @parameters:
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (time.Time): временная отметка результата или окончания действия разрешения.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *TokenService) VerifyWithExpiry(raw string) (string, time.Time, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return "", time.Time{}, apperrors.ErrUnauthorized
	}
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, /* Вложенный обработчик выполняет выделенный шаг обработки в проверке учётных данных и ограниченных разрешений, используя состояние окружающей функции.

		@parameters:
		  - _ (*jwt.Token): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.

		@return:
		  - результат 1 (any): значение, подготовленное операцией для вызывающей стороны.
		  - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(_ *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithIssuer(tokenIssuer), jwt.WithAudience(tokenAudience), jwt.WithTimeFunc(s.now))
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return "", time.Time{}, apperrors.ErrUnauthorized
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return "", time.Time{}, apperrors.ErrUnauthorized
	}
	return id.String(), claims.ExpiresAt.Time, nil
}
