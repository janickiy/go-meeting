package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
)

// ProviderTokens шифрует секреты AES-256-GCM; дополнительный контекст запрещает перенос токена между пользователями и устройствами.
type ProviderTokens struct{ aead cipher.AEAD }

// NewProviderTokens проверяет независимый ключ шифрования, заданный в base64; пустой ключ отключает хранение секретов.
// @args key — base64-кодированный случайный 32-байтовый ключ, не JWT secret.
// @return: шифратор либо nil при отключении; ошибка некорректного ключа.
func NewProviderTokens(key string) (*ProviderTokens, error) {
	if key == "" {
		return nil, nil
	}
	b, err := base64.StdEncoding.Strict().DecodeString(key)
	if err != nil || len(b) != 32 {
		return nil, errors.New("provider encryption key must contain 32 base64-encoded bytes")
	}
	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &ProviderTokens{aead: aead}, nil
}

// Encrypt создаёт случайный nonce и аутентифицированный ciphertext, не сохраняя plaintext.
// @args value — секрет; binding — user/provider/entity контекст владельца.
// @return: base64 ciphertext либо ошибка генератора случайных чисел/отключённого шифрования.
func (p *ProviderTokens) Encrypt(value, binding string) (string, error) {
	if p == nil {
		return "", errors.New("provider encryption disabled")
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := p.aead.Seal(nonce, nonce, []byte(value), []byte(binding))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt проверяет ciphertext и контекст владельца перед расшифровкой.
// @args value — сохранённый ciphertext; binding — тот же контекст, что при Encrypt.
// @return: исходный секрет либо безопасная ошибка без токенов в тексте.
func (p *ProviderTokens) Decrypt(value, binding string) (string, error) {
	if p == nil {
		return "", errors.New("provider encryption disabled")
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || len(b) < p.aead.NonceSize() {
		return "", errors.New("invalid encrypted provider token")
	}
	n := p.aead.NonceSize()
	out, err := p.aead.Open(nil, b[:n], b[n:], []byte(binding))
	if err != nil {
		return "", errors.New("invalid encrypted provider token")
	}
	return string(out), nil
}
