package httptransport

import (
	"github.com/gin-gonic/gin"
	app "github.com/janickiy/go-recorder/internal/app/captions"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"time"
)

// RegisterCaptionRoutes защищает чтение и ограничивает частоту переключения платного распознавания.
// @args router — HTTP; handler — сценарии; auth — JWT; limiter — общая Redis квота.
func RegisterCaptionRoutes(router gin.IRouter, handler *app.Handler, auth gin.HandlerFunc, limiter middleware.Limiter) {
	base := APIV1Prefix + "/conferences/:id/captions"
	limited := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: []middleware.Rule{{Method: "PUT", Path: base, Scope: "live_caption_control", Limit: 6, Window: time.Minute, Key: func(c *gin.Context) string { return middleware.UserID(c) }}}})
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.GET(base, private, auth, handler.Read)
	router.PUT(base, private, auth, limited, handler.Set)
	router.GET(base+"/segments", private, auth, handler.Finals)
	router.GET(APIV1Prefix+"/conferences/:id/analytics", private, auth, handler.ReadAnalytics)
	router.POST(APIV1Prefix+"/conferences/:id/recordings/:recordingId/search/reindex", private, auth, handler.Reindex)
}
