package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

type Repository interface {
	Authorize(context.Context, string, string) (conferences.Participant, error)
	Open(context.Context, domain.Session) error
	Close(context.Context, string, time.Time) error
	Stale(context.Context, time.Time, string) ([]domain.Session, error)
	Roster(context.Context, string) (conferences.Status, []conferences.Participant, error)
}
type Store interface {
	Register(context.Context, domain.Session, time.Duration) error
	Unregister(context.Context, string) error
	Touch(context.Context, string, time.Duration) error
	Get(context.Context, string) (domain.Session, error)
	Active(context.Context, string) ([]domain.Session, error)
	Prune(context.Context) error
	Publish(context.Context, domain.Bus) error
	Subscribe(context.Context) (domain.Subscription, error)
}

// Socket owns its bounded queue and its single writer. Hub never writes sockets.
type Socket interface {
	Offer(domain.Envelope) bool
	Stop(string)
}
type DisconnectObserver interface {
	Disconnected(context.Context, domain.Session)
}

// DisconnectObservers composes bounded cleanup hooks without making one
// subsystem responsible for another subsystem's lifecycle.
type DisconnectObservers []DisconnectObserver

func (observers DisconnectObservers) Disconnected(ctx context.Context, session domain.Session) {
	for _, observer := range observers {
		if observer != nil {
			observer.Disconnected(ctx, session)
		}
	}
}

type localSocket struct {
	session domain.Session
	socket  Socket
	ready   bool
}
type Hub struct {
	repo               Repository
	store              Store
	ttl                time.Duration
	logger             *slog.Logger
	ctx                context.Context
	cancel             context.CancelFunc
	sub                domain.Subscription
	mu                 sync.Mutex
	closing            bool
	local              map[string]*localSocket
	sockets            sync.WaitGroup
	workers            sync.WaitGroup
	disconnectObserver DisconnectObserver
	hands              HandStore
	lowEvents          chan domain.Bus
}

func NewHub(repo Repository, store Store, ttl time.Duration, logger *slog.Logger) (*Hub, error) {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = slog.Default()
	}
	h := &Hub{repo: repo, store: store, ttl: ttl, logger: logger, ctx: ctx, cancel: cancel, local: map[string]*localSocket{}, lowEvents: make(chan domain.Bus, 128)}
	op, c := context.WithTimeout(ctx, 5*time.Second)
	defer c()
	sub, err := store.Subscribe(op)
	if err != nil {
		cancel()
		return nil, err
	}
	h.sub = sub
	h.workers.Add(3)
	go h.receive()
	go h.receiveLowPriority()
	go h.janitor()
	return h, nil
}
func (h *Hub) Authorize(ctx context.Context, conferenceID, userID string) (conferences.Participant, error) {
	return h.repo.Authorize(ctx, conferenceID, userID)
}
func (h *Hub) SetDisconnectObserver(observer DisconnectObserver) {
	h.mu.Lock()
	h.disconnectObserver = observer
	h.mu.Unlock()
}
func (h *Hub) SetHands(store HandStore) { h.mu.Lock(); h.hands = store; h.mu.Unlock() }

