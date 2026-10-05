package httpmiddleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

const authenticatedUserIDKey = "authenticated_user_id"

type SessionVerifier interface {
	VerifySession(string) (string, string, time.Time, error)
}

// GuestRequestAllowed keeps guest credentials inside one meeting, including WS.
// Normal conference admission/role checks still run after this scope check.
func GuestRequestAllowed(method, path, conferenceID string) bool {
	if conferenceID == "" {
		return true
	}
	if method == "GET" && (path == "/api/v1/auth/me" || path == "/api/v1/capabilities" || path == "/api/v1/webrtc/config") {
		return true
	}
	if method == "POST" && path == "/api/v1/auth/logout" {
		return true
	}
	base := "/api/v1/conferences/" + conferenceID
	return path == base || strings.HasPrefix(path, base+"/")
}

// TokenVerifier задаёт контракт зависимого компонента TokenVerifier в проверке HTTP-авторизации и ограничений запросов; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Verify: операция Verify с контрактом, описанным у метода.
type TokenVerifier interface {
	// Verify проверяет подпись, срок и содержимое переданного разрешения согласно контракту сервиса.
	//
	// @args
	//   - аргумент 1 (string): исходные байты JSON, пакета или сериализованного значения.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Verify(string) (string, error)
}

// Authenticate создаёт HTTP-посредник проверки Bearer JWT и сохраняет подтверждённый идентификатор пользователя в контексте.
//
// @args
//   - tokens (TokenVerifier): сервис выпуска или проверки JWT авторизации.
//
// @return:
//   - результат 1 (gin.HandlerFunc): обработчик Gin для включения в HTTP-маршруты.
func Authenticate(tokens TokenVerifier) gin.HandlerFunc {
	// Вложенный обработчик выполняет выделенный шаг обработки в проверке HTTP-авторизации и ограничений запросов, используя состояние окружающей функции.
	//
	// @args
	//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") || tokens == nil {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		var id, conferenceID string
		var err error
		if scoped, ok := tokens.(SessionVerifier); ok {
			id, conferenceID, _, err = scoped.VerifySession(header[1])
		} else {
			id, err = tokens.Verify(header[1])
		}
		if err != nil || id == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		if !GuestRequestAllowed(c.Request.Method, c.Request.URL.Path, conferenceID) {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		c.Set(authenticatedUserIDKey, id)
		c.Next()
	}
}

// UserID возвращает идентификатор пользователя, ранее проверенный HTTP-посредником авторизации.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func UserID(c *gin.Context) string { return c.GetString(authenticatedUserIDKey) }
