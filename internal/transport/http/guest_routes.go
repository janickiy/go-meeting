package httptransport

import (
	"github.com/gin-gonic/gin"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"time"
)

func RegisterGuestRoutes(router gin.IRouter, handler *conferencesapp.GuestHandler, limiter httpmiddleware.Limiter) {
	path := APIV1Prefix + "/conference-invites/:code/guest"
	limit := httpmiddleware.RateLimit(limiter, httpmiddleware.RateLimitConfig{Enabled: limiter != nil, Rules: []httpmiddleware.Rule{
		{Method: "POST", Path: path, Scope: "guest_join_ip", Limit: 10, Window: time.Minute, Key: httpmiddleware.ClientIPKey},
	}})
	router.POST(path, limit, handler.Join)
}
