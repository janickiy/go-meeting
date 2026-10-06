package httptransport

import (
	"github.com/gin-gonic/gin"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"time"
)

func RegisterPersonalRoutes(router gin.IRouter, h *personalapp.Handler, chat *chatapp.Handler, auth gin.HandlerFunc, limiter httpmiddleware.Limiter) {
	rules := []httpmiddleware.Rule{}
	for _, v := range []struct {
		Method, Path, Scope string
		Limit               int
	}{{"GET", "/users", "personal_search", 30}, {"GET", "/conversations", "personal_list", 120}, {"POST", "/conversations/direct", "personal_create", 30}, {"GET", "/conversations/:id", "personal_detail", 120}, {"POST", "/conversations/:id/read", "personal_read", 30}} {
		rules = append(rules, httpmiddleware.Rule{Method: v.Method, Path: APIV1Prefix + v.Path, Scope: v.Scope, Limit: v.Limit, Window: time.Minute, Key: httpmiddleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, h.AccountOnly, httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.GET("/users", h.Search)
	g.GET("/conversations", h.List)
	g.POST("/conversations/direct", h.Create)
	g.GET("/conversations/:id", h.Get)
	g.POST("/conversations/:id/read", chat.MarkRead)
	// AccountOnly is also applied to all shared message/file routes.
	registerMessageRoutes(router, chat, auth, limiter, "conversations", h.AccountOnly)
}
