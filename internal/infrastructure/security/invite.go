package security

import (
	"crypto/rand"
	"encoding/base64"
)

// GenerateInviteCode создаёт криптографически случайный код приглашения без предсказуемой последовательности.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func GenerateInviteCode() (string, error) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}
