package httpresponse

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// InternalErrorMessage is shared with legacy responses that retain their status.
const InternalErrorMessage = "internal server error"

// Fail сопоставляет прикладную ошибку с HTTP-статусом и безопасным JSON-ответом.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - err (error): ошибка, которую необходимо классифицировать, сохранить или вернуть клиенту.
func Fail(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, InternalErrorMessage
	switch apperrors.Classify(err) {
	case apperrors.Validation:
		status, message = http.StatusUnprocessableEntity, "invalid input"
	case apperrors.Unauthenticated:
		status, message = http.StatusUnauthorized, "authentication required or token invalid"
	case apperrors.Forbidden:
		status, message = http.StatusForbidden, "access denied"
	case apperrors.NotFound:
		status, message = http.StatusNotFound, "not found"
	case apperrors.Conflict:
		status, message = http.StatusConflict, "conflict"
	case apperrors.RateLimited:
		status, message = http.StatusTooManyRequests, "rate limit exceeded"
	case apperrors.Timeout:
		status, message = http.StatusGatewayTimeout, "operation timed out"
	case apperrors.Unavailable:
		status, message = http.StatusServiceUnavailable, "service temporarily unavailable"
	}
	var applicationError *apperrors.Error
	if status != http.StatusInternalServerError && errors.As(err, &applicationError) {
		message = applicationError.Message
	}
	c.AbortWithStatusJSON(status, gin.H{"status": "failed", "message": message})
}

// BindJSON строго разбирает JSON-тело HTTP-запроса и сообщает безопасную ошибку формата.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - target (any): целевой объект, участник или состояние операции.
//   - optional (bool): логический признак optional, управляющий соответствующей веткой обработки.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func BindJSON(c *gin.Context, target any, optional bool) bool {
	if c.Request.Body == nil {
		c.Request.Body = http.NoBody
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if optional && errors.Is(err, io.EOF) {
		return true
	}
	if err == nil {
		var trailing any
		if errors.Is(decoder.Decode(&trailing), io.EOF) {
			return true
		}
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "failed", "message": "invalid json body or unknown fields"})
	return false
}

// Pagination разбирает предел и смещение страницы из URL и проверяет допустимые границы.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 3 (bool): признак выполнения проверяемого условия или изменения состояния.
func Pagination(c *gin.Context) (int, int, bool) {
	limit, offset := 20, 0
	var err error
	if raw, exists := c.GetQuery("limit"); exists {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
		return 0, 0, false
	}
	if raw, exists := c.GetQuery("offset"); exists {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "offset must be non-negative"))
		return 0, 0, false
	}
	return limit, offset, true
}
