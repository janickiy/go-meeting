package conferencesapp

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	usecase "github.com/janickiy/meet-space/internal/usecase/conferences"
)

type GuestHandler struct {
	Service *usecase.GuestService
	Tokens  httpmiddleware.SessionVerifier
}

func (h *GuestHandler) Join(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request struct {
		DisplayName string `json:"displayName"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	var resumeUserID string
	if header := c.GetHeader("Authorization"); header != "" {
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		id, scope, _, err := h.Tokens.VerifySession(parts[1])
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if scope == "" {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return
		}
		resumeUserID = id
	}
	result, err := h.Service.Join(c.Request.Context(), c.Param("code"), request.DisplayName, resumeUserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
