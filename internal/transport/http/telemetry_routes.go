package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/telemetry"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// RegisterClientErrorRoutes ограничивает приём ошибок браузера на уровне IP и всего API.
// @args router — маршрутизатор; handler — проверяющий обработчик; limiter — общий Redis-ограничитель.
func RegisterClientErrorRoutes(router gin.IRouter, handler *telemetry.Handler, limiter middleware.Limiter) {
	path := APIV1Prefix + "/client-errors"
	router.POST(path, middleware.RateLimit(limiter, middleware.RateLimitConfig{
		Enabled: true,
		Rules: []middleware.Rule{
			{Method: "POST", Path: path, Scope: "client-errors-ip", Limit: 30, Window: time.Minute, Key: middleware.ClientIPKey},
			{Method: "POST", Path: path, Scope: "client-errors-global", Limit: 300, Window: time.Minute, Key: func(*gin.Context) string { return "all" }},
		},
	}), handler.Receive)
}
