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

// Service задаёт API сценариев без внешнего распознавания и ИИ внутри HTTP-запроса.
type Service interface {
	Transcript(context.Context, string, string, string) (domain.TranscriptState, error)
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	Summary(context.Context, string, string, string) (domain.SummaryState, error)
	RetryTranscript(context.Context, string, string, string) (domain.TranscriptState, error)
	RegenerateSummary(context.Context, string, string, string) (domain.SummaryState, error)
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}

// Handler связывает авторизованные HTTP-запросы с прикладными сценариями содержимого.
type Handler struct{ service Service }

// NewHandler создаёт адаптер маршрутов содержимого.
// @args service — зависимый сценарий с repository authorization.
// @return обработчик без SDK провайдера и состояния сетевых соединений.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// identifiers проверяет связанную пару UUID из пути до чтения или запуска.
// @args c — запрос Gin с подтверждённой идентичностью пользователя.
// @return UUID конференции и записи и признак успеха; при ошибке пишет безопасный HTTP-ответ.
func identifiers(c *gin.Context) (string, string, bool) {
	cid, e := uuid.Parse(c.Param("id"))
	rid, e2 := uuid.Parse(c.Param("recordingId"))
	if e != nil || e2 != nil || cid == uuid.Nil || rid == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", "", false
	}
	return cid.String(), rid.String(), true
}

// pageValue разбирает десятичную пагинацию, не скрывая неверные данные значением по умолчанию.
// @args c — запрос; key — ключ параметра запроса; fallback — значение при отсутствии.
// @return число и признак успеха; некорректные входные данные создают HTTP 400.
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

// Transcript возвращает необязательные метаданные, возможности и достоверную метку режима провайдера.
// @args c — авторизованный Gin request.
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

// Segments выдаёт ограниченную страницу для переходов по временным отметкам.
// @args c — request с limit/offset.
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

// Summary возвращает необязательный результат ИИ текущего поколения без вызова провайдера.
// @args c — авторизованный request.
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
// @args c — авторизованный request; права/лимит проверяет repository.
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

// RegenerateSummary ставит ограниченное поколение, не блокируя HTTP длительным вызовом ИИ.
// @args c — авторизованный request.
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

// Search проверяет фильтры дат RFC3339 и возвращает страницу полнотекстового поиска с проверкой прав.
// @args c — авторизованный запрос с q/source/conferenceId/from/to/limit/offset.
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
	q := domain.SearchQuery{Query: c.Query("q"), Source: c.Query("source"), ConferenceID: c.Query("conferenceId"), Mode: c.Query("mode"), ParticipantID: c.Query("participantId"), Membership: c.Query("membership"), Limit: limit, Offset: offset}
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
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "total": page.Total, "limit": page.Limit, "offset": page.Offset, "effectiveMode": page.EffectiveMode, "fallbackReason": page.FallbackReason})
}
