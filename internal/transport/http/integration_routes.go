package httptransport

import (
	"github.com/gin-gonic/gin"
	app "github.com/janickiy/go-recorder/internal/app/integrations"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"time"
)

// RegisterIntegrationRoutes подключает preferences/devices/calendar только с общей Bearer авторизацией.
// @parameters: router — HTTP transport; handler — прикладные обработчики; auth — проверка пользовательского токена;
// limiters — необязательный общий Redis limiter, отключаемый только в изолированных тестах.
func RegisterIntegrationRoutes(router gin.IRouter, handler *app.Handler, auth gin.HandlerFunc, limiters ...middleware.Limiter) {
	var limiter middleware.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	// Middleware исключает caching любых интеграционных ответов, включая ошибки.
	// @parameters: c — HTTP запрос/ответ текущей цепочки.
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	// Ключ использует только ранее проверенную Bearer identity, не пользовательский input.
	// @parameters: c — authenticated запрос.
	// @return: идентификатор владельца rate limit bucket.
	key := func(c *gin.Context) string { return middleware.UserID(c) }
	rules := []middleware.Rule{}
	for _, rule := range []struct {
		method, path, scope string
		limit               int
	}{
		{"GET", "/integrations/calendars/:provider/connect", "calendar_oauth_connect", 5},
		{"POST", "/integrations/calendars/:provider/callback", "calendar_oauth_callback", 10},
		{"POST", "/notifications/devices", "push_device_register", 10},
		{"POST", "/integrations/calendars/mock", "calendar_mock_connect", 5},
		{"PUT", "/notifications/preferences", "notification_preferences", 30},
		{"DELETE", "/integrations/calendars/:provider", "calendar_disconnect", 10},
		{"DELETE", "/notifications/devices/:deviceId", "push_device_revoke", 30},
	} {
		rules = append(rules, middleware.Rule{Method: rule.method, Path: APIV1Prefix + rule.path, Scope: rule.scope, Limit: rule.limit, Window: time.Minute, Key: key})
	}
	limited := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules})
	notifications := router.Group(APIV1Prefix+"/notifications", private, auth, limited)
	notifications.GET("/preferences", handler.Preferences)
	notifications.PUT("/preferences", handler.SavePreferences)
	notifications.GET("/devices", handler.Devices)
	notifications.POST("/devices", handler.RegisterDevice)
	notifications.DELETE("/devices/:deviceId", handler.RevokeDevice)
	integration := router.Group(APIV1Prefix+"/integrations", private, auth, limited)
	integration.GET("/capabilities", handler.Capabilities)
	integration.GET("/calendars", handler.Calendars)
	integration.POST("/calendars/mock", handler.MockConnect)
	integration.GET("/calendars/:provider/connect", handler.Connect)
	integration.POST("/calendars/:provider/callback", handler.Callback)
	integration.DELETE("/calendars/:provider", handler.Disconnect)
	// Gin использует одно имя wildcard для provider connect/callback и UUID удаления; handler валидирует UUID отдельно.
	router.GET(APIV1Prefix+"/conferences/:id/calendar", private, auth, handler.CalendarMappings)
}
