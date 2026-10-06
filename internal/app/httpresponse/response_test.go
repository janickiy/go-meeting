package httpresponse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestInternalErrorsDoNotExposeInfrastructureDetails проверяет сокрытие деталей инфраструктуры во внутренних ошибках.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestInternalErrorsDoNotExposeInfrastructureDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/fail", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
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

func TestApplicationErrorHTTPContract(t *testing.T) {
	for _, test := range []struct {
		err     error
		status  int
		message string
	}{
		{apperrors.ErrInvalidInput, 422, "invalid input"}, {apperrors.ErrUnauthorized, 401, "authentication required or token invalid"},
		{apperrors.ErrForbidden, 403, "access denied"}, {apperrors.ErrNotFound, 404, "not found"}, {apperrors.ErrConflict, 409, "conflict"},
		{apperrors.ErrUnavailable, 503, "service temporarily unavailable"}, {apperrors.ErrRateLimited, 429, "rate limit exceeded"},
		{apperrors.ErrTimeout, 504, "operation timed out"}, {apperrors.ErrInternal, 500, "internal server error"},
		{context.DeadlineExceeded, 500, "internal server error"},
		{apperrors.Wrap(apperrors.ErrInternal, errors.New("private DB"), "private override"), 500, "internal server error"},
		{apperrors.Wrap(apperrors.ErrForbidden, errors.New("private DB"), "safe detail"), 403, "safe detail"},
	} {
		t.Run(test.message+strconv.Itoa(test.status), func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			Fail(c, fmt.Errorf("operation: %w", test.err))
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != test.status || body["status"] != "failed" || body["message"] != test.message || len(body) != 2 {
				t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
