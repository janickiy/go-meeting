package integrationsapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	u "github.com/janickiy/go-recorder/internal/usecase/integrations"
)

// Handler предоставляет только пользовательские проекции интеграций; credentials никогда не сериализуются в API.
type Handler struct{ service *u.Service }

// NewHandler связывает защищённые HTTP команды с прикладным сервисом.
// @parameters: service — настроенный серверный сервис интеграций.
// @return: handler для authenticated маршрутов.
func NewHandler(service *u.Service) *Handler { return &Handler{service: service} }

// Capabilities сообщает доступные режимы провайдеров без endpoints и credentials.
// @parameters: c — authenticated HTTP запрос.
func (h *Handler) Capabilities(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, h.service.Capabilities())
}

// Preferences читает только настройки текущего пользователя.
// @parameters: c — authenticated HTTP запрос.
func (h *Handler) Preferences(c *gin.Context) {
	value, err := h.service.Preferences(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, value)
}

// SavePreferences заменяет boolean настройки только для identity из токена.
// @parameters: c — authenticated JSON запрос.
func (h *Handler) SavePreferences(c *gin.Context) {
	var request d.Preferences
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	if err := h.service.SavePreferences(c.Request.Context(), middleware.UserID(c), request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, request)
}

// Devices перечисляет собственные устройства без расшифрованного токена.
// @parameters: c — authenticated HTTP запрос.
func (h *Handler) Devices(c *gin.Context) {
	items, err := h.service.Devices(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// RegisterDevice принимает platform token от клиентского SDK, но не произвольный provider endpoint.
// @parameters: c — authenticated JSON запрос регистрации.
func (h *Handler) RegisterDevice(c *gin.Context) {
	var request d.DeviceRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.RegisterDevice(c.Request.Context(), middleware.UserID(c), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// RevokeDevice отключает только UUID регистрации текущего пользователя.
// @parameters: c — authenticated HTTP запрос с deviceId.
func (h *Handler) RevokeDevice(c *gin.Context) {
	id, ok := resourceID(c, "deviceId")
	if !ok {
		return
	}
	if err := h.service.RevokeDevice(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Calendars перечисляет собственные подключённые и отозванные календари без OAuth tokens.
// @parameters: c — authenticated HTTP запрос.
func (h *Handler) Calendars(c *gin.Context) {
	items, err := h.service.Connections(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// MockConnect создаёт явно демонстрационное подключение только при серверном разрешении.
// @parameters: c — authenticated HTTP запрос локального/test режима.
func (h *Handler) MockConnect(c *gin.Context) {
	var request struct{}
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.MockConnect(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

// Connect начинает серверный authorization-code поток; PKCE verifier остаётся зашифрованным в DB.
// @parameters: c — authenticated HTTP запрос с provider=generic.
func (h *Handler) Connect(c *gin.Context) {
	authURL, state, err := h.service.OAuthStart(c.Request.Context(), middleware.UserID(c), c.Param("provider"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"authUrl": authURL, "state": state})
}

// oauthCallbackRequest содержит только одноразовые code/state, не access/refresh tokens.
type oauthCallbackRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// Callback меняет одноразовый code после проверки state/user/provider; token never reaches browser.
// @parameters: c — authenticated JSON запрос возврата OAuth.
func (h *Handler) Callback(c *gin.Context) {
	var request oauthCallbackRequest
	if !httpresponse.BindJSON(c, &request, true) {
		return
	}
	item, err := h.service.OAuthCallback(c.Request.Context(), middleware.UserID(c), c.Param("provider"), request.Code, request.State)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"item": item})
}

// Disconnect удаляет локальные credentials и пытается отозвать grant внешнего провайдера.
// @parameters: c — authenticated HTTP запрос с connectionId.
func (h *Handler) Disconnect(c *gin.Context) {
	id, ok := resourceID(c, "provider")
	if !ok {
		return
	}
	if err := h.service.Disconnect(c.Request.Context(), middleware.UserID(c), id); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CalendarMappings показывает внешнее состояние только actor с owner/cohost permissions.
// @parameters: c — authenticated HTTP запрос с conference id.
func (h *Handler) CalendarMappings(c *gin.Context) {
	id, ok := resourceID(c, "id")
	if !ok {
		return
	}
	items, err := h.service.CalendarMappings(c.Request.Context(), middleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// resourceID нормализует path UUID до обращения к постоянному хранилищу.
// @parameters: c — HTTP запрос; name — фиксированный route parameter.
// @return: canonical UUID и признак успеха; ошибка уже отправлена клиенту.
func resourceID(c *gin.Context, name string) (string, bool) {
	value, err := uuid.Parse(c.Param(name))
	if err != nil || value == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return value.String(), true
}
