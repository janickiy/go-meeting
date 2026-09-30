package authapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/users"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

type Service interface {
	Register(context.Context, users.RegisterRequest) (users.View, error)
	Login(context.Context, users.LoginRequest) (users.LoginResponse, error)
	Me(context.Context, string) (users.View, error)
}

type Handler struct{ service Service }

func NewHandler(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) Register(c *gin.Context) {
	var request users.RegisterRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	user, err := h.service.Register(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "user": user})
}

func (h *Handler) Login(c *gin.Context) {
	var request users.LoginRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	response, err := h.service.Login(c.Request.Context(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) Me(c *gin.Context) {
	user, err := h.service.Me(c.Request.Context(), httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

func (h *Handler) Logout(c *gin.Context) {
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Discard the access token on the client; the token remains valid until expiration"})
}
