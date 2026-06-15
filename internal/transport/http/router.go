package httptransport

import (
	"net/http"

	recordsapp "git.svc-dev.net/board/go-recorder/internal/app/records"
	"github.com/gin-gonic/gin"
)

// APIV1Prefix задает основной префикс версионированного REST API.
const APIV1Prefix = "/api/v1"

// NewRouter создает Gin router API.
// Параметры:
// - recordsHandler: handler записей.
// - debug: включить local debug pages.
// - completedRecords: источник завершенных записей для debug pages.
// - middleware: дополнительные Gin middleware.
// Возвращает: готовый *gin.Engine.
func NewRouter(recordsHandler *recordsapp.Handler, debug bool, completedRecords CompletedRecordsLister, middleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.Default()
	if len(middleware) > 0 {
		router.Use(middleware...)
	}
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	RegisterRecordRoutes(router, recordsHandler)
	if debug {
		RegisterDebugRoutes(router, completedRecords)
	}

	return router
}

// RegisterRecordRoutes регистрирует HTTP routes записей.
// Параметры:
// - router: Gin router group.
// - recordsHandler: handler записей.
// Возвращает: ничего.
func RegisterRecordRoutes(router gin.IRouter, recordsHandler *recordsapp.Handler) {
	registerRecordRoutes(router.Group(APIV1Prefix), recordsHandler)
}

// registerRecordRoutes регистрирует endpoints records внутри переданного API-префикса.
// Параметры:
// - router: Gin group, например /api/v1.
// - recordsHandler: handler записей.
// Возвращает: ничего.
func registerRecordRoutes(router gin.IRouter, recordsHandler *recordsapp.Handler) {
	router.POST("/records/start", recordsHandler.Start)
	router.POST("/records/end", recordsHandler.End)
	router.POST("/records/:id/webrtc/offer", recordsHandler.Offer)
	router.GET("/records", recordsHandler.List)
	router.GET("/records/count-by-conference", recordsHandler.CountByConference)
	router.GET("/records/:id", recordsHandler.Read)
}
