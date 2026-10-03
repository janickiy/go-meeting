package httptransport

import (
	"github.com/gin-gonic/gin"
	contentapp "github.com/janickiy/go-recorder/internal/app/content"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"time"
)

// RegisterContentRoutes защищает все маршруты содержимого и поиска общей авторизацией JWT.
// @args router — HTTP router; handler — content сценарии; authentication — Bearer verifier.
func RegisterContentRoutes(router gin.IRouter, handler *contentapp.Handler, authentication gin.HandlerFunc, limiters ...middleware.Limiter) {
	var limiter middleware.Limiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	searchLimit := middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: []middleware.Rule{{Method: "GET", Path: APIV1Prefix + "/search", Scope: "content_search", Limit: 30, Window: time.Minute, Key: func(c *gin.Context) string { return middleware.UserID(c) }}}})
	// Приватный текст не сохраняется браузером/proxy после logout или revoke.
	// @args c — текущий HTTP response/request context.
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("Pragma", "no-cache")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	routes := router.Group(APIV1Prefix, private, authentication)
	base := "/conferences/:id/recordings/:recordingId"
	routes.GET(base+"/transcript", handler.Transcript)
	routes.GET(base+"/transcript/segments", handler.Segments)
	routes.POST(base+"/transcript/retry", handler.RetryTranscript)
	routes.GET(base+"/summary", handler.Summary)
	routes.POST(base+"/summary/regenerate", handler.RegenerateSummary)
	routes.GET("/search", searchLimit, handler.Search)
}
