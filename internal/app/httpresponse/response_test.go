package httpresponse

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestInternalErrorsDoNotExposeInfrastructureDetails проверяет сценарий «Internal ошибки Do не Expose Infrastructure Details», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestInternalErrorsDoNotExposeInfrastructureDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/fail", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@parameters:
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			Fail(c, errors.New("SQL failure: password_hash=private-hash, password=private-password"))
		})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/fail", nil))
	if response.Code != 500 || response.Body.String() != `{"message":"internal server error","status":"failed"}` {
		t.Fatal("internal error response leaked details or changed contract")
	}
}
