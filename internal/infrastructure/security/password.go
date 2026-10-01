package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 4
	saltLength   = 16
	keyLength    = 32
)

// PasswordHasher предоставляет хеширование и проверку пароля без хранения открытого текста.
type PasswordHasher struct{}

// Hash вычисляет защищённый хеш пароля для сохранения вместо открытого текста.
//
// @parameters:
//   - password (string): открытый пароль для хеширования или проверки; не предназначен для журналирования.
//
// @return:
//   - результат 1 (string): строка Argon2id с версией, фиксированными параметрами, случайной солью и хешем.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (PasswordHasher) Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify сравнивает открытый пароль с сохранённым защищённым хешем.
//
// @parameters:
//   - password (string): открытый пароль для хеширования или проверки; не предназначен для журналирования.
//   - encoded (string): сохранённая строка Argon2id с параметрами, солью и ожидаемым хешем.
//
// @return:
//   - результат 1 (bool): true при совпадении пароля с хешем; сравнение выполняется за постоянное время.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (PasswordHasher) Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// Фиксированные параметры и версия не позволяют повреждённому хешу вызвать чрезмерное выделение памяти.
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=4" {
		return false, fmt.Errorf("invalid password hash format")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false, fmt.Errorf("invalid password salt")
	}
	want, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(want) != keyLength {
		return false, fmt.Errorf("invalid password hash")
	}
	got := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLength)
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
