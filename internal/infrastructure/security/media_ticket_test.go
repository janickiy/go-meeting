package security

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
)

// mediaTicketFixture подготавливает или проверяет часть тестового сценария «медиа билет тестовое окружение».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*MediaTickets): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (media.Binding): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (media.Route): значение, подготовленное операцией для вызывающей стороны.
func mediaTicketFixture(t *testing.T) (*MediaTickets, media.Binding, media.Route) {
	t.Helper()
	s, err := NewMediaTickets(strings.Repeat("media-unit-secret-", 3), 45*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	s.now = func() time.Time { return now }
	b := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString(), AuthorizationExpiresAt: now.Add(time.Hour)}
	r := media.Route{WorkerID: "unit-worker", Endpoint: "http://worker:8091", LeaseID: uuid.NewString()}
	return s, b, r
}

// TestMediaTicketBindingAndExpiration проверяет сценарий «медиа билет Binding и Expiration», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaTicketBindingAndExpiration(t *testing.T) {
	s, b, r := mediaTicketFixture(t)
	raw, err := s.Issue(b, r)
	if err != nil {
		t.Fatal(err)
	}
	actual, owner, err := s.Verify(raw)
	if err != nil || actual != b || owner != r {
		t.Fatal("ticket binding lost or rejected", err)
	}
	now := s.now()
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	s.now = func() time.Time { return now.Add(46 * time.Second) }
	if _, _, err = s.Verify(raw); err == nil {
		t.Fatal("expired ticket accepted")
	}
	b.AuthorizationExpiresAt = now.Add(10 * time.Second)
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	s.now = func() time.Time { return now }
	raw, err = s.Issue(b, r)
	if err != nil {
		t.Fatal(err)
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	//
	// @return:
	//   - результат 1 (time.Time): временная отметка результата или окончания действия разрешения.
	s.now = func() time.Time { return now.Add(11 * time.Second) }
	if _, _, err = s.Verify(raw); err == nil {
		t.Fatal("ticket outlived original authorization")
	}
}

// TestMediaTicketRejectsForgedPurposeAlgorithmAndIdentity проверяет сценарий «медиа билет Rejects Forged Purpose Algorithm и Identity», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMediaTicketRejectsForgedPurposeAlgorithmAndIdentity(t *testing.T) {
	s, b, r := mediaTicketFixture(t)
	now := s.now()
	base := mediaClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: b.ConnectionID, ID: uuid.NewString(), Issuer: "go-recorder-api", Audience: jwt.ClaimStrings{"go-recorder-media"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(45 * time.Second))}, Purpose: "media", Binding: b, Route: r}
	cases := []struct {
		name   string
		change func(*mediaClaims)
		method jwt.SigningMethod
	}{
		{"purpose", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Purpose = "access" }, jwt.SigningMethodHS256},
		{"issuer", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Issuer = "foreign" }, jwt.SigningMethodHS256},
		{"audience", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Audience = jwt.ClaimStrings{"go-recorder-api"} }, jwt.SigningMethodHS256},
		{"subject", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Subject = uuid.NewString() }, jwt.SigningMethodHS256},
		{"identity", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Binding.ParticipantID = "not-uuid" }, jwt.SigningMethodHS256},
		{"missing expiry", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.ExpiresAt = nil }, jwt.SigningMethodHS256},
		{"missing issued", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.IssuedAt = nil }, jwt.SigningMethodHS256},
		{"future issued", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.IssuedAt = jwt.NewNumericDate(now.Add(time.Hour)) }, jwt.SigningMethodHS256},
		{"long ttl", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(10 * time.Minute)) }, jwt.SigningMethodHS256},
		{"lease", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - c (*mediaClaims): значение настроек или состояния компонента согласно указанному типу.
			*/func(c *mediaClaims) { c.Route.LeaseID = "" }, jwt.SigningMethodHS256},
		{"algorithm", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - аргумент 1 (*mediaClaims): значение для проверки, нормализации или преобразования.
			*/func(*mediaClaims) {}, jwt.SigningMethodHS512},
	}
	for _, tc := range cases {
		t.Run(tc.name, /* Вложенный обработчик выполняет отдельный вариант тестового сценария с проверкой результата и очисткой ресурсов.

			@args
			  - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
			*/func(t *testing.T) {
				claims := base
				tc.change(&claims)
				raw, err := jwt.NewWithClaims(tc.method, claims).SignedString(s.secret)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = s.Verify(raw); err == nil {
					t.Fatal("invalid ticket accepted")
				}
			})
	}
	raw, _ := s.Issue(b, r)
	other, _ := NewMediaTickets(strings.Repeat("different-key-", 4), 45*time.Second)
	if _, _, err := other.Verify(raw); err == nil {
		t.Fatal("wrong signing key accepted")
	}
	if _, _, err := s.Verify(strings.Repeat("x", 8193)); err == nil {
		t.Fatal("oversized ticket accepted")
	}
}
