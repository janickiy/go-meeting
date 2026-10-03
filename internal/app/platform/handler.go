// Package platformapp exposes safe capabilities and read-only admin operations.
package platformapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	domain "github.com/janickiy/go-recorder/internal/domain/platform"
)

type Service interface {
	Capabilities(context.Context) domain.Capabilities
	Summary(context.Context) (domain.Summary, error)
}

type Handler struct {
	Service      Service
	BuildVersion string
}

// Capabilities returns only effective feature switches and a safe build identifier.
func (h *Handler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "capabilities": h.Service.Capabilities(c.Request.Context()), "buildVersion": h.BuildVersion})
}

// Summary returns aggregate operations data after the persisted admin check.
func (h *Handler) Summary(c *gin.Context) {
	value, err := h.Service.Summary(c.Request.Context())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}