// ValidateSession binds media commands to a still-authorized, live physical
// WebSocket connection instead of trusting identifiers from a browser payload.
func (h *Hub) ValidateSession(ctx context.Context, session domain.Session) error {
	p, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID)
	if err != nil {
		return err
	}
	live, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil {
		return err
	}
	if p.ID != session.ParticipantID || live.ID != session.ID || live.ConferenceID != session.ConferenceID || live.ParticipantID != session.ParticipantID || live.UserID != session.UserID || live.ConnectionID != session.ConnectionID || live.Status != "connected" {
		return apperrors.ErrForbidden
	}
	return nil
}
func (h *Hub) Prepare(ctx context.Context, conferenceID, userID string) (domain.Session, error) {
	h.mu.Lock()
	closing := h.closing
	h.mu.Unlock()
	if closing {
		return domain.Session{}, apperrors.ErrConflict
	}
	p, err := h.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return domain.Session{}, err
	}
	now := time.Now().UTC()
	s := domain.Session{ID: uuid.NewString(), ConferenceID: conferenceID, ParticipantID: p.ID, UserID: userID, ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: now, LastSeenAt: now}
	return s, h.repo.Open(ctx, s)
}
func (h *Hub) Abort(session domain.Session) {
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = h.repo.Close(ctx, session.ConnectionID, session.LastSeenAt)
}
func (h *Hub) Register(ctx context.Context, session domain.Session, socket Socket) error {
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		h.Abort(session)
		return apperrors.ErrConflict
	}
	h.sockets.Add(1)
	h.local[session.ConnectionID] = &localSocket{session: session, socket: socket}
	h.mu.Unlock()
	if err := h.store.Register(ctx, session, h.ttl); err != nil {
		h.Unregister(session)
		return err
	}
	// Covers finish/leave concurrent with upgrade and Redis registration.
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		h.Unregister(session)
		return err
	}
	state, err := h.state(ctx, session)
	if err == nil && !stateAllows(state, session.ParticipantID) {
		err = apperrors.ErrForbidden
	}
	if err != nil || !socket.Offer(domain.Event("conference.state", session.ConferenceID, stateFor(state, session))) {
		h.Unregister(session)
		if err != nil {
			return err
		}
		return apperrors.ErrConflict
	}
	h.mu.Lock()
	if entry := h.local[session.ConnectionID]; entry != nil {
		entry.ready = true
	}
	h.mu.Unlock()
	h.log(session, "connected")
	return nil
}
func (h *Hub) Unregister(session domain.Session) {
	h.mu.Lock()
	_, exists := h.local[session.ConnectionID]
	delete(h.local, session.ConnectionID)
	observer := h.disconnectObserver
	h.mu.Unlock()
	if !exists {
		return
	}
	defer h.sockets.Done()
	ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	seen := session.LastSeenAt
	if live, err := h.store.Get(ctx, session.ConnectionID); err == nil {
		seen = live.LastSeenAt
	}
	if err := h.store.Unregister(ctx, session.ConnectionID); err != nil {
		h.log(session, "redis_cleanup_failed")
	}
	if err := h.repo.Close(ctx, session.ConnectionID, seen); err != nil {
		h.log(session, "history_cleanup_failed")
	}
	// Observers see the session already closed. In particular, two tabs closing
	// together cannot both mistake the other tab for the last active session.
	if observer != nil {
		observer.Disconnected(ctx, session)
	}
	h.log(session, "disconnected")
}
func (h *Hub) Touch(ctx context.Context, session domain.Session) error {
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		return err
	}
	return h.store.Touch(ctx, session.ConnectionID, h.ttl)
}
func (h *Hub) GetActiveSessions(ctx context.Context, conferenceID string) ([]domain.Session, error) {
	return h.store.Active(ctx, conferenceID)
}
func (h *Hub) Broadcast(ctx context.Context, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: event.ConferenceID, Event: &event})
}
func (h *Hub) SendToParticipant(ctx context.Context, conferenceID, participantID string, event domain.Envelope) error {
	return h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: conferenceID, ParticipantID: participantID, Event: &event})
}
func (h *Hub) SendToConnection(ctx context.Context, session domain.Session, targetID string, event domain.Envelope) error {
	if _, err := h.repo.Authorize(ctx, session.ConferenceID, session.UserID); err != nil {
		return err
	}
	sender, err := h.store.Get(ctx, session.ConnectionID)
	if err != nil || sender.ConferenceID != session.ConferenceID || sender.UserID != session.UserID {
		return apperrors.ErrForbidden
	}
	target, err := h.store.Get(ctx, targetID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return apperrors.New(apperrors.ErrNotFound, "target connection is offline")
	}
	if err != nil {
		return err
	}
	if target.ConferenceID != session.ConferenceID {
		return apperrors.ErrForbidden
	}
	if _, err := h.repo.Authorize(ctx, target.ConferenceID, target.UserID); err != nil {
		return apperrors.ErrForbidden
	}
	err = h.store.Publish(ctx, domain.Bus{Kind: "event", ConferenceID: session.ConferenceID, ConnectionID: targetID, Event: &event})
	if err == nil {
		h.log(session, event.Type)
	}
	return err
}

