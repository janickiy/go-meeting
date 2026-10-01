package websocket

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/operations"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	mediadomain "github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	usecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

// Verifier задаёт контракт зависимого компонента Verifier в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - VerifyWithExpiry: операция Verify с истечение срока с контрактом, описанным у метода.
type Verifier interface {
	// VerifyWithExpiry проверяет подпись и содержимое JWT и возвращает идентификатор пользователя вместе со сроком действия.
	//
	// @parameters:
	//   - аргумент 1 (string): исходные байты JSON, пакета или сериализованного значения.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (time.Time): временная отметка результата или окончания действия разрешения.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	VerifyWithExpiry(string) (string, time.Time, error)
}

// Tickets задаёт контракт зависимого компонента Tickets в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - SaveTicket: операция сохранение билет с контрактом, описанным у метода.
//   - ConsumeTicket: операция Consume билет с контрактом, описанным у метода.
type Tickets interface {
	// SaveTicket сохраняет одноразовый билет подключения с ограниченным сроком жизни.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): одноразовый билет ограниченного подключения.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (domain.Identity): проверенная идентичность пользователя и его членства.
	//   - аргумент 5 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	SaveTicket(context.Context, string, string, domain.Identity, time.Duration) error
	// ConsumeTicket атомарно забирает одноразовый билет, исключая повторное использование.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): одноразовый билет ограниченного подключения.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (domain.Identity): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ConsumeTicket(context.Context, string, string) (domain.Identity, error)
}

// Limiter задаёт контракт зависимого компонента Limiter в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Allow: операция Allow с контрактом, описанным у метода.
type Limiter interface {
	// Allow проверяет ограничение частоты и возвращает решение, остаток и время сброса.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//   - аргумент 3 (int): предел количества обрабатываемых элементов.
	//   - аргумент 4 (time.Duration): значение window типа time.Duration, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (ratelimit.Result): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
//   - hub: координатор присутствия и доставки событий комнаты.
//   - verifier: значение verifier типа Verifier, используемое согласно назначению этой операции.
//   - tickets: сервис выпуска и проверки ограниченных билетов подключения.
//   - limiter: ограничитель частоты запросов, общий для экземпляров API.
//   - cfg: проверенные настройки соответствующего компонента.
//   - media: значение media типа MediaController, используемое согласно назначению этой операции.
type Handler struct {
	hub         *usecase.Hub
	verifier    Verifier
	tickets     Tickets
	limiter     Limiter
	cfg         config.RealtimeConfig
	media       MediaController
	connections atomic.Int64
}

// MediaController задаёт контракт зависимого компонента MediaController в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Handle: операция Handle с контрактом, описанным у метода.
type MediaController interface {
	// Handle обрабатывает проверенное событие сигнализации в рамках живой серверной сессии.
	//
	// @parameters:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (domain.Session): историческая физическая сессия или состояние текущего соединения.
	//   - аргумент 3 (time.Time): момент окончания действия сессии, токена или аренды.
	//   - аргумент 4 (domain.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Handle(context.Context, domain.Session, time.Time, domain.Envelope) error
}

// NewHandler создаёт и связывает зависимости компонента Handler, используемого в присутствии участников и доставке realtime-событий.
//
// @parameters:
//   - hub (*usecase.Hub): координатор присутствия и доставки событий комнаты.
//   - verifier (Verifier): значение verifier типа Verifier, используемое согласно назначению этой операции.
//   - tickets (Tickets): сервис выпуска и проверки ограниченных билетов подключения.
//   - limiter (Limiter): ограничитель частоты запросов, общий для экземпляров API.
//   - cfg (config.RealtimeConfig): проверенные настройки соответствующего компонента.
//
// @return:
//   - результат 1 (*Handler): созданный компонент с переданными зависимостями.
func NewHandler(hub *usecase.Hub, verifier Verifier, tickets Tickets, limiter Limiter, cfg config.RealtimeConfig) *Handler {
	return &Handler{hub: hub, verifier: verifier, tickets: tickets, limiter: limiter, cfg: cfg}
}

// SetMedia подключает обработчик медиа-команд к WebSocket-транспорту.
//
// @parameters:
//   - controller (MediaController): значение controller типа MediaController, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*Handler): значение, подготовленное операцией для вызывающей стороны.
func (h *Handler) SetMedia(controller MediaController) *Handler { h.media = controller; return h }

// RegisterRoutes регистрирует HTTP-маршруты соответствующего сценария и подключает авторизацию и ограничения запросов.
//
// @parameters:
//   - router (gin.IRouter): значение router типа gin.IRouter, используемое согласно назначению этой операции.
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	router.GET("/api/v1/conferences/:id/ws", h.Connect)
	router.POST("/api/v1/conferences/:id/ws-ticket", h.Ticket)
	router.GET("/api/v1/webrtc/config", h.ICE)
}

