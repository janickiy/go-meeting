package httptransport

import (
	"github.com/gin-gonic/gin"
	contentapp "github.com/janickiy/go-recorder/internal/app/content"
)

// RegisterContentRoutes защищает все content/search routes общей JWT auth.
// @parameters: router — HTTP router; handler — content сценарии; authentication — Bearer verifier.
func RegisterContentRoutes(router gin.IRouter, handler *contentapp.Handler, authentication gin.HandlerFunc) {
	// Приватный текст не сохраняется браузером/proxy после logout или revoke.
	// @parameters: c — текущий HTTP response/request context.
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
	routes.GET("/search", handler.Search)
}
