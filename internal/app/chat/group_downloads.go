package chatapp

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// GroupDownloads replaces bearer-capability URLs for groups only. The returned
// URL requires authentication again when its bytes are requested.
type GroupDownloads interface {
	GroupDownload(context.Context, string, string, string) (bool, string, time.Time, error)
}

func (h *Handler) WithGroupDownloads(provider GroupDownloads) *Handler {
	h.groupDownloads = provider
	return h
}

func (h *Handler) downloadGroup(c *gin.Context, scope, attachment string) bool {
	if h.namespace != "conversations" || h.groupDownloads == nil {
		return false
	}
	handled, url, expires, err := h.groupDownloads.GroupDownload(c.Request.Context(), middleware.UserID(c), scope, attachment)
	if !handled {
		return false
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"status": "success", "url": url, "expiresAt": expires, "authenticated": true})
	return true
}
