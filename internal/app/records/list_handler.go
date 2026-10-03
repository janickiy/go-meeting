package recordsapp

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// List возвращает список записей.
// @args
// - c: контекст HTTP-запроса Gin.
// @return JSON response.
func (h *Handler) List(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	items, err := h.service.List(c.Request.Context(), limit, offset)
	if err != nil {
		failed(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

// queryInt разбирает целочисленный параметр URL и применяет значение по умолчанию и допустимые границы.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - key (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
//   - fallback (int): значение, используемое при отсутствии входного параметра.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
func queryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
