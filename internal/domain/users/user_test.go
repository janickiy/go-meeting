package users

import (
	"errors"
	"strings"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// TestRegistrationPasswordCharacterLimits проверяет сценарий «Registration Password Character ограничения», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRegistrationPasswordCharacterLimits(t *testing.T) {
	cases := []struct {
		name, password string
		valid          bool
	}{
		{"empty", "", false},
		{"seven latin characters", "abcdefg", false},
		{"eight latin characters without composition requirements", "abcdefgh", true},
		{"digits only", "12345678", true},
		{"seven cyrillic characters", "абвгдеж", false},
		{"eight cyrillic characters", "абвгдежз", true},
		{"seven supplementary unicode characters", strings.Repeat("😀", 7), false},
		{"eight supplementary unicode characters", strings.Repeat("😀", 8), true},
		{"128 latin characters", strings.Repeat("a", 128), true},
		{"129 latin characters", strings.Repeat("a", 129), false},
		{"128 cyrillic characters", strings.Repeat("я", 128), true},
		{"129 cyrillic characters", strings.Repeat("я", 129), false},
		{"128 supplementary unicode characters", strings.Repeat("😀", 128), true},
		{"129 supplementary unicode characters", strings.Repeat("😀", 129), false},
		{"spaces are not trimmed", " abcd e ", true},
		{"invalid UTF-8", string([]byte{0xff}) + "abcdefgh", false},
	}
	for _, test := range cases {
		t.Run(test.name, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@args
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				request, err := NormalizeRegister(RegisterRequest{Email: "test@example.com", Password: test.password})
				if test.valid {
					if err != nil {
						t.Fatal(err)
					}
					if request.Password != test.password {
						t.Fatal("password was modified")
					}
				} else if !errors.Is(err, apperrors.ErrInvalidInput) {
					t.Fatal("invalid password was accepted")
				}
			})
	}
}
