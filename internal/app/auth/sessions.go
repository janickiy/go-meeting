package authapp

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/users"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

const SessionCookieName = "meetrix_session"
const sessionCookiePath = "/api/v1/auth"
const sessionCookieMaxAge = 400 * 24 * 60 * 60

type persistentService interface {
	Refresh(context.Context, string) (users.LoginResponse, error)
	Bootstrap(context.Context, string, string, string) (users.LoginResponse, error)
	Logout(context.Context, string, string) error
}

// WithSessionCookies configures the public origin rather than trusting forwarded hosts.
func (h *Handler) WithSessionCookies(publicURL string, secure bool) *Handler {
	if parsed, err := url.Parse(publicURL); err == nil && parsed.Host != "" {
		h.publicOrigin = parsed.Scheme + "://" + parsed.Host
	}
	h.secureCookie = secure
	h.persistentSessions = true
	return h
}

func (h *Handler) PersistentSessionsEnabled() bool { return h.persistentSessions }

func (h *Handler) sameOrigin(c *gin.Context) bool {
	if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return false
	}
	origin := c.GetHeader("Origin")
	if origin == "" {
		// Non-browser CLI clients do not send Origin or Fetch Metadata.
		if c.GetHeader("Sec-Fetch-Site") != "" {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	} else {
		expected := h.publicOrigin
		if expected == "" {
			scheme := "http"
			if h.secureCookie || c.Request.TLS != nil {
				scheme = "https"
			}
			expected = scheme + "://" + c.Request.Host
		}
		if origin != expected {
			httpresponse.Fail(c, apperrors.ErrForbidden)
			return false
		}
	}
	if c.Request.ContentLength != 0 {
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || mediaType != "application/json" {
			c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"status": "failed", "message": "application/json is required"})
			return false
		}
	}
	return true
}

func (h *Handler) sessionCookie(c *gin.Context) string {
	raw, _ := c.Cookie(SessionCookieName)
	return raw
}

func (h *Handler) setSessionCookie(c *gin.Context, raw string) {
	cookie := &http.Cookie{Name: SessionCookieName, Value: raw, Path: sessionCookiePath,
		MaxAge: sessionCookieMaxAge, Expires: time.Now().UTC().Add(time.Duration(sessionCookieMaxAge) * time.Second),
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode}
	if raw == "" {
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0).UTC()
	}
	http.SetCookie(c.Writer, cookie)
}

func bearerToken(c *gin.Context) string {
	fields := strings.Fields(c.GetHeader("Authorization"))
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return fields[1]
	}
	return ""
}

// Refresh accepts a browser session even when its previous JWT has expired.
func (h *Handler) Refresh(c *gin.Context) { h.sessionOperation(c, false) }

// Bootstrap migrates existing signed-in accounts without requiring a new login.
func (h *Handler) Bootstrap(c *gin.Context) { h.sessionOperation(c, true) }

func (h *Handler) sessionOperation(c *gin.Context, bootstrap bool) {
	if !h.sameOrigin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	if !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	sessions, ok := h.service.(persistentService)
	if !ok || !h.persistentSessions {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	var response users.LoginResponse
	var err error
	if bootstrap {
		response, err = sessions.Bootstrap(c.Request.Context(), httpmiddleware.UserID(c), h.sessionCookie(c), bearerToken(c))
	} else {
		response, err = sessions.Refresh(c.Request.Context(), h.sessionCookie(c))
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	h.setSessionCookie(c, response.SessionToken)
	c.JSON(http.StatusOK, response)
}
