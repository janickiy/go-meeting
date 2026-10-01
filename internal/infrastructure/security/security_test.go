package security

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testSecret = "only-for-tests-32-bytes-or-more-secret"

// TestPasswordHashesAreSaltedAndVerified проверяет сценарий «Password Hashes Are Salted и Verified», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPasswordHashesAreSaltedAndVerified(t *testing.T) {
	hasher := PasswordHasher{}
	password := "StrongPassword123"
	first, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := hasher.Hash(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "$argon2id$") || strings.Contains(first, password) {
		t.Fatal("hash is not independently salted")
	}
	for _, test := range []struct {
		password string
		want     bool
	}{{password, true}, {"wrong-password", false}} {
		got, err := hasher.Verify(test.password, first)
		if err != nil || got != test.want {
			t.Fatalf("Verify() = %v, %v", got, err)
		}
	}
	for _, bad := range []string{"plain-text", "$argon2id$v=19$m=999999999,t=3,p=4$a$b", strings.Replace(first, "v=19", "v=18", 1)} {
		if _, err := hasher.Verify(password, bad); err == nil {
			t.Fatal("corrupt or unsupported hash accepted")
		}
	}
}

// TestTokenValidation проверяет сценарий «токен проверка входных данных», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestTokenValidation(t *testing.T) {
	service, err := NewTokenService(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	service.now = func() time.Time { return now }
	id := uuid.NewString()
	raw, err := service.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.Verify(raw); err != nil || got != id {
		t.Fatalf("Verify() = %q, %v", got, err)
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	service.now = func() time.Time { return now.Add(time.Hour) }
	if _, err := service.Verify(raw); err == nil {
		t.Fatal("expired token accepted")
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	service.now = func() time.Time { return now }
	base := jwt.RegisteredClaims{Subject: id, Issuer: tokenIssuer, Audience: jwt.ClaimStrings{tokenAudience}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))}
	for _, name := range []string{"missing expiry", "missing issued-at", "future issued-at", "wrong issuer", "wrong audience", "bad subject", "wrong key", "wrong algorithm"} {
		t.Run(name, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@parameters:
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				claims := base
				method, key := jwt.SigningMethodHS256, []byte(testSecret)
				switch name {
				case "missing expiry":
					claims.ExpiresAt = nil
				case "missing issued-at":
					claims.IssuedAt = nil
				case "future issued-at":
					claims.IssuedAt = jwt.NewNumericDate(now.Add(time.Minute))
				case "wrong issuer":
					claims.Issuer = "other-app"
				case "wrong audience":
					claims.Audience = jwt.ClaimStrings{"other-app"}
				case "bad subject":
					claims.Subject = "not-a-user-uuid"
				case "wrong key":
					key = []byte("different-secret-for-testing")
				case "wrong algorithm":
					method = jwt.SigningMethodHS384
				}
				token, err := jwt.NewWithClaims(method, claims).SignedString(key)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := service.Verify(token); err == nil {
					t.Fatal("invalid token accepted")
				}
			})
	}
	if _, err := service.Verify(""); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := NewTokenService("short"); err == nil {
		t.Fatal("weak signing secret accepted")
	}
}

// TestInviteCodesAreIndependentRandomIdentifiers проверяет сценарий «Invite Codes Are Independent Random Identifiers», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestInviteCodesAreIndependentRandomIdentifiers(t *testing.T) {
	seen := make(map[string]bool)
	for range 20 {
		code, err := GenerateInviteCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 32 || seen[code] || strings.ContainsAny(code, "+/=") {
			t.Fatal("invalid or duplicate invite code")
		}
		seen[code] = true
	}
}