// identity извлекает доверенную идентичность пользователя из проверенной авторизации.
//
// @parameters:
//   - r (*http.Request): входящий HTTP-запрос.
//
// @return:
//   - результат 1 (domain.Identity): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (h *Handler) identity(r *http.Request) (domain.Identity, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return domain.Identity{}, apperrors.ErrUnauthorized
	}
	id, expiry, err := h.verifier.VerifyWithExpiry(parts[1])
	return domain.Identity{UserID: id, ExpiresAt: expiry}, err
}

// conferenceID проверяет и нормализует идентификатор конференции из HTTP-маршрута.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func conferenceID(c *gin.Context) (string, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid conferenceId")
	}
	return id.String(), nil
}

// Ticket создаёт ограниченный по времени билет подключения после проверки авторизации.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Ticket(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	identity, err := h.identity(c.Request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	id, err := conferenceID(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if _, err = h.hub.Authorize(ctx, id, identity.UserID); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if h.limiter != nil {
		limit, err := h.limiter.Allow(ctx, h.cfg.Namespace+":ticket-limit:"+identity.UserID, 30, time.Minute)
		if err != nil {
			httpresponse.Fail(c, err)
			return
		}
		if !limit.Allowed {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(429, gin.H{"error": "too many connection tickets"})
			return
		}
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	ticket := base64.RawURLEncoding.EncodeToString(random[:])
	ttl := min(h.cfg.TicketTTL, time.Until(identity.ExpiresAt))
	if ttl <= 0 {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	if err = h.tickets.SaveTicket(ctx, ticket, id, identity, ttl); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ticket": ticket, "expiresAt": time.Now().UTC().Add(ttl)})
}

// ICE выдаёт авторизованному пользователю STUN/TURN config с временными
// credentials и relay-policy. Shared secret не включается в ответ; no-store
// предотвращает кеширование credentials за пределами их короткого TTL.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) ICE(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := h.identity(c.Request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, h.cfg.TURN.ClientICE(h.cfg.ICE, time.Now()))
}

// origin проверяет Origin подключения по настроенному списку допустимых источников.
//
// @parameters:
//   - r (*http.Request): входящий HTTP-запрос.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (h *Handler) origin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	} // Native clients use Bearer headers.
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if len(h.cfg.AllowedOrigins) == 0 {
		return strings.EqualFold(u.Host, r.Host)
	}
	for _, allowed := range h.cfg.AllowedOrigins {
		if strings.TrimSuffix(allowed, "/") == origin {
			return true
		}
	}
	return false
}

// Connect проверяет билет, Origin и членство, открывает WebSocket и запускает единственные циклы чтения и записи.
//
// @parameters:
//   - c (*gin.Context): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
func (h *Handler) Connect(c *gin.Context) {
	if h.cfg.MaxConnections > 0 && h.connections.Add(1) > int64(h.cfg.MaxConnections) {
		h.connections.Add(-1)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	if h.cfg.MaxConnections <= 0 {
		h.connections.Add(1)
	}
	reserved := true
	defer func() {
		if reserved {
			h.connections.Add(-1)
		}
	}()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if !h.origin(c.Request) {
		httpresponse.Fail(c, apperrors.ErrForbidden)
		return
	}
	id, err := conferenceID(c)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || len(c.Request.URL.RawQuery) > 512 || len(query) > 1 || (len(query) == 1 && len(query["ticket"]) != 1) {
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	var identity domain.Identity
	if ticket := query.Get("ticket"); ticket != "" {
		if c.GetHeader("Authorization") != "" {
			httpresponse.Fail(c, apperrors.ErrUnauthorized)
			return
		}
		identity, err = h.tickets.ConsumeTicket(ctx, ticket, id)
	} else {
		identity, err = h.identity(c.Request)
	}
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	session, err := h.hub.Prepare(ctx, id, identity.UserID)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	if !identity.ExpiresAt.After(time.Now()) {
		h.hub.Abort(session)
		httpresponse.Fail(c, apperrors.ErrUnauthorized)
		return
	}
	upgrader := ws.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, HandshakeTimeout: 5 * time.Second, CheckOrigin: h.origin, Subprotocols: []string{"go-recorder.v1"}}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.hub.Abort(session)
		return
	}
	client := newClient(conn, h, session, identity.ExpiresAt)
	registration, cancelRegistration := context.WithTimeout(context.Background(), 5*time.Second)
	err = h.hub.Register(registration, session, client)
	cancelRegistration()
	if err != nil {
		client.Stop("registration_failed")
		_ = conn.Close()
		return
	}
	reserved = false
	go func() {
		operations.WSActive(1)
		defer operations.WSActive(-1)
		defer h.connections.Add(-1)
		client.run()
	}()
}

