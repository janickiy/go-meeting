package engagementapp

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	usecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}
type Handler struct {
	service   *usecase.Engagement
	limiter   Limiter
	namespace string
}

func NewHandler(service *usecase.Engagement, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, limiter: limiter, namespace: namespace}
}
func id(c *gin.Context) (string, bool) {
	v, err := uuid.Parse(c.Param("id"))
	if err != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}
func (h *Handler) limit(c *gin.Context, conferenceID, kind string, n int, window time.Duration) bool {
	if h.limiter == nil {
		return true
	}
	keys := []string{h.namespace + ":engagement:" + kind + ":user:" + httpmiddleware.UserID(c), h.namespace + ":engagement:" + kind + ":conference:" + conferenceID}
	for i, key := range keys {
		count := n
		if i == 1 {
			// A nonmember must not spend the victim conference's shared budget.
			if err := h.service.Authorize(c.Request.Context(), conferenceID, httpmiddleware.UserID(c)); err != nil {
				httpresponse.Fail(c, err)
				return false
			}
			count = 30
			window = time.Second
		}
		result, err := h.limiter.Allow(c.Request.Context(), key, count, window)
		if err != nil {
			httpresponse.Fail(c, err)
			return false
		}
		if !result.Allowed {
			c.Header("Retry-After", "10")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many " + kind})
			return false
		}
	}
	return true
}
func (h *Handler) List(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	if h.limiter != nil {
		result, err := h.limiter.Allow(c.Request.Context(), h.namespace+":engagement:hands-read:user:"+httpmiddleware.UserID(c), 120, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !result.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many hand state requests"})
			return
		}
	}
	items, err := h.service.List(c.Request.Context(), conf, httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}
func (h *Handler) Hand(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	participant, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participant == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request struct {
		Raised *bool `json:"raised"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.Raised == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !h.limit(c, conf, "hands", 10, time.Minute) {
		return
	}
	item, err := h.service.Hand(c.Request.Context(), conf, httpmiddleware.UserID(c), participant.String(), *request.Raised)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"item": item})
}
func (h *Handler) Reaction(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Emoji string `json:"emoji"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if !h.limit(c, conf, "reactions", 5, 10*time.Second) {
		return
	}
	if err := h.service.Reaction(c.Request.Context(), conf, httpmiddleware.UserID(c), request.Emoji); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success"})
}
