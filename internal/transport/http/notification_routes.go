package httptransport

import (
	"github.com/gin-gonic/gin"
	notificationsapp "github.com/janickiy/go-recorder/internal/app/notifications"
)

// RegisterNotificationRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @parameters:
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
//   - handler (*notificationsapp.Handler): обработчик вызываемой команды или маршрута.
//   - auth (gin.HandlerFunc): значение auth типа gin.HandlerFunc, используемое согласно назначению этой операции.
func RegisterNotificationRoutes(router gin.IRouter, handler *notificationsapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/notifications", auth)
	group.GET("", handler.List)
	group.GET("/events", handler.Events)
	group.POST("/:notificationId/read", handler.Read)
}