// control передаёт управляющую WebSocket-команду писателю сокета.
// Состав:
//   - kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - data: полезная нагрузка события или байты обрабатываемого содержимого.
type control struct {
	kind int
	data []byte
}

// client хранит физический WebSocket-клиент, единственного писателя, очереди событий и ограничения входящих сообщений.
// Состав:
//   - conn: действующее сетевое соединение операции.
//   - handler: обработчик вызываемой команды или маршрута.
//   - session: историческая физическая сессия или состояние текущего соединения.
//   - expiresAt: момент окончания действия сессии, токена или аренды.
//   - out: ограниченная очередь исходящих данных.
//   - low: отдельная ограниченная очередь восстанавливаемых событий.
//   - controls: канал «управление» для передачи данных или завершения ожидания.
//   - done: канал уведомления о завершении ресурса.
//   - once: значение once типа sync.Once, используемое согласно назначению этой операции.
//   - reason: причина завершения, отказа или изменения состояния.
type client struct {
	conn      *ws.Conn
	handler   *Handler
	session   domain.Session
	expiresAt time.Time
	out       chan domain.Envelope
	low       chan domain.Envelope
	controls  chan control
	done      chan struct{}
	once      sync.Once
	reason    string
}

// newClient создаёт состояние сокета с единственным писателем и ограниченными очередями событий.
//
// @parameters:
//   - conn (*ws.Conn): действующее сетевое соединение операции.
//   - h (*Handler): значение h типа *Handler, используемое согласно назначению этой операции.
//   - s (domain.Session): значение s типа domain.Session, используемое согласно назначению этой операции.
//   - expiry (time.Time): временная отметка expiry; указатель допускает отсутствие значения.
//
// @return:
//   - результат 1 (*client): значение, подготовленное операцией для вызывающей стороны.
func newClient(conn *ws.Conn, h *Handler, s domain.Session, expiry time.Time) *client {
	return &client{conn: conn, handler: h, session: s, expiresAt: expiry, out: make(chan domain.Envelope, h.cfg.QueueSize), low: make(chan domain.Envelope, 8), controls: make(chan control, 8), done: make(chan struct{})}
}

// Offer ставит событие в соответствующую ограниченную очередь сокета с учётом приоритета.
//
// @parameters:
//   - event (domain.Envelope): конверт входящего или публикуемого события.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (c *client) Offer(event domain.Envelope) bool {
	if limit := c.handler.cfg.OutboundBytes; limit > 0 && len(event.Data) > limit {
		c.Stop("outbound_too_large")
		return false
	}
	select {
	case <-c.done:
		return false
	default:
	}
	if domain.LowPriorityEvent(event.Type) {
		select {
		case c.low <- event:
		default:
		}
		return true // Dropping recoverable events must not close media sockets.
	}
	select {
	case c.out <- event:
		return true
	default:
		c.Stop("slow_client")
		return false
	}
}

// Stop останавливает активную обработку физических сессий и событий комнаты и освобождает связанные ресурсы.
//
// @parameters:
//   - reason (string): причина завершения, отказа или изменения состояния.
func (c *client) Stop(reason string) {
	c.once.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

		 */func() { c.reason = reason; close(c.done) })
}

// run выполняет основной цикл компонента до завершения работы или отмены контекста.
func (c *client) run() {
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

	 */func() { c.handler.hub.Unregister(c.session) }()
	written := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

	 */func() { defer close(written); c.write() }()
	c.read()
	c.Stop("client_closed")
	<-written
}

