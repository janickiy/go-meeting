package httptransport

import (
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
)

// RegisterPlatformRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @args
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
//   - auth (*authapp.Handler): значение auth типа *authapp.Handler, используемое согласно назначению этой операции.
//   - conference (*conferencesapp.Handler): конференция либо её идентификатор, ограничивающий область операции.
//   - authentication (gin.HandlerFunc): значение authentication типа gin.HandlerFunc, используемое согласно назначению этой операции.
func RegisterPlatformRoutes(router gin.IRouter, auth *authapp.Handler, conference *conferencesapp.Handler, authentication gin.HandlerFunc) {
	public := router.Group(APIV1Prefix + "/auth")
	public.POST("/register", auth.Register)
	public.POST("/login", auth.Login)
	protected := public.Group("", authentication)
	protected.GET("/me", auth.Me)
	protected.PATCH("/me", auth.UpdateProfile)
	protected.POST("/logout", auth.Logout)

	conferenceRoutes := router.Group(APIV1Prefix+"/conferences", authentication)
	conferenceRoutes.POST("", conference.Create)
	conferenceRoutes.GET("", conference.List)
	conferenceRoutes.GET("/:id", conference.Read)
	conferenceRoutes.POST("/:id/start", conference.Transition(conferences.Active))
	conferenceRoutes.POST("/:id/finish", conference.Transition(conferences.Finished))
	conferenceRoutes.POST("/:id/cancel", conference.Transition(conferences.Cancelled))
	conferenceRoutes.POST("/:id/join", conference.Join)
	conferenceRoutes.POST("/:id/leave", conference.Leave)
	conferenceRoutes.GET("/:id/participants", conference.Participants)
	conferenceRoutes.GET("/:id/participants/me", conference.Self)
	conferenceRoutes.POST("/:id/participants/:participantId/admission", conference.Admission)
	conferenceRoutes.PUT("/:id/schedule", conference.Schedule)
	conferenceRoutes.GET("/:id/history", conference.History)
	router.GET(APIV1Prefix+"/me/conferences", authentication, conference.Timeline)

	// Отдельный префикс предотвращает конфликт шаблонов :id/:inviteCode и открывает только ограниченное представление.
	invites := router.Group(APIV1Prefix+"/conference-invites", authentication)
	invites.GET("/:code", conference.LookupInvite)
	invites.POST("/:code/join", conference.JoinInvite)
}

// RegisterControlRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @args
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
//   - handler (*conferencesapp.ControlHandler): обработчик вызываемой команды или маршрута.
//   - authentication (gin.HandlerFunc): значение authentication типа gin.HandlerFunc, используемое согласно назначению этой операции.
func RegisterControlRoutes(router gin.IRouter, handler *conferencesapp.ControlHandler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.PUT("/:id/participants/me/media", handler.Media)
	routes.POST("/:id/participants/:participantId/moderation", handler.Moderate)
}
