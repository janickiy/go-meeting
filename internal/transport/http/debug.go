package httptransport

import (
	"context"
	_ "embed"
	"net/http"
	"strconv"

	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"

	"github.com/gin-gonic/gin"
)

//go:embed views/webrtc-smoke.html
var webrtcSmokeHTML string

// CompletedRecordsLister задаёт контракт зависимого компонента CompletedRecordsLister в регистрации и обработке HTTP-маршрутов; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// - ListCompletedRecords: получение списка завершённых записей с контрактом, описанным у метода.
type CompletedRecordsLister interface {
	// ListCompletedRecords возвращает записи, у которых в MinIO есть preview.jpg и final.mp4.
	// @args
	// - ctx: контекст HTTP-запроса.
	// - limit: максимальное количество записей.
	// @return список записей или ошибку хранилища.
	ListCompletedRecords(ctx context.Context, limit int) ([]s3storage.CompletedRecord, error)
}

// RegisterDebugRoutes регистрирует страницы отладки только для локального режима.
// @args
// - router: маршрутизатор Gin.
// - completedRecords: источник завершенных записей из MinIO.
// @return ничего.
func RegisterDebugRoutes(router gin.IRouter, completedRecords CompletedRecordsLister) {
	router.GET("/debug/webrtc-smoke", /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			noStore(c)
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(webrtcSmokeHTML))
		})

	if completedRecords == nil {
		return
	}
	router.GET("/debug/records/completed", /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			noStore(c)
			items, err := completedRecords.ListCompletedRecords(c.Request.Context(), debugLimit(c, 50))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status":  "failed",
					"message": err.Error(),
				})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"items":  items,
			})
		})
}

// noStore отключает кеш браузера для маршрутов отладки.
// @args
// - c: контекст HTTP-запроса Gin.
// @return ничего.
func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// debugLimit читает параметр limit строки запроса для маршрутов отладки.
// @args
// - c: контекст HTTP-запроса Gin.
// - fallback: значение по умолчанию.
// @return корректный limit в диапазоне 1..100.
func debugLimit(c *gin.Context, fallback int) int {
	value, err := strconv.Atoi(c.Query("limit"))
	if err != nil || value <= 0 || value > 100 {
		return fallback
	}

	return value
}