// write последовательно записывает события и управляющие кадры сокета, отдавая приоритет критичным событиям.
func (c *client) write() {
	defer c.conn.Close()
	ticker := time.NewTicker(c.handler.cfg.PingInterval)
	defer ticker.Stop()
	expiry := time.NewTimer(max(time.Until(c.expiresAt), time.Nanosecond))
	defer expiry.Stop()
	// Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.
	//
	// @parameters:
	//   - kind (int): тип события, ошибки или медиа, определяющий ветку обработки.
	//   - data ([]byte): полезная нагрузка события или байты обрабатываемого содержимого.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	write := func(kind int, data []byte) bool {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.handler.cfg.WriteTimeout))
		return c.conn.WriteMessage(kind, data) == nil
	}
	for {
		select {
		case <-c.done:
			code := ws.CloseNormalClosure
			if c.reason == "server_shutdown" {
				code = ws.CloseGoingAway
			} else if c.reason != "client_closed" {
				code = ws.ClosePolicyViolation
			}
			_ = write(ws.CloseMessage, ws.FormatCloseMessage(code, c.reason))
			return
		case <-expiry.C:
			c.Stop("authentication_expired")
		case <-ticker.C:
			if !write(ws.PingMessage, []byte(c.session.ConnectionID)) {
				c.Stop("write_failed")
				return
			}
		case frame := <-c.controls:
			if !write(frame.kind, frame.data) {
				c.Stop("write_failed")
				return
			}
		case event := <-c.out:
			raw, err := json.Marshal(event)
			if err != nil || !write(ws.TextMessage, raw) {
				c.Stop("write_failed")
				return
			}
		case event := <-c.low:
			// If both queues are ready, always write the critical event first.
			// The selected low-priority event may be dropped, like queue overflow.
			select {
			case event = <-c.out:
			default:
			}
			raw, err := json.Marshal(event)
			if err != nil || !write(ws.TextMessage, raw) {
				c.Stop("write_failed")
				return
			}
		}
	}
}

// bucket хранит локальное состояние ограничения частоты для физического сокета.
// Состав:
//   - tokens: сервис выпуска или проверки JWT авторизации.
//   - updated: временная отметка updated; указатель допускает отсутствие значения.
//   - rate: значение rate типа float64, используемое согласно назначению этой операции.
//   - burst: значение burst типа float64, используемое согласно назначению этой операции.
type bucket struct {
	tokens      float64
	updated     time.Time
	rate, burst float64
}

