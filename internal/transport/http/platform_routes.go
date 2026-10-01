package httptransport

import (
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
)

func RegisterPlatformRoutes(router gin.IRouter, auth *authapp.Handler, conference *conferencesapp.Handler, authentication gin.HandlerFunc) {
	public := router.Group(APIV1Prefix + "/auth")
	public.POST("/register", auth.Register)
	public.POST("/login", auth.Login)
	protected := public.Group("", authentication)
	protected.GET("/me", auth.Me)
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

	// A separate prefix avoids :id/:inviteCode wildcard conflicts and exposes only a limited view.
	invites := router.Group(APIV1Prefix+"/conference-invites", authentication)
	invites.GET("/:code", conference.LookupInvite)
	invites.POST("/:code/join", conference.JoinInvite)
}

func RegisterControlRoutes(router gin.IRouter, handler *conferencesapp.ControlHandler, authentication gin.HandlerFunc) {
	routes := router.Group(APIV1Prefix+"/conferences", authentication)
	routes.PUT("/:id/participants/me/media", handler.Media)
	routes.POST("/:id/participants/:participantId/moderation", handler.Moderate)
}
