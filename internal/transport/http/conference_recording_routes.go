package httptransport

import (
	"github.com/gin-gonic/gin"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
)

func RegisterConferenceRecordingRoutes(router gin.IRouter, handler *recordingsapp.Handler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.POST("/:id/recordings", handler.Start)
	routes.GET("/:id/recordings", handler.List)
	routes.GET("/:id/recordings/:recordingId", handler.Read)
	routes.POST("/:id/recordings/:recordingId/stop", handler.Stop)
}
