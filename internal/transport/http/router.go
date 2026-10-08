package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/operations"
)

// APIV1Prefix задает основной префикс версионированного REST API.
const APIV1Prefix = "/api/v1"

// NewRouter создаёт маршрутизатор API на основе Gin.
// @args
// - middleware: дополнительные промежуточные обработчики Gin.
// @return готовый *gin.Engine.
func NewRouter(middleware ...gin.HandlerFunc) *gin.Engine {
	router := gin.New()
	router.Use( /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@args
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

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			start := time.Now()
			c.Next()
			// Строки запросов не журналируются: одноразовые билеты WS также являются учётными данными.
			slog.Info("http request", "method", c.Request.Method, "route", c.FullPath(), "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds(), "request_id", operations.ID(c.Request.Context()))
		})
	if len(middleware) > 0 {
		router.Use(middleware...)
	}
	router.GET("/health", /* Вложенный обработчик выполняет выделенный шаг обработки в регистрации и обработке HTTP-маршрутов, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	registerRetiredRecordingRoutes(router)

	return router
}
