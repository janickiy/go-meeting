package contentapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/operations"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"net/http"
	"strconv"
	"time"
)

// Service задаёт API сценариев, не выполняющих внешний STT/AI в HTTP request.
type Service interface {
	Transcript(context.Context, string, string, string) (domain.TranscriptState, error)
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	Summary(context.Context, string, string, string) (domain.SummaryState, error)
	RetryTranscript(context.Context, string, string, string) (domain.TranscriptState, error)
	RegenerateSummary(context.Context, string, string, string) (domain.SummaryState, error)
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}

// Handler связывает авторизованные HTTP requests с content usecase.
type Handler struct{ service Service }

// NewHandler создаёт content routes adapter.
// @parameters: service — зависимый сценарий с repository authorization.
// @return handler без vendor SDK/network state.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// identifiers проверяет связанную пару path UUID перед чтением/запуском.
// @parameters: c — Gin request с подтверждённой auth identity.
// @return conference/recording UUID, success flag; при ошибке пишет safe HTTP response.
func identifiers(c *gin.Context) (string, string, bool) {
	cid, e := uuid.Parse(c.Param("id"))
	rid, e2 := uuid.Parse(c.Param("recordingId"))
	if e != nil || e2 != nil || cid == uuid.Nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", "", false
	}
	return cid.String(), rid.String(), true
}

// pageValue разбирает decimal пагинацию, не скрывая invalid input за default.
// @parameters: c — request; key — query key; fallback — значение при отсутствии.
// @return число и success; invalid input создаёт HTTP400.
func pageValue(c *gin.Context, key string, fallback int) (int, bool) {
	value, ok := c.GetQuery(key)
	if !ok {
		return fallback, true
	}
	n, e := strconv.Atoi(value)
	if e != nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return 0, false
	}
	return n, true
}

// Transcript возвращает nullable metadata, capabilities и честную provider-mode метку.
// @parameters: c — авторизованный Gin request.
func (h *Handler) Transcript(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	state, e := h.service.Transcript(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRetry": state.CanRetry, "providerMode": state.ProviderMode})
}

// Segments выдаёт bounded страницу для timestamp navigation.
// @parameters: c — request с limit/offset.
func (h *Handler) Segments(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	limit, ok := pageValue(c, "limit", 100)
	if !ok {
		return
	}
	offset, ok := pageValue(c, "offset", 0)
	if !ok {
		return
	}
	page, e := h.service.Segments(c.Request.Context(), httpmiddleware.UserID(c), cid, rid, limit, offset)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}

// Summary возвращает nullable current-generation AI output без вызова provider.
// @parameters: c — авторизованный request.
func (h *Handler) Summary(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok {
		return
	}
	state, e := h.service.Summary(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRegenerate": state.CanRegenerate, "providerMode": state.ProviderMode})
}

// RetryTranscript принимает пустой JSON и только ставит дорогую обработку в queue.
// @parameters: c — авторизованный request; права/лимит проверяет repository.
func (h *Handler) RetryTranscript(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok || !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	state, e := h.service.RetryTranscript(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRetry": false, "providerMode": state.ProviderMode})
}

// RegenerateSummary ставит bounded generation, не блокируя HTTP длительным AI.
// @parameters: c — авторизованный request.
func (h *Handler) RegenerateSummary(c *gin.Context) {
	cid, rid, ok := identifiers(c)
	if !ok || !httpresponse.BindJSON(c, &struct{}{}, true) {
		return
	}
	state, e := h.service.RegenerateSummary(c.Request.Context(), httpmiddleware.UserID(c), cid, rid)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "success", "item": state.Item, "enabled": state.Enabled, "canRegenerate": false, "providerMode": state.ProviderMode})
}

// Search проверяет RFC3339 date filters и возвращает permission-filtered FTS page.
// @parameters: c — авторизованный request с q/source/conferenceId/from/to/limit/offset.
func (h *Handler) Search(c *gin.Context) {
	started := time.Now()
	defer func() { operations.Search(time.Since(started), c.Writer.Status() >= http.StatusBadRequest) }()
	limit, ok := pageValue(c, "limit", 20)
	if !ok {
		return
	}
	offset, ok := pageValue(c, "offset", 0)
	if !ok {
		return
	}
	q := domain.SearchQuery{Query: c.Query("q"), Source: c.Query("source"), ConferenceID: c.Query("conferenceId"), Limit: limit, Offset: offset}
	for key, target := range map[string]**time.Time{"from": &q.From, "to": &q.To} {
		if value, exists := c.GetQuery(key); exists {
			at, e := time.Parse(time.RFC3339, value)
			if e != nil {
				httpresponse.Fail(c, apperrors.ErrInvalidInput)
				return
			}
			utc := at.UTC()
			*target = &utc
		}
	}
	page, e := h.service.Search(c.Request.Context(), httpmiddleware.UserID(c), q)
	if e != nil {
		httpresponse.Fail(c, e)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset})
}