// Called after committed REST mutations. Failure does not lie about a committed
// transaction; heartbeat/message authorization provides a bounded fallback.
func (h *Hub) ConferenceChanged(ctx context.Context, conferenceID string) {
	ctx, c := context.WithTimeout(ctx, 3*time.Second)
	defer c()
	if err := h.store.Publish(ctx, domain.Bus{Kind: "changed", ConferenceID: conferenceID}); err != nil {
		h.logger.Warn("realtime event", "conference_id", conferenceID, "event_type", "mutation_publish_failed")
	}
}
func (h *Hub) state(ctx context.Context, session domain.Session) (domain.State, error) {
	status, roster, err := h.repo.Roster(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	active, err := h.store.Active(ctx, session.ConferenceID)
	if err != nil {
		return domain.State{}, err
	}
	byParticipant := map[string][]string{}
	for _, s := range active {
		byParticipant[s.ParticipantID] = append(byParticipant[s.ParticipantID], s.ConnectionID)
	}
	state := domain.State{ConnectionID: session.ConnectionID, ParticipantID: session.ParticipantID, Status: status, Participants: make([]domain.Presence, 0, len(roster)), Hands: []domain.Hand{}}
	eligible := map[string]bool{}
	for _, p := range roster {
		eligible[p.ID] = p.CanParticipate()
		ids := byParticipant[p.ID]
		if !p.CanParticipate() {
			ids = nil
		}
		if ids == nil {
			ids = []string{}
		}
		sort.Strings(ids)
		state.Participants = append(state.Participants, domain.Presence{ParticipantView: p.View(), Online: len(ids) > 0, Connections: len(ids), ConnectionIDs: ids})
	}
	h.mu.Lock()
	hands := h.hands
	h.mu.Unlock()
	if hands != nil && status == conferences.Active {
		items, err := hands.List(ctx, session.ConferenceID)
		if err != nil {
			return domain.State{}, err
		}
		for _, hand := range items {
			if eligible[hand.ParticipantID] {
				state.Hands = append(state.Hands, hand)
			}
		}
	}
	return state, nil
}

// Build recipient-specific views from one canonical snapshot. Sharing an
// owner's state with an ordinary participant would leak the waiting queue.
func stateFor(state domain.State, session domain.Session) domain.State {
	state.ConnectionID, state.ParticipantID = session.ConnectionID, session.ParticipantID
	moderator := false
	for _, p := range state.Participants {
		if p.ID == session.ParticipantID {
			moderator = p.Role == conferences.Owner || p.Role == conferences.CoHost
			break
		}
	}
	if !moderator {
		visible := make([]domain.Presence, 0, len(state.Participants))
		for _, p := range state.Participants {
			if (p.AdmissionState == conferences.AdmissionAdmitted || p.AdmissionState == "") && (p.Status == conferences.Joined || p.Status == conferences.Left) {
				visible = append(visible, p)
			}
		}
		state.Participants = visible
	}
	return state
}
func stateAllows(state domain.State, participantID string) bool {
	if state.Status != conferences.Created && state.Status != conferences.Active {
		return false
	}
	for _, p := range state.Participants {
		if p.ID == participantID {
			return p.Status == conferences.Joined && (p.AdmissionState == conferences.AdmissionAdmitted || p.AdmissionState == "")
		}
	}
	return false
}
func (h *Hub) receive() {
	defer h.workers.Done()
	for {
		bus, err := h.sub.Receive(h.ctx)
		if err != nil {
			if h.ctx.Err() == nil {
				h.logger.Warn("realtime event", "event_type", "broker_unavailable")
				h.stopSockets("broker_unavailable")
			}
			return // Fail closed: never silently keep sockets on a broken broker.
		}
		if bus.Kind == "event" && bus.Event != nil && domain.LowPriorityEvent(bus.Event.Type) {
			select {
			case h.lowEvents <- bus:
			default:
			} // Durable history repairs overflow.
		} else {
			h.deliver(bus)
		}
	}
}
func (h *Hub) receiveLowPriority() {
	defer h.workers.Done()
	for {
		select {
		case <-h.ctx.Done():
			return
		case bus := <-h.lowEvents:
			h.deliver(bus)
		}
	}
}
func (h *Hub) entries(conferenceID string) []*localSocket {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := make([]*localSocket, 0, len(h.local))
	for _, entry := range h.local {
		if entry.ready && (conferenceID == "" || entry.session.ConferenceID == conferenceID) {
			rows = append(rows, entry)
		}
	}
	return rows
}
func (h *Hub) deliver(bus domain.Bus) {
	ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
	defer c()
	if bus.Kind == "expired" && bus.Session != nil {
		_ = h.repo.Close(ctx, bus.Session.ConnectionID, bus.Session.LastSeenAt)
		h.mu.Lock()
		observer := h.disconnectObserver
		h.mu.Unlock()
		if observer != nil {
			observer.Disconnected(ctx, *bus.Session)
		}
	}
	entries := h.entries(bus.ConferenceID)
	if len(entries) == 0 {
		return
	}
	if bus.Kind == "event" && bus.Event != nil {
		// Revalidate recipients from current durable membership, not the socket's
		// cached identity. A kick/finish can commit before its changed event arrives.
		kind := bus.Event.Type
		restricted := strings.HasPrefix(kind, "chat.") || strings.HasPrefix(kind, "recording.") || strings.HasPrefix(kind, "hand.") || strings.HasPrefix(kind, "reaction.") || strings.HasPrefix(kind, "participant.")
		allowed := map[string]bool{}
		if restricted {
			status, roster, err := h.repo.Roster(ctx, bus.ConferenceID)
			if err != nil {
				return
			}
			for _, p := range roster {
				ok := p.CanParticipate()
				if status == conferences.Finished || status == conferences.Cancelled {
					ok = false
				}
				if kind == "participant.waiting" || kind == "participant.rejected" {
					ok = ok && p.CanAdmit()
				}
				allowed[p.ID] = ok
			}
		}
		if kind == "hand.raised" || kind == "hand.lowered" {
			h.mu.Lock()
			hands := h.hands
			h.mu.Unlock()
			if hands != nil {
				current, err := hands.List(ctx, bus.ConferenceID)
				if err != nil {
					return
				}
				event := currentHandEvent(*bus.Event, current, allowed)
				bus.Event = &event
			}
		}
		for _, entry := range entries {
			s := entry.session
			if restricted && !allowed[s.ParticipantID] {
				continue
			}
			if (bus.ConnectionID == "" || bus.ConnectionID == s.ConnectionID) && (bus.ParticipantID == "" || bus.ParticipantID == s.ParticipantID) {
				if !entry.socket.Offer(*bus.Event) {
					entry.socket.Stop("slow_client")
				}
			}
		}
		return
	}
	if len(entries) == 0 {
		return
	}
	// One snapshot per conference event, not one database/Redis scan per socket.
	state, err := h.state(ctx, entries[0].session)
	if err != nil {
		for _, e := range entries {
			e.socket.Stop("state_unavailable")
		}
		return
	}
	joined := map[string]bool{}
	for _, p := range state.Participants {
		joined[p.ID] = p.Status == conferences.Joined && (p.AdmissionState == conferences.AdmissionAdmitted || p.AdmissionState == "")
	}
	for _, entry := range entries {
		if state.Status == conferences.Finished || state.Status == conferences.Cancelled || !joined[entry.session.ParticipantID] {
			entry.socket.Stop("membership_closed")
			continue
		}
		kind := "conference.state"
		if bus.Kind == "connected" {
			kind = "participant.connected"
		} else if bus.Kind == "disconnected" || bus.Kind == "expired" {
			kind = "participant.disconnected"
		}
		if !entry.socket.Offer(domain.Event(kind, bus.ConferenceID, stateFor(state, entry.session))) {
			entry.socket.Stop("slow_client")
		}
	}
}

// Concurrent Redis mutations can publish in a different order. Resolve each
// hand hint against current state instead of replaying a stale raise/lower.
func currentHandEvent(event domain.Envelope, hands []domain.Hand, allowed map[string]bool) domain.Envelope {
	var target struct {
		ParticipantID string `json:"participantId"`
	}
	if json.Unmarshal(event.Data, &target) != nil {
		return event
	}
	event.Type = "hand.lowered"
	event.Data, _ = json.Marshal(target)
	if allowed[target.ParticipantID] {
		for _, hand := range hands {
			if hand.ParticipantID == target.ParticipantID {
				event.Type = "hand.raised"
				event.Data, _ = json.Marshal(hand)
				break
			}
		}
	}
	return event
}
func (h *Hub) janitor() {
	defer h.workers.Done()
	interval := h.ttl / 4
	if interval > 5*time.Second {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	cursor := ""
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			ctx, c := context.WithTimeout(h.ctx, 5*time.Second)
			if err := h.store.Prune(ctx); err != nil {
				h.stopSockets("broker_unavailable")
			}
			// Crash between SQL open and Redis register is repaired too.
			rows, err := h.repo.Stale(ctx, time.Now().Add(-2*h.ttl), cursor)
			if err == nil {
				for _, s := range rows {
					if _, err := h.store.Get(ctx, s.ConnectionID); errors.Is(err, apperrors.ErrNotFound) {
						_ = h.repo.Close(ctx, s.ConnectionID, s.LastSeenAt)
					}
				}
				if len(rows) < 500 {
					cursor = ""
				} else {
					cursor = rows[len(rows)-1].ID
				}
			}
			c()
		}
	}
}
func (h *Hub) stopSockets(reason string) {
	h.mu.Lock()
	h.closing = true
	entries := make([]*localSocket, 0, len(h.local))
	for _, e := range h.local {
		entries = append(entries, e)
	}
	h.mu.Unlock()
	for _, e := range entries {
		e.socket.Stop(reason)
	}
}
func (h *Hub) Shutdown() {
	h.stopSockets("server_shutdown")
	h.sockets.Wait()
	h.cancel()
	_ = h.sub.Close()
	h.workers.Wait()
}
func (h *Hub) LocalCount() int { h.mu.Lock(); defer h.mu.Unlock(); return len(h.local) }
func (h *Hub) log(s domain.Session, kind string) {
	h.logger.Info("realtime event", "conference_id", s.ConferenceID, "participant_id", s.ParticipantID, "user_id", s.UserID, "connection_id", s.ConnectionID, "event_type", kind)
}
