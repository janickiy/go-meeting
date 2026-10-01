package recordsapp

import (
	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// failed формирует безопасный HTTP-ответ об ошибке прикладного сценария.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - status (int): состояние ресурса, ответа или фильтра выборки.
//   - message (string): сообщение чата или безопасный текст ответа согласно указанному типу.
func failed(c *gin.Context, status int, message string) {
	c.JSON(status, records.Response{Status: "failed", Message: message})
}
