package httpresponse

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

func Fail(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, apperrors.ErrInvalidInput):
		status, message = http.StatusUnprocessableEntity, "invalid input"
	case errors.Is(err, apperrors.ErrUnauthorized):
		status, message = http.StatusUnauthorized, "authentication required or token invalid"
	case errors.Is(err, apperrors.ErrForbidden):
		status, message = http.StatusForbidden, "access denied"
	case errors.Is(err, apperrors.ErrNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, apperrors.ErrConflict):
		status, message = http.StatusConflict, "conflict"
	case errors.Is(err, apperrors.ErrUnavailable):
		status, message = http.StatusServiceUnavailable, "service temporarily unavailable"
	}
	var applicationError *apperrors.Error
	if status != http.StatusInternalServerError && errors.As(err, &applicationError) {
		message = applicationError.Message
	}
	c.AbortWithStatusJSON(status, gin.H{"status": "failed", "message": message})
}

// BindJSON bounds new platform requests and rejects unknown fields and trailing JSON.
// It does not alter the legacy recorder's request contract.
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
