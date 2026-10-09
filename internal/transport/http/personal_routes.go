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
	}{{"GET", "/users", "personal_search", 30}, {"GET", "/conversations", "personal_list", 120}, {"POST", "/conversations/direct", "personal_create", 30}, {"GET", "/conversations/:id", "personal_detail", 120}, {"POST", "/conversations/:id/read", "personal_read", 30},
		{"POST", "/conversations/group", "group_create", 20}, {"PATCH", "/conversations/:id", "group_metadata", 30}, {"DELETE", "/conversations/:id", "group_delete", 20},
		{"GET", "/conversations/:id/members", "group_members", 120}, {"POST", "/conversations/:id/members", "group_add", 30},
		{"DELETE", "/conversations/:id/members/:userId", "group_remove", 30}, {"PATCH", "/conversations/:id/members/:userId", "group_role", 30},
		{"POST", "/conversations/:id/leave", "group_leave", 30}, {"POST", "/conversations/:id/ownership", "group_ownership", 20},
		{"PATCH", "/conversations/:id/preferences", "direct_preferences", 30}, {"POST", "/conversations/:id/clear-history", "direct_clear", 20}, {"POST", "/conversations/:id/hide", "direct_hide", 20}} {
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
	g.POST("/conversations/group", h.CreateGroup)
	g.GET("/conversations/:id", h.Get)
	g.PATCH("/conversations/:id/preferences", h.SetDirectPreferences)
	g.POST("/conversations/:id/clear-history", h.ClearDirectHistory)
	g.POST("/conversations/:id/hide", h.HideDirectConversation)
	g.PATCH("/conversations/:id", h.UpdateGroup)
	g.DELETE("/conversations/:id", h.DeleteGroup)
	g.GET("/conversations/:id/members", h.GroupMembers)
	g.POST("/conversations/:id/members", h.AddGroupMembers)
	g.DELETE("/conversations/:id/members/:userId", h.RemoveGroupMember)
	g.PATCH("/conversations/:id/members/:userId", h.ChangeGroupRole)
	g.POST("/conversations/:id/leave", h.LeaveGroup)
	g.POST("/conversations/:id/ownership", h.TransferGroupOwnership)
	g.POST("/conversations/:id/read", chat.MarkRead)
	// AccountOnly is also applied to all shared message/file routes.
	registerMessageRoutes(router, chat, auth, limiter, "conversations", h.AccountOnly)
}
