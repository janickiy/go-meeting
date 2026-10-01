package notificationsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	usecase "github.com/janickiy/go-recorder/internal/usecase/notifications"
	goredis "github.com/redis/go-redis/v9"
)

type Subscriber interface {
	Subscribe(context.Context, string) (*goredis.PubSub, error)
}
type Verifier interface {
	VerifyWithExpiry(string) (string, time.Time, error)
}
type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}
type Handler struct {
	service   *usecase.Service
	bus       Subscriber
	tokens    Verifier
	limiter   Limiter
	streams   atomic.Int64
	counts    syncCounts
	namespace string
}

func NewHandler(service *usecase.Service, bus Subscriber, tokens Verifier, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, bus: bus, tokens: tokens, limiter: limiter, namespace: namespace, counts: syncCounts{values: map[string]int{}}}
}
func (h *Handler) List(c *gin.Context) {
	limit := 30
	if raw := c.Query("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit = v
	}
	page, err := h.service.List(c.Request.Context(), httpmiddleware.UserID(c), c.Query("cursor"), limit)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, page)
}
func (h *Handler) Read(c *gin.Context) {
	id, err := uuid.Parse(c.Param("notificationId"))
	if err != nil || id == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	item, err := h.service.Read(c.Request.Context(), httpmiddleware.UserID(c), id.String())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"item": item})
}
func (h *Handler) Events(c *gin.Context) {
	// Header authentication only: credentials never appear in URLs/access logs.
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) != 2 || len(c.Request.URL.RawQuery) > 0 {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	userID, expiry, err := h.tokens.VerifyWithExpiry(fields[1])
	if err != nil || userID != httpmiddleware.UserID(c) {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	if h.limiter != nil {
		result, err := h.limiter.Allow(c.Request.Context(), h.namespace+":notifications:sse:"+userID, 12, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !result.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many notification connections"})
			return
		}
	}
	if h.streams.Add(1) > 256 {
		h.streams.Add(-1)
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	defer h.streams.Add(-1)
	if !h.counts.acquire(userID) {
		c.AbortWithStatusJSON(429, gin.H{"error": "too many notification connections"})
		return
	}
	defer h.counts.release(userID)
	ctx, cancel := context.WithDeadline(c.Request.Context(), expiry)
	defer cancel()
	connect, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	sub, err := h.bus.Subscribe(connect, userID)
	cancelConnect()
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	defer sub.Close()
	messages := sub.Channel(goredis.WithChannelSize(32), goredis.WithChannelSendTimeout(time.Millisecond))
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	controller := http.NewResponseController(c.Writer)
	defer controller.SetWriteDeadline(time.Time{})
	write := func(value string) bool {
		// Gin's writer does not expose deadlines on every server. The bounded
		// SSE queue still prevents a slow reader from blocking room signaling.
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && err != http.ErrNotSupported {
			return false
		}
		if _, err := fmt.Fprint(c.Writer, value); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	if !write(": connected\n\n") {
		return
	}
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			if !write(": keepalive\n\n") {
				return
			}
		case message, ok := <-messages:
			if !ok {
				return
			}
			var event domain.Envelope
			if len(message.Payload) > 8192 || json.Unmarshal([]byte(message.Payload), &event) != nil {
				return
			}
			if event.Type != "notification.created" && event.Type != "notification.read" {
				continue
			}
			if !write("data: " + message.Payload + "\n\n") {
				return
			}
		}
	}
}
