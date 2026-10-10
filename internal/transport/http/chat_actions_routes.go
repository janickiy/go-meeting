package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

func RegisterChatActionRoutes(router gin.IRouter, handler *chatapp.ActionHandler, auth gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	base := APIV1Prefix + "/conferences/:id/chat"
	key := func(c *gin.Context) string {
		id, err := uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			return ""
		}
		return httpmiddleware.UserID(c) + ":" + id.String()
	}
	rules := []httpmiddleware.Rule{}
	for _, item := range []struct {
		method, path string
		limit        int
	}{
		{"GET", "/info", 120}, {"PATCH", "/info", 20}, {"GET", "/members", 120},
		{"GET", "/preferences", 120}, {"PUT", "/preferences", 30},
		{"DELETE", "/membership", 10}, {"GET", "/messages/search", 60},
		{"GET", "/messages/:messageId/context", 60}, {"GET", "/materials", 60},
		{"GET", "/pins", 60}, {"PUT", "/pins/:messageId", 60}, {"DELETE", "/pins/:messageId", 60},
	} {
		rules = append(rules, httpmiddleware.Rule{Method: item.method, Path: base + item.path, Scope: "conference_chat_actions", Limit: item.limit, Window: time.Minute, Key: key})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	routes := router.Group(base, private, auth, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	routes.GET("/info", handler.Info)
	routes.GET("/members", handler.Members)
	routes.PATCH("/info", handler.UpdateInfo)
	routes.GET("/preferences", handler.Preferences)
	routes.PUT("/preferences", handler.SetPreferences)
	routes.DELETE("/membership", handler.Leave)
	routes.GET("/messages/search", handler.Search)
	routes.GET("/messages/:messageId/context", handler.Context)
	routes.GET("/materials", handler.Materials)
	routes.GET("/pins", handler.Pins)
	routes.PUT("/pins/:messageId", handler.SetPin(true))
	routes.DELETE("/pins/:messageId", handler.SetPin(false))
}
