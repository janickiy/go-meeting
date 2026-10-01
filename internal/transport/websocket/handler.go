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
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/go-recorder/internal/app/httpresponse"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	usecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

type Verifier interface {
	VerifyWithExpiry(string) (string, time.Time, error)
}
type Tickets interface {
	SaveTicket(context.Context, string, string, domain.Identity, time.Duration) error
	ConsumeTicket(context.Context, string, string) (domain.Identity, error)
}
type Limiter interface {
	Allow(context.Context, string, int, time.Duration) (ratelimit.Result, error)
}
type Handler struct {
	hub      *usecase.Hub
	verifier Verifier
	tickets  Tickets
	limiter  Limiter
	cfg      config.RealtimeConfig
}

func NewHandler(hub *usecase.Hub, verifier Verifier, tickets Tickets, limiter Limiter, cfg config.RealtimeConfig) *Handler {
	return &Handler{hub, verifier, tickets, limiter, cfg}
}
func (h *Handler) RegisterRoutes(router gin.IRouter) {
	router.GET("/api/v1/conferences/:id/ws", h.Connect)
	router.POST("/api/v1/conferences/:id/ws-ticket", h.Ticket)
	router.GET("/api/v1/webrtc/config", h.ICE)
}
func (h *Handler) identity(r *http.Request) (domain.Identity, error) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return domain.Identity{}, apperrors.ErrUnauthorized
	}
	id, expiry, err := h.verifier.VerifyWithExpiry(parts[1])
	return domain.Identity{UserID: id, ExpiresAt: expiry}, err
}
func conferenceID(c *gin.Context) (string, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid conferenceId")
	}
	return id.String(), nil
}
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
func (h *Handler) ICE(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, err := h.identity(c.Request); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(200, h.cfg.ICE)
}
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
func (h *Handler) Connect(c *gin.Context) {
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
	go client.run()
}

type control struct {
	kind int
	data []byte
}
type client struct {
	conn      *ws.Conn
	handler   *Handler
	session   domain.Session
	expiresAt time.Time
	out       chan domain.Envelope
	controls  chan control
	done      chan struct{}
	once      sync.Once
	reason    string
}

func newClient(conn *ws.Conn, h *Handler, s domain.Session, expiry time.Time) *client {
	return &client{conn: conn, handler: h, session: s, expiresAt: expiry, out: make(chan domain.Envelope, h.cfg.QueueSize), controls: make(chan control, 8), done: make(chan struct{})}
}
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
	select {
	case c.out <- event:
		return true
	default:
		c.Stop("slow_client")
		return false
	}
}
func (c *client) Stop(reason string) { c.once.Do(func() { c.reason = reason; close(c.done) }) }
func (c *client) run() {
	defer func() { c.handler.hub.Unregister(c.session) }()
	written := make(chan struct{})
	go func() { defer close(written); c.write() }()
	c.read()
	c.Stop("client_closed")
	<-written
}
func (c *client) write() {
	defer c.conn.Close()
	ticker := time.NewTicker(c.handler.cfg.PingInterval)
	defer ticker.Stop()
	expiry := time.NewTimer(max(time.Until(c.expiresAt), time.Nanosecond))
	defer expiry.Stop()
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
		}
	}
}

// Application and control frames share a token bucket, preventing ping/pong
// flooding from bypassing message limits. Only the reader mutates this bucket.
type bucket struct {
	tokens      float64
	updated     time.Time
	rate, burst float64
}

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
func (c *client) read() {
	cfg := c.handler.cfg
	c.conn.SetReadLimit(cfg.MessageBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(cfg.PingInterval + cfg.PongTimeout))
	rate := bucket{tokens: float64(cfg.Burst), updated: time.Now(), rate: float64(cfg.MessagesPerSecond), burst: float64(cfg.Burst)}
	lastPong := time.Time{}
	c.conn.SetPongHandler(func(value string) error {
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
	c.conn.SetPingHandler(func(value string) error {
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
	c.conn.SetCloseHandler(func(_ int, _ string) error { c.Stop("client_closed"); return nil })
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
func (c *client) failure(replyTo, code string) {
	event := domain.Event("error", c.session.ConferenceID, map[string]string{"code": code})
	if len(replyTo) <= 36 {
		event.ReplyTo = replyTo
	}
	c.Offer(event)
}
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
