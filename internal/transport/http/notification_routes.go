package httptransport

import (
	"github.com/gin-gonic/gin"
	notificationsapp "github.com/janickiy/go-recorder/internal/app/notifications"
)

func RegisterNotificationRoutes(router gin.IRouter, handler *notificationsapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/notifications", auth)
	group.GET("", handler.List)
	group.GET("/events", handler.Events)
	group.POST("/:notificationId/read", handler.Read)
}
