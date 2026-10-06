package httptransport

import (
	"time"

	"github.com/gin-gonic/gin"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

func RegisterPersonalAssetRoutes(router gin.IRouter, h *personalapp.AssetHandler, auth, accountOnly gin.HandlerFunc, limiter middleware.Limiter) {
	rules := []middleware.Rule{}
	for _, rule := range []struct {
		method, path, scope string
		limit               int
	}{
		{"PUT", "/conversations/:id/avatar", "group_avatar_put", 10},
		{"DELETE", "/conversations/:id/avatar", "group_avatar_delete", 30},
		{"GET", "/conversations/:id/avatar/content", "group_avatar_content", 120},
		{"GET", "/conversations/:id/attachments/:attachmentId/content", "group_attachment_content", 60},
	} {
		rules = append(rules, middleware.Rule{Method: rule.method, Path: APIV1Prefix + rule.path, Scope: rule.scope, Limit: rule.limit, Window: time.Minute, Key: middleware.UserID})
	}
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	g := router.Group(APIV1Prefix, private, auth, accountOnly, middleware.RateLimit(limiter, middleware.RateLimitConfig{Enabled: limiter != nil, Rules: rules}))
	g.PUT("/conversations/:id/avatar", h.PutAvatar)
	g.DELETE("/conversations/:id/avatar", h.DeleteAvatar)
	g.GET("/conversations/:id/avatar/content", h.AvatarContent)
	g.GET("/conversations/:id/attachments/:attachmentId/content", h.AttachmentContent)
}
