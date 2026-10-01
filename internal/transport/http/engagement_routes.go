package httptransport

import (
	"github.com/gin-gonic/gin"
	engagementapp "github.com/janickiy/go-recorder/internal/app/engagement"
)

func RegisterEngagementRoutes(router gin.IRouter, handler *engagementapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/conferences/:id", auth, func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	})
	group.GET("/hands", handler.List)
	group.PUT("/participants/:participantId/hand", handler.Hand)
	group.POST("/reactions", handler.Reaction)
}
