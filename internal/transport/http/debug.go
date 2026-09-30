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

// CompletedRecordsLister читает завершенные записи из хранилища артефактов.
type CompletedRecordsLister interface {
	// ListCompletedRecords возвращает записи, у которых в MinIO есть preview.jpg и final.mp4.
	// Параметры:
	// - ctx: контекст HTTP-запроса.
	// - limit: максимальное количество записей.
	// Возвращает: список записей или ошибку хранилища.
	ListCompletedRecords(ctx context.Context, limit int) ([]s3storage.CompletedRecord, error)
}

// RegisterDebugRoutes регистрирует local-only debug страницы.
// Параметры:
// - router: Gin router.
// - completedRecords: источник завершенных записей из MinIO.
// Возвращает: ничего.
func RegisterDebugRoutes(router gin.IRouter, completedRecords CompletedRecordsLister) {
	router.GET("/debug/webrtc-smoke", func(c *gin.Context) {
		noStore(c)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(webrtcSmokeHTML))
	})

	if completedRecords == nil {
		return
	}
	router.GET("/debug/records/completed", func(c *gin.Context) {
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

// noStore отключает browser cache для debug endpoint-ов.
// Параметры:
// - c: Gin context.
// Возвращает: ничего.
func noStore(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// debugLimit читает query-параметр limit для debug endpoint-ов.
// Параметры:
// - c: Gin context.
// - fallback: значение по умолчанию.
// Возвращает: корректный limit в диапазоне 1..100.
func debugLimit(c *gin.Context, fallback int) int {
	value, err := strconv.Atoi(c.Query("limit"))
	if err != nil || value <= 0 || value > 100 {
		return fallback
	}

	return value
}
