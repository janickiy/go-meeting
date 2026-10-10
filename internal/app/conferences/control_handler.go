package conferencesapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// ControlService задаёт контракт зависимого компонента ControlService в жизненном цикле конференций и правах участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Moderate: операция Moderate с контрактом, описанным у метода.
//   - UpdateMediaState: операция обновление медиа состояние с контрактом, описанным у метода.
type ControlService interface {
	// Moderate применяет действие модерации с проверкой роли инициатора и ограничений целевого участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 4 (string): идентификатор членства участника внутри конференции.
	//   - аргумент 5 (conferences.ModerationRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (conferences.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Moderate(context.Context, string, string, string, conferences.ModerationRequest) (conferences.ParticipantView, error)
	// UpdateMediaState сохраняет заявленное состояние источников медиа участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 4 (conferences.MediaState): значение state типа conferences.MediaState, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (conferences.ParticipantView): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	UpdateMediaState(context.Context, string, string, conferences.MediaState) (conferences.ParticipantView, error)
}

// ControlHandler обрабатывает HTTP-действия модерации и изменения состояния медиа участника.
// Состав:
//   - service: значение service типа ControlService, используемое согласно назначению этой операции.
type ControlHandler struct{ service ControlService }

// NewControlHandler создаёт и связывает зависимости компонента ControlHandler, используемого в жизненном цикле конференций и правах участников.
//
// @args
//   - service (ControlService): значение service типа ControlService, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*ControlHandler): созданный компонент с переданными зависимостями.
func NewControlHandler(service ControlService) *ControlHandler {
	return &ControlHandler{service: service}
}

// Moderate применяет действие модерации с проверкой роли инициатора и ограничений целевого участника.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *ControlHandler) Moderate(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	participant, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participant == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request conferences.ModerationRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Moderate(c.Request.Context(), httpmiddleware.UserID(c), id, participant.String(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Media принимает новое состояние микрофона, камеры и экрана текущего участника.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *ControlHandler) Media(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.MediaState
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.UpdateMediaState(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}