// allow проверяет локальный лимит входящего сообщения физического сокета.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (b *bucket) allow() bool {
	now := time.Now()
	b.tokens = min(b.burst, b.tokens+now.Sub(b.updated).Seconds()*b.rate)
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// read читает состояние физических сессий и событий комнаты для дальнейшей обработки или ответа.
func (c *client) read() {
	cfg := c.handler.cfg
	c.conn.SetReadLimit(cfg.MessageBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(cfg.PingInterval + cfg.PongTimeout))
	rate := bucket{tokens: float64(cfg.Burst), updated: time.Now(), rate: float64(cfg.MessagesPerSecond), burst: float64(cfg.Burst)}
	lastPong := time.Time{}
	c.conn.SetPongHandler( /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

		@parameters:
		  - value (string): значение для проверки, нормализации или преобразования.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(value string) error {
			if !c.expiresAt.After(time.Now()) {
				c.Stop("authentication_expired")
				return errors.New("authentication expired")
			}
			if !rate.allow() {
				return errors.New("control rate exceeded")
			}
			if value != c.session.ConnectionID || time.Since(lastPong) < cfg.PingInterval/2 {
				return nil
			}
			lastPong = time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := c.handler.hub.Touch(ctx, c.session); err != nil {
				return errors.New("presence lease lost")
			}
			c.session.LastSeenAt = lastPong.UTC()
			return c.conn.SetReadDeadline(time.Now().Add(cfg.PingInterval + cfg.PongTimeout))
		})
	c.conn.SetPingHandler( /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

		@parameters:
		  - value (string): значение для проверки, нормализации или преобразования.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(value string) error {
			if !rate.allow() {
				return errors.New("control rate exceeded")
			}
			select {
			case c.controls <- control{ws.PongMessage, []byte(value)}:
				return nil
			default:
				return errors.New("control overflow")
			}
		})
	c.conn.SetCloseHandler( /* Вложенный обработчик выполняет выделенный шаг обработки в присутствии участников и доставке realtime-событий, используя состояние окружающей функции.

		@parameters:
		  - _ (int): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		  - _ (string): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(_ int, _ string) error { c.Stop("client_closed"); return nil })
	for {
		kind, raw, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if !c.expiresAt.After(time.Now()) {
			c.Stop("authentication_expired")
			return
		}
		if !rate.allow() {
			c.Stop("rate_limited")
			return
		}
		if kind != ws.TextMessage {
			c.Stop("text_messages_only")
			return
		}
		var event domain.Envelope
		if !utf8.Valid(raw) || strictJSON(raw, &event) != nil {
			c.failure("", "invalid_message")
			continue
		}
		if parsed, err := uuid.Parse(event.ID); err != nil || parsed == uuid.Nil || event.Version != 1 || event.ConferenceID != c.session.ConferenceID || event.Timestamp.IsZero() || event.ReplyTo != "" {
			c.failure(event.ID, "invalid_envelope")
			continue
		}
		operations.WSMessage(event.Type)
		if strings.HasPrefix(event.Type, "media.") {
			if c.handler.media == nil {
				c.failure(event.ID, "media_unavailable")
				continue
			}
			ctx, cancel := context.WithTimeout(operations.WithID(context.Background(), event.ID), 10*time.Second)
			err := c.handler.media.Handle(ctx, c.session, c.expiresAt, event)
			cancel()
			if err != nil {
				c.failure(event.ID, mediadomain.ErrorCode(err))
			}
			continue
		}
		if event.Type != "webrtc.offer" && event.Type != "webrtc.answer" && event.Type != "webrtc.ice" {
			c.failure(event.ID, "unsupported_event")
			continue
		}
		var signal domain.Signal
		if strictJSON(event.Data, &signal) != nil || signal.SenderConnectionID != "" || signal.SenderParticipantID != "" {
			c.failure(event.ID, "invalid_signal")
			continue
		}
		target, err := uuid.Parse(signal.TargetConnectionID)
		if err != nil || target == uuid.Nil || target.String() == c.session.ConnectionID {
			c.failure(event.ID, "invalid_target")
			continue
		}
		signal.TargetConnectionID = target.String()
		if event.Type == "webrtc.ice" {
			var candidate map[string]json.RawMessage
			if signal.SDP != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > cfg.ICEBytes || json.Unmarshal(signal.Candidate, &candidate) != nil || candidate == nil {
				c.failure(event.ID, "invalid_ice")
				continue
			}
		} else if len(signal.Candidate) != 0 || len(signal.SDP) == 0 || len(signal.SDP) > cfg.SDPBytes {
			c.failure(event.ID, "invalid_sdp")
			continue
		}
		signal.SenderConnectionID = c.session.ConnectionID
		signal.SenderParticipantID = c.session.ParticipantID
		forward := domain.Event(event.Type, c.session.ConferenceID, signal)
		forward.ReplyTo = event.ID
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = c.handler.hub.SendToConnection(ctx, c.session, target.String(), forward)
		cancel()
		if err != nil {
			if errors.Is(err, apperrors.ErrForbidden) || errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrConflict) {
				c.failure(event.ID, "target_unavailable")
			} else {
				c.Stop("broker_unavailable")
				return
			}
		} else {
			ack := domain.Event("ack", c.session.ConferenceID, map[string]string{"type": event.Type})
			ack.ReplyTo = event.ID
			c.Offer(ack)
		}
	}
}

// failure формирует безопасное событие ошибки протокола для клиента.
//
// @parameters:
//   - replyTo (string): идентификатор исходного запроса или сообщения, на которое даётся ответ.
//   - code (string): код приглашения или машинный код результата.
func (c *client) failure(replyTo, code string) {
	event := domain.Event("error", c.session.ConferenceID, map[string]string{"code": code})
	if len(replyTo) <= 36 {
		event.ReplyTo = replyTo
	}
	c.Offer(event)
}

// strictJSON строго разбирает JSON-пакет и отвергает неизвестные поля и лишние данные.
//
// @parameters:
//   - raw ([]byte): исходные байты JSON, пакета или сериализованного значения.
//   - value (any): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func strictJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
