package httptransport

import (
	"github.com/gin-gonic/gin"
	engagementapp "github.com/janickiy/go-recorder/internal/app/engagement"
)

// RegisterEngagementRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @args
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
//   - handler (*engagementapp.Handler): обработчик вызываемой команды или маршрута.
//   - auth (gin.HandlerFunc): значение auth типа gin.HandlerFunc, используемое согласно назначению этой операции.
func RegisterEngagementRoutes(router gin.IRouter, handler *engagementapp.Handler, auth gin.HandlerFunc) {
	group := router.Group(APIV1Prefix+"/conferences/:id", auth, /* Вложенный обработчик выполняет выделенный шаг обработки во временных реакциях участников, используя состояние окружающей функции.

		@args
		  - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
		*/func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			c.Header("X-Content-Type-Options", "nosniff")
			c.Next()
		})
	group.POST("/reactions", handler.Reaction)
}
