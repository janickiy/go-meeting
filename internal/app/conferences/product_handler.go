package conferencesapp

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// Self возвращает собственное членство пользователя, включая состояние ожидания и решение о допуске.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Self(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	item, err := h.service.Self(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Admission обрабатывает решение о допуске или отказе с проверкой полномочий организатора.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Admission(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	participantID, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participantID == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request conferences.AdmissionRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Admission(c.Request.Context(), httpmiddleware.UserID(c), id, participantID.String(), request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Schedule обновляет расписание запланированной встречи с проверкой полномочий владельца.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Schedule(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	var request conferences.ScheduleRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	item, err := h.service.Schedule(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// History собирает сведения завершённой встречи, историю участников и сводку записей с проверкой доступа.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) History(c *gin.Context) {
	id, ok := conferenceID(c)
	if !ok {
		return
	}
	item, err := h.service.History(c.Request.Context(), httpmiddleware.UserID(c), id)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": item})
}

// Timeline возвращает страницу встреч текущего пользователя с фильтрами будущих, активных и прошедших встреч.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Timeline(c *gin.Context) {
	query := conferences.TimelineQuery{View: c.DefaultQuery("view", "upcoming"), Scope: c.DefaultQuery("scope", "all"), Status: conferences.Status(c.Query("status")), Cursor: c.Query("cursor"), Limit: 20}
	if raw, exists := c.GetQuery("limit"); exists {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		query.Limit = limit
	}
	for name, target := range map[string]**time.Time{"from": &query.From, "to": &query.To} {
		if raw, exists := c.GetQuery(name); exists {
			at, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				httpresponse.Fail(c, apperrors.New(apperrors.ErrInvalidInput, name+" must be RFC3339 with timezone"))
				return
			}
			utc := at.UTC()
			*target = &utc
		}
	}
	if err := query.Validate(); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	page, err := h.service.Timeline(c.Request.Context(), httpmiddleware.UserID(c), query)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}
