package httptransport

import (
	"github.com/gin-gonic/gin"
	platformapp "github.com/janickiy/go-recorder/internal/app/platform"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// RegisterPlatformStatusRoutes exposes an authenticated capability view and admin-only aggregates.
func RegisterPlatformStatusRoutes(router gin.IRouter, handler *platformapp.Handler, auth gin.HandlerFunc, checker middleware.AdminChecker) {
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.GET(APIV1Prefix+"/capabilities", private, auth, handler.Capabilities)
	admin := router.Group(APIV1Prefix+"/admin", private, auth, middleware.RequireAdmin(checker))
	admin.GET("/summary", handler.Summary)
}
