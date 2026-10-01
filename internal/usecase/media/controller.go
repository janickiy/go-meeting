package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type Registry interface {
	Workers(context.Context) ([]domain.Worker, error)
	Claim(context.Context, string, string, time.Duration) (domain.Route, error)
	GetOwner(context.Context, string) (domain.Route, error)
}
type Tickets interface {
	Issue(domain.Binding, domain.Route) (string, error)
}
type Transport interface {
	Call(context.Context, string, domain.Command) (domain.Result, error)
}
type Sessions interface {
	ValidateSession(context.Context, realtime.Session) error
}
type Publisher interface {
	Publish(context.Context, realtime.Bus) error
}
type endpoint struct {
	binding domain.Binding
	route   domain.Route
	peerID  string
}

type Controller struct {
	registry  Registry
	tickets   Tickets
	transport Transport
	sessions  Sessions
	publisher Publisher
	cfg       config.MediaConfig
	limits    config.RealtimeConfig
	mu        sync.Mutex
	peers     map[string]endpoint
}

func NewController(registry Registry, tickets Tickets, transport Transport, sessions Sessions, publisher Publisher, cfg config.MediaConfig, limits config.RealtimeConfig) *Controller {
	limits.SDPBytes = min(limits.SDPBytes, domain.MaxSDPBytes)
	limits.ICEBytes = min(limits.ICEBytes, domain.MaxICEBytes)
	return &Controller{registry: registry, tickets: tickets, transport: transport, sessions: sessions, publisher: publisher, cfg: cfg, limits: limits, peers: map[string]endpoint{}}
}

func (c *Controller) Handle(ctx context.Context, session realtime.Session, expiresAt time.Time, event realtime.Envelope) error {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.OperationTimeout)
	defer cancel()
	if !expiresAt.After(time.Now()) || c.sessions.ValidateSession(ctx, session) != nil {
		return domain.ErrUnauthorized
	}
	signal, err := c.decode(event.Type, event.Data)
	if err != nil {
		return err
	}
	binding := domain.Binding{ConferenceID: session.ConferenceID, ParticipantID: session.ParticipantID, SessionID: session.ID, ConnectionID: session.ConnectionID, UserID: session.UserID, AuthorizationExpiresAt: expiresAt}
	if event.Type == "media.join" {
		return c.join(ctx, binding, event.ID)
	}
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	c.mu.Unlock()
	if !exists {
		if event.Type == "media.leave" {
			return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": signal.MediaPeerID})
		}
		return domain.ErrPeerNotFound
	}
	// Empty media.leave cancels a join that was queued before the browser knew
	// its peer ID. The single WS reader serializes it after the join response;
	// its authenticated session is the only allowed cleanup target.
	if signal.MediaPeerID != peer.peerID && !(event.Type == "media.leave" && signal.MediaPeerID == "") {
		return domain.ErrUnauthorized
	}
	owner, err := c.registry.GetOwner(ctx, session.ConferenceID)
	if err != nil {
		return domain.ErrOwnership
	}
	if owner != peer.route {
		return domain.ErrOwnership
	}
	command := domain.Command{RequestID: event.ID, Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID, NegotiationID: signal.NegotiationID, SDP: signal.SDP, Candidate: signal.Candidate, TrackID: signal.TrackID}
	action := strings.TrimPrefix(event.Type, "media.")
	result, err := c.transport.Call(ctx, action, command)
	if err != nil {
		return err
	}
	if action == "offer" {
		if result.MediaPeerID != peer.peerID || result.SDP == "" || len(result.SDP) > c.limits.SDPBytes {
			return domain.ErrUnavailable
		}
		return c.emit(ctx, binding, "media.answer", event.ID, map[string]string{"mediaPeerId": peer.peerID, "negotiationId": signal.NegotiationID, "sdp": result.SDP})
	}
	if action == "leave" {
		c.mu.Lock()
		delete(c.peers, session.ConnectionID)
		c.mu.Unlock()
		return c.emit(ctx, binding, "media.left", event.ID, map[string]string{"mediaPeerId": peer.peerID})
	}
	return c.emit(ctx, binding, "ack", event.ID, map[string]string{"type": event.Type})
}

