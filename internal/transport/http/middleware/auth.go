package httpmiddleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

const authenticatedUserIDKey = "authenticated_user_id"

// TokenVerifier задаёт контракт зависимого компонента TokenVerifier в проверке HTTP-авторизации и ограничений запросов; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Verify: операция Verify с контрактом, описанным у метода.
type TokenVerifier interface {
	// Verify проверяет подпись, срок и содержимое переданного разрешения согласно контракту сервиса.
	//
	// @parameters:
	//   - аргумент 1 (string): исходные байты JSON, пакета или сериализованного значения.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Verify(string) (string, error)
}

// Authenticate создаёт HTTP-посредник проверки Bearer JWT и сохраняет подтверждённый идентификатор пользователя в контексте.
//
// @parameters:
//   - tokens (TokenVerifier): сервис выпуска или проверки JWT авторизации.
//
// @return:
//   - результат 1 (gin.HandlerFunc): обработчик Gin для включения в HTTP-маршруты.
func Authenticate(tokens TokenVerifier) gin.HandlerFunc {
	// Вложенный обработчик выполняет выделенный шаг обработки в проверке HTTP-авторизации и ограничений запросов, используя состояние окружающей функции.
	//
	// @parameters:
	//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
	return func(c *gin.Context) {
		header := strings.Fields(c.GetHeader("Authorization"))
		if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") || tokens == nil {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		id, err := tokens.Verify(header[1])
		if err != nil || id == "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		c.Set(authenticatedUserIDKey, id)
		c.Next()
	}
}

// UserID возвращает идентификатор пользователя, ранее проверенный HTTP-посредником авторизации.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func UserID(c *gin.Context) string { return c.GetString(authenticatedUserIDKey) }
