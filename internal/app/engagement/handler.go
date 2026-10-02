package engagementapp

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	usecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

// Limiter задаёт контракт зависимого компонента Limiter в поднятых руках и временных реакциях участников; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Allow: операция Allow с контрактом, описанным у метода.
type Limiter interface {
	// Allow проверяет ограничение частоты и возвращает решение, остаток и время сброса.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//   - аргумент 3 (int): предел количества обрабатываемых элементов.
	//   - аргумент 4 (time.Duration): значение window типа time.Duration, используемое согласно назначению этой операции.
	//
	// @return
	//   - результат 1 (ratelimit.Result): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
//   - service: значение service типа *usecase.Engagement, используемое согласно назначению этой операции.
//   - limiter: ограничитель частоты запросов, общий для экземпляров API.
//   - namespace: пространство изолированных ключей и каналов Redis.
type Handler struct {
	service   *usecase.Engagement
	limiter   Limiter
	namespace string
}

// NewHandler создаёт и связывает зависимости компонента Handler, используемого в поднятых руках и временных реакциях участников.
//
// @args
//   - service (*usecase.Engagement): значение service типа *usecase.Engagement, используемое согласно назначению этой операции.
//   - limiter (Limiter): ограничитель частоты запросов, общий для экземпляров API.
//   - namespace (string): пространство изолированных ключей и каналов Redis.
//
// @return
//   - результат 1 (*Handler): созданный компонент с переданными зависимостями.
func NewHandler(service *usecase.Engagement, limiter Limiter, namespace string) *Handler {
	return &Handler{service: service, limiter: limiter, namespace: namespace}
}

// id проверяет и нормализует идентификатор ресурса из HTTP-маршрута.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
func id(c *gin.Context) (string, bool) {
	v, err := uuid.Parse(c.Param("id"))
	if err != nil || v == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return "", false
	}
	return v.String(), true
}

// limit проверяет отдельные пользовательские и комнатные ограничения частоты действия.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - n (int): значение n типа int, используемое согласно назначению этой операции.
//   - window (time.Duration): значение window типа time.Duration, используемое согласно назначению этой операции.
//
// @return
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (h *Handler) limit(c *gin.Context, conferenceID, kind string, n int, window time.Duration) bool {
	if h.limiter == nil {
		return true
	}
	keys := []string{h.namespace + ":engagement:" + kind + ":user:" + httpmiddleware.UserID(c), h.namespace + ":engagement:" + kind + ":conference:" + conferenceID}
	for i, key := range keys {
		count := n
		if i == 1 {
			// A nonmember must not spend the victim conference's shared budget.
			if err := h.service.Authorize(c.Request.Context(), conferenceID, httpmiddleware.UserID(c)); err != nil {
				httpresponse.Fail(c, err)
				return false
			}
			count = 30
			window = time.Second
		}
		result, err := h.limiter.Allow(c.Request.Context(), key, count, window)
		if err != nil {
			httpresponse.Fail(c, err)
			return false
		}
		if !result.Allowed {
			c.Header("Retry-After", "10")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many " + kind})
			return false
		}
	}
	return true
}

// List возвращает ограниченный список поднятых рук и реакций комнаты с принятыми в данном слое фильтрами.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) List(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	if h.limiter != nil {
		result, err := h.limiter.Allow(c.Request.Context(), h.namespace+":engagement:hands-read:user:"+httpmiddleware.UserID(c), 120, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !result.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many hand state requests"})
			return
		}
	}
	items, err := h.service.List(c.Request.Context(), conf, httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items})
}

// Hand меняет состояние руки с проверкой прав участника или модератора.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Hand(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	participant, err := uuid.Parse(c.Param("participantId"))
	if err != nil || participant == uuid.Nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	var request struct {
		Raised *bool `json:"raised"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if request.Raised == nil {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if !h.limit(c, conf, "hands", 10, time.Minute) {
		return
	}
	item, err := h.service.Hand(c.Request.Context(), conf, httpmiddleware.UserID(c), participant.String(), *request.Raised)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"item": item})
}

// Reaction публикует разрешённую временную реакцию допущенного участника.
//
// @args
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Reaction(c *gin.Context) {
	conf, ok := id(c)
	if !ok {
		return
	}
	var request struct {
		Emoji string `json:"emoji"`
	}
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	if !h.limit(c, conf, "reactions", 5, 10*time.Second) {
		return
	}
	if err := h.service.Reaction(c.Request.Context(), conf, httpmiddleware.UserID(c), request.Emoji); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, gin.H{"status": "success"})
}