func (c *Controller) join(ctx context.Context, binding domain.Binding, requestID string) error {
	workers, err := c.registry.Workers(ctx)
	if err != nil || len(workers) == 0 {
		return domain.ErrUnavailable
	}
	// A small stable hash distributes new conferences; Claim preserves the
	// existing healthy owner. This is routing, not a distributed scheduler.
	conf, _ := uuid.Parse(binding.ConferenceID)
	index := int(conf[0]) % len(workers)
	route, err := c.registry.Claim(ctx, binding.ConferenceID, workers[index].ID, c.cfg.OwnershipTTL)
	if err != nil {
		return domain.ErrUnavailable
	}
	ticket, err := c.tickets.Issue(binding, route)
	if err != nil {
		return domain.ErrUnauthorized
	}
	result, err := c.transport.Call(ctx, "join", domain.Command{RequestID: requestID, Binding: binding, Route: route, Ticket: ticket})
	if err != nil {
		return err
	}
	if parsed, err := uuid.Parse(result.MediaPeerID); err != nil || parsed == uuid.Nil || result.MaxPeers < 2 || result.MaxPeers > 32 || result.WorkerID != route.WorkerID {
		return domain.ErrUnavailable
	}
	video := result.VideoCapture
	if video.MaxWidth < 1 || video.MaxWidth > 1280 || video.MaxHeight < 1 || video.MaxHeight > 720 || video.MaxFrameRate < 1 || video.MaxFrameRate > 30 {
		// The worker prepared a peer, but an invalid capture target may not reach
		// the browser. Do not leave that prepared media endpoint orphaned.
		cleanup, cancel := context.WithTimeout(context.Background(), min(c.cfg.OperationTimeout, 3*time.Second))
		_, _ = c.transport.Call(cleanup, "leave", domain.Command{RequestID: uuid.NewString(), Binding: binding, Route: route, MediaPeerID: result.MediaPeerID})
		cancel()
		return domain.ErrUnavailable
	}
	peer := endpoint{binding: binding, route: route, peerID: result.MediaPeerID}
	c.mu.Lock()
	c.peers[binding.ConnectionID] = peer
	c.mu.Unlock()
	if result.Tracks == nil {
		result.Tracks = []domain.Track{}
	}
	if result.ICEServers == nil {
		result.ICEServers = []realtime.ICEServer{}
	}
	err = c.emit(ctx, binding, "media.joined", requestID, map[string]any{"mediaPeerId": result.MediaPeerID, "workerId": result.WorkerID, "maxPeers": result.MaxPeers, "iceServers": result.ICEServers, "tracks": result.Tracks, "videoCapture": result.VideoCapture})
	if err != nil {
		c.Disconnected(context.Background(), realtime.Session{ConnectionID: binding.ConnectionID})
	}
	return err
}

func (c *Controller) emit(ctx context.Context, binding domain.Binding, kind, replyTo string, data any) error {
	event := realtime.Event(kind, binding.ConferenceID, data)
	event.ReplyTo = replyTo
	if err := c.publisher.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event}); err != nil {
		return domain.ErrUnavailable
	}
	return nil
}

// Disconnected is called outside the Hub lock. Lost cleanup is repaired by the
// worker's authoritative Redis session/authorization expiry sweep.
func (c *Controller) Disconnected(ctx context.Context, session realtime.Session) {
	c.mu.Lock()
	peer, exists := c.peers[session.ConnectionID]
	delete(c.peers, session.ConnectionID)
	c.mu.Unlock()
	if !exists {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, min(c.cfg.OperationTimeout, 3*time.Second))
	defer cancel()
	_, _ = c.transport.Call(ctx, "leave", domain.Command{RequestID: uuid.NewString(), Binding: peer.binding, Route: peer.route, MediaPeerID: peer.peerID})
}

func strictDecode(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return domain.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil {
		return domain.ErrInvalid
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ErrInvalid
	}
	return nil
}

func validUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

func (c *Controller) decode(kind string, raw json.RawMessage) (domain.Signal, error) {
	var signal domain.Signal
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return signal, domain.ErrInvalid
	}
	if strictDecode(raw, &signal) != nil {
		return signal, domain.ErrInvalid
	}
	if kind == "media.join" {
		if signal.MediaPeerID != "" || signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
		return signal, nil
	}
	if !validUUID(signal.MediaPeerID) && !(kind == "media.leave" && signal.MediaPeerID == "") {
		return signal, domain.ErrInvalid
	}
	switch kind {
	case "media.offer":
		if !validUUID(signal.NegotiationID) || signal.SDP == "" || len(signal.SDP) > c.limits.SDPBytes || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.ready":
		if !validUUID(signal.NegotiationID) || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.ice":
		if signal.NegotiationID != "" || signal.SDP != "" || signal.TrackID != "" || len(signal.Candidate) == 0 || len(signal.Candidate) > c.limits.ICEBytes {
			return signal, domain.ErrInvalid
		}
		if string(signal.Candidate) != "null" {
			var ice struct {
				Candidate        string  `json:"candidate"`
				SDPMid           *string `json:"sdpMid"`
				SDPMLineIndex    *uint16 `json:"sdpMLineIndex"`
				UsernameFragment *string `json:"usernameFragment"`
			}
			if strictDecode(signal.Candidate, &ice) != nil || strings.ContainsAny(ice.Candidate, "\r\n\x00") || (ice.SDPMid != nil && len(*ice.SDPMid) > 128) || (ice.UsernameFragment != nil && len(*ice.UsernameFragment) > 256) {
				return signal, domain.ErrInvalid
			}
		}
	case "media.leave":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || signal.TrackID != "" {
			return signal, domain.ErrInvalid
		}
	case "media.unpublish":
		if signal.NegotiationID != "" || signal.SDP != "" || len(signal.Candidate) != 0 || len(signal.TrackID) < 1 || len(signal.TrackID) > 128 || strings.ContainsAny(signal.TrackID, "\r\n\x00") {
			return signal, domain.ErrInvalid
		}
	default:
		return signal, domain.ErrInvalid
	}
	return signal, nil
}
