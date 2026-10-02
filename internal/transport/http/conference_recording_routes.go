package httptransport

import (
	"github.com/gin-gonic/gin"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
)

// RegisterConferenceRecordingRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @args
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
//   - handler (*recordingsapp.Handler): обработчик вызываемой команды или маршрута.
//   - authentication (gin.HandlerFunc): значение authentication типа gin.HandlerFunc, используемое согласно назначению этой операции.
func RegisterConferenceRecordingRoutes(router gin.IRouter, handler *recordingsapp.Handler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.POST("/:id/recordings", handler.Start)
	routes.GET("/:id/recordings", handler.List)
	routes.GET("/:id/recordings/:recordingId", handler.Read)
	routes.POST("/:id/recordings/:recordingId/stop", handler.Stop)
}
