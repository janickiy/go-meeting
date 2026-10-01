package httptransport

import (
	"github.com/janickiy/go-recorder/internal/operations"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	recordsapp "github.com/janickiy/go-recorder/internal/app/records"
)

// APIV1Prefix задает основной префикс версионированного REST API.
const APIV1Prefix = "/api/v1"

// NewRouter создает Gin router API.
// @parameters:
// - recordsHandler: handler записей.
// - debug: включить local debug pages.
// - completedRecords: источник завершенных записей для debug pages.
// - middleware: дополнительные Gin middleware.
// @return готовый *gin.Engine.
func NewRouter(recordsHandler *recordsapp.Handler, debug bool, completedRecords CompletedRecordsLister, middleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use( /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@parameters:
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			defer /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

			 */func() {
				if recover() != nil {
					slog.Error("http request panic", "method", c.Request.Method, "route", c.FullPath())
					c.AbortWithStatusJSON(500, gin.H{"status": "failed", "message": "internal server error"})
				}
			}()
			c.Next()
		}, /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@parameters:
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			start := time.Now()
			c.Next()
			// Never log query strings: one-time WS tickets are credentials too.
			slog.Info("http request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(), "request_id", operations.ID(c.Request.Context()))
		})
	if len(middleware) > 0 {
		router.Use(middleware...)
	}
	router.GET("/health", /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@parameters:
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	RegisterRecordRoutes(router, recordsHandler)
	if debug {
		RegisterDebugRoutes(router, completedRecords)
	}

	return router
}

// RegisterRecordRoutes регистрирует HTTP routes записей.
// @parameters:
// - router: Gin router group.
// - recordsHandler: handler записей.
// @return ничего.
func RegisterRecordRoutes(router gin.IRouter, recordsHandler *recordsapp.Handler) {
	registerRecordRoutes(router.Group(APIV1Prefix), recordsHandler)
}

// registerRecordRoutes регистрирует endpoints records внутри переданного API-префикса.
// @parameters:
// - router: Gin group, например /api/v1.
// - recordsHandler: handler записей.
// @return ничего.
func registerRecordRoutes(router gin.IRouter, recordsHandler *recordsapp.Handler) {
	router.POST("/records/start", recordsHandler.Start)
	router.POST("/records/end", recordsHandler.End)
	router.POST("/records/:id/webrtc/offer", recordsHandler.Offer)
	router.GET("/records", recordsHandler.List)
	router.GET("/records/count-by-conference", recordsHandler.CountByConference)
	router.GET("/records/:id", recordsHandler.Read)
}
