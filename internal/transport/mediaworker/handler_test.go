package mediaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	"github.com/pion/webrtc/v4"
)

type testEngine struct {
	mu     sync.Mutex
	peers  map[string]media.Binding
	offers int
}

func (e *testEngine) Join(_ context.Context, b media.Binding) (media.PeerView, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, old := range e.peers {
		if sameBinding(old, b) {
			return media.PeerView{MediaPeerID: id}, nil
		}
	}
	id := uuid.NewString()
	e.peers[id] = b
	return media.PeerView{MediaPeerID: id}, nil
}
func (e *testEngine) Offer(_ context.Context, _, _, _ string) (webrtc.SessionDescription, error) {
	e.mu.Lock()
	e.offers++
	e.mu.Unlock()
	return webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: "answer"}, nil
}
func (*testEngine) Ready(context.Context, string, string) error                 { return nil }
func (*testEngine) ICE(context.Context, string, *webrtc.ICECandidateInit) error { return nil }
func (*testEngine) Unpublish(context.Context, string, string) error             { return nil }
func (e *testEngine) Leave(_ context.Context, id string) error {
	e.mu.Lock()
	delete(e.peers, id)
	e.mu.Unlock()
	return nil
}
func (e *testEngine) LeaveConnection(ctx context.Context, id string) {
	for peer, b := range e.copy() {
		if b.ConnectionID == id {
			_ = e.Leave(ctx, peer)
			return
		}
	}
}
func (e *testEngine) PeerBinding(id string) (media.Binding, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, ok := e.peers[id]
	return b, ok
}
func (e *testEngine) copy() map[string]media.Binding {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := make(map[string]media.Binding)
	for k, b := range e.peers {
		v[k] = b
	}
	return v
}
func (e *testEngine) Bindings() []media.Binding {
	v := []media.Binding{}
	for _, b := range e.copy() {
		v = append(v, b)
	}
	return v
}
func (*testEngine) Tracks(string) []media.Track { return []media.Track{} }
func (e *testEngine) CloseConference(ctx context.Context, id string) error {
	for peer, b := range e.copy() {
		if b.ConferenceID == id {
			_ = e.Leave(ctx, peer)
		}
	}
	return nil
}
func (e *testEngine) Shutdown(ctx context.Context) error {
	for id := range e.copy() {
		_ = e.Leave(ctx, id)
	}
	return nil
}

type testRegistry struct {
	mu           sync.Mutex
	route        media.Route
	lost         bool
	released     int
	registerErr  error
	renewBlock   <-chan struct{}
	renewStarted chan struct{}
}

func (r *testRegistry) RegisterWorker(context.Context, media.Worker, time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registerErr
}
func (r *testRegistry) GetOwner(context.Context, string) (media.Route, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lost {
		return media.Route{}, media.ErrOwnership
	}
	return r.route, nil
}
func (r *testRegistry) Renew(ctx context.Context, _ string, _ media.Route, _ time.Duration) error {
	r.mu.Lock()
	lost, block, started := r.lost, r.renewBlock, r.renewStarted
	r.mu.Unlock()
	if block != nil {
		if started != nil {
			select {
			case started <- struct{}{}:
			default:
			}
		}
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if lost {
		return media.ErrOwnership
	}
	return nil
}
func (r *testRegistry) Release(context.Context, string, media.Route) error {
	r.mu.Lock()
	r.released++
	r.mu.Unlock()
	return nil
}
func (*testRegistry) RemoveWorker(context.Context, string, string) error { return nil }

type testSessions struct {
	mu      sync.Mutex
	session realtime.Session
	missing bool
	err     error
}

func (s *testSessions) Get(context.Context, string) (realtime.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.missing {
		return realtime.Session{}, media.ErrUnauthorized
	}
	if s.err != nil {
		return realtime.Session{}, s.err
	}
	return s.session, nil
}

type fixture struct {
	h        *Handler
	cfg      config.MediaConfig
	engine   *testEngine
	registry *testRegistry
	sessions *testSessions
	tickets  *security.MediaTickets
	cmd      media.Command
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	b := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString(), AuthorizationExpiresAt: time.Now().UTC().Add(time.Hour)}
	r := media.Route{WorkerID: "test-worker", Endpoint: "http://worker:8091", LeaseID: uuid.NewString()}
	f := &fixture{cfg: config.MediaConfig{WorkerID: r.WorkerID, WorkerInternalURL: r.Endpoint, InternalSecret: strings.Repeat("i", 32), OperationTimeout: time.Second, WorkerTTL: time.Second, OwnershipTTL: time.Second, HeartbeatInterval: 20 * time.Millisecond, SessionCheckInterval: 20 * time.Millisecond, MaxPeers: 10, ICE: realtime.ICEConfig{ICEServers: []realtime.ICEServer{}}}, engine: &testEngine{peers: make(map[string]media.Binding)}, registry: &testRegistry{route: r}, sessions: &testSessions{session: realtime.Session{ID: b.SessionID, ConferenceID: b.ConferenceID, ParticipantID: b.ParticipantID, ConnectionID: b.ConnectionID, UserID: b.UserID, Status: "connected"}}}
	f.tickets, _ = security.NewMediaTickets(strings.Repeat("t", 32), 45*time.Second)
	f.cfg.VideoMaxWidth = 1280
	f.cfg.VideoMaxHeight = 720
	f.cfg.VideoMaxFPS = 30
	ticket, err := f.tickets.Issue(b, r)
	if err != nil {
		t.Fatal(err)
	}
	f.cmd = media.Command{RequestID: uuid.NewString(), Binding: b, Route: r, Ticket: ticket}
	f.h = NewHandler(f.cfg, f.registry, f.sessions, f.tickets, f.engine, func() any { return map[string]int{"peers": len(f.engine.Bindings())} }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done, err := f.h.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); <-done; _ = f.h.Stop(context.Background()) })
	return f
}
func (f *fixture) request(op string, cmd media.Command, secret string) (int, media.Result) {
	raw, _ := json.Marshal(cmd)
	req := httptest.NewRequest("POST", "/internal/media/"+op, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("X-Request-ID", cmd.RequestID)
	w := httptest.NewRecorder()
	f.h.Routes().ServeHTTP(w, req)
	var result media.Result
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result
}
func (f *fixture) join(t *testing.T) string {
	t.Helper()
	status, result := f.request("join", f.cmd, f.cfg.InternalSecret)
	if status != 200 || !validUUID(result.MediaPeerID) {
		t.Fatalf("join status=%d", status)
	}
	return result.MediaPeerID
}

func TestInternalMediaAuthorizationAndBinding(t *testing.T) {
	f := newFixture(t)
	if status, _ := f.request("join", f.cmd, "wrong"); status != 401 {
		t.Fatal(status)
	}
	bad := f.cmd
	bad.Ticket = "invalid"
	if status, _ := f.request("join", bad, f.cfg.InternalSecret); status != 401 {
		t.Fatal(status)
	}
	bad = f.cmd
	bad.Binding.ConferenceID = uuid.NewString()
	if status, _ := f.request("join", bad, f.cfg.InternalSecret); status != 401 {
		t.Fatal("cross-conference ticket accepted", status)
	}
	bad = f.cmd
	bad.Route.LeaseID = uuid.NewString()
	if status, _ := f.request("join", bad, f.cfg.InternalSecret); status != 409 {
		t.Fatal("stale lease accepted", status)
	}
	id := f.join(t)
	if second := f.join(t); second != id {
		t.Fatal("duplicate join created another endpoint")
	}
	bad = f.cmd
	bad.MediaPeerID = id
	bad.Ticket = ""
	bad.Binding.ParticipantID = uuid.NewString()
	if status, _ := f.request("leave", bad, f.cfg.InternalSecret); status != 401 {
		t.Fatal("foreign peer cleanup accepted", status)
	}
}

func TestMediaJoinPropagatesConfiguredCaptureTarget(t *testing.T) {
	f := newFixture(t)
	// Handler already copied config at construction; mutate only a fresh
	// handler with no lifecycle workers so this test is race-free.
	cfg := f.cfg
	cfg.VideoMaxWidth, cfg.VideoMaxHeight, cfg.VideoMaxFPS = 640, 360, 15
	h := NewHandler(cfg, f.registry, f.sessions, f.tickets, f.engine, func() any { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.mu.Lock()
	h.workerDeadline = time.Now().Add(time.Hour)
	h.ready.Store(true)
	h.mu.Unlock()
	previous := f.h
	f.h = h
	status, result := f.request("join", f.cmd, f.cfg.InternalSecret)
	f.h = previous
	if status != 200 || result.VideoCapture.MaxWidth != 640 || result.VideoCapture.MaxHeight != 360 || result.VideoCapture.MaxFrameRate != 15 {
		t.Fatal("worker join lost configured video limits", status, result.VideoCapture)
	}
}

func TestRoomGateCancellationAndReferenceCleanup(t *testing.T) {
	f := newFixture(t)
	unlock, err := f.h.lockRoom(context.Background(), "room-gate-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if second, err := f.h.lockRoom(ctx, "room-gate-test"); err == nil {
		second()
		t.Fatal("room waiter ignored cancellation")
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("room lock held handler beyond operation deadline")
	}
	unlock()
	f.h.mu.Lock()
	_, retained := f.h.roomGates["room-gate-test"]
	f.h.mu.Unlock()
	if retained {
		t.Fatal("cancelled room waiter leaked gate reference")
	}
}

func TestMediaUUIDAndUTF8StrictValidation(t *testing.T) {
	if validUUID(uuid.Nil.String()) || validUUID("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA") || validUUID("not-uuid") {
		t.Fatal("noncanonical/nil UUID accepted")
	}
	if strictJSON([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}, new(any)) == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestLocalLeaseWatchdogFencesWhileRenewalIsBlocked(t *testing.T) {
	f := newFixture(t)
	f.join(t)
	block := make(chan struct{})
	started := make(chan struct{}, 1)
	f.registry.mu.Lock()
	f.registry.renewBlock = block
	f.registry.renewStarted = started
	f.registry.mu.Unlock()
	f.h.mu.Lock()
	lease := f.h.leases[f.cmd.Binding.ConferenceID]
	lease.deadline = time.Now().Add(70 * time.Millisecond)
	f.h.leases[f.cmd.Binding.ConferenceID] = lease
	f.h.mu.Unlock()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not enter blocked renew")
	}
	deadline := time.Now().Add(600 * time.Millisecond)
	for len(f.engine.Bindings()) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(f.engine.Bindings()) != 0 {
		close(block)
		t.Fatal("blocked renewal postponed monotonic lease fencing")
	}
	if f.h.ready.Load() {
		close(block)
		t.Fatal("fenced worker remained ready")
	}
	close(block)
}

func TestMediaSessionBrokerFailureClosesEveryOwnedRoom(t *testing.T) {
	f := newFixture(t)
	f.join(t)
	other := f.cmd.Binding
	other.ConferenceID = uuid.NewString()
	other.ConnectionID = uuid.NewString()
	other.SessionID = uuid.NewString()
	if _, err := f.engine.Join(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	f.sessions.mu.Lock()
	f.sessions.err = errors.New("broker unreachable")
	f.sessions.mu.Unlock()
	f.registry.mu.Lock()
	f.registry.registerErr = errors.New("broker unreachable")
	f.registry.mu.Unlock()
	f.h.sweep(context.Background())
	if len(f.engine.Bindings()) != 0 || f.h.ready.Load() {
		t.Fatal("Redis failure did not fence all rooms")
	}
}

type deadlineSessions struct{ calls atomic.Int32 }

func (s *deadlineSessions) Get(ctx context.Context, _ string) (realtime.Session, error) {
	s.calls.Add(1)
	<-ctx.Done()
	return realtime.Session{}, ctx.Err()
}

func TestMediaSessionSweepUsesOneGlobalBudget(t *testing.T) {
	base := newFixture(t)
	engine := &testEngine{peers: make(map[string]media.Binding)}
	for i := 0; i < 20; i++ {
		b := base.cmd.Binding
		b.ConnectionID = uuid.NewString()
		b.SessionID = uuid.NewString()
		if _, err := engine.Join(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}
	sessions := &deadlineSessions{}
	cfg := base.cfg
	cfg.OperationTimeout = 35 * time.Millisecond
	h := NewHandler(cfg, base.registry, sessions, base.tickets, engine, func() any { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.mu.Lock()
	h.workerDeadline = time.Now().Add(time.Hour)
	h.ready.Store(true)
	h.mu.Unlock()
	started := time.Now()
	h.sweep(context.Background())
	if time.Since(started) > 300*time.Millisecond || sessions.calls.Load() != 1 || len(engine.Bindings()) != 0 {
		t.Fatal("sweep multiplied timeouts by peer count")
	}
}

func TestInternalMediaOfferIdempotenceAndBounds(t *testing.T) {
	f := newFixture(t)
	id := f.join(t)
	cmd := f.cmd
	cmd.MediaPeerID = id
	cmd.Ticket = ""
	cmd.NegotiationID = uuid.NewString()
	cmd.SDP = "offer"
	for i := 0; i < 2; i++ {
		status, result := f.request("offer", cmd, f.cfg.InternalSecret)
		if status != 200 || result.SDP != "answer" {
			t.Fatal(status)
		}
	}
	f.engine.mu.Lock()
	offers := f.engine.offers
	f.engine.mu.Unlock()
	if offers != 1 {
		t.Fatal("duplicate offer was renegotiated", offers)
	}
	ready := cmd
	ready.SDP = ""
	if status, _ := f.request("ready", ready, f.cfg.InternalSecret); status != 200 {
		t.Fatal("answer acknowledgement rejected", status)
	}
	ready.NegotiationID = uuid.NewString()
	if status, _ := f.request("ready", ready, f.cfg.InternalSecret); status != 409 {
		t.Fatal("stale acknowledgement accepted", status)
	}
	cmd.SDP = "changed"
	if status, _ := f.request("offer", cmd, f.cfg.InternalSecret); status != 409 {
		t.Fatal(status)
	}
	cmd.SDP = strings.Repeat("x", 49153)
	if status, _ := f.request("offer", cmd, f.cfg.InternalSecret); status != 400 {
		t.Fatal(status)
	}
	cmd.SDP = ""
	cmd.Candidate = json.RawMessage("null")
	if status, _ := f.request("ice", cmd, f.cfg.InternalSecret); status != 200 {
		t.Fatal("end-of-candidates rejected", status)
	}
	cmd.Candidate = json.RawMessage(`{"candidate":42}`)
	if status, _ := f.request("ice", cmd, f.cfg.InternalSecret); status != 400 {
		t.Fatal("invalid ICE accepted", status)
	}
	req := httptest.NewRequest("POST", "/internal/media/join", strings.NewReader(`{"untrusted":true}`))
	req.Header.Set("Authorization", "Bearer "+f.cfg.InternalSecret)
	w := httptest.NewRecorder()
	f.h.Routes().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	unauth := httptest.NewRecorder()
	f.h.Routes().ServeHTTP(unauth, httptest.NewRequest("GET", "/internal/media/metrics", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatal("metrics are not protected")
	}
}

func TestWorkerLostLeaseAndOrphanCleanup(t *testing.T) {
	for _, lostLease := range []bool{true, false} {
		t.Run(map[bool]string{true: "lease", false: "session"}[lostLease], func(t *testing.T) {
			f := newFixture(t)
			_ = f.join(t)
			if lostLease {
				f.registry.mu.Lock()
				f.registry.lost = true
				f.registry.mu.Unlock()
			} else {
				f.sessions.mu.Lock()
				f.sessions.missing = true
				f.sessions.mu.Unlock()
			}
			deadline := time.Now().Add(time.Second)
			for len(f.engine.Bindings()) > 0 {
				if time.Now().After(deadline) {
					t.Fatal("orphan endpoint remained active")
				}
				time.Sleep(5 * time.Millisecond)
			}
			f.h.mu.Lock()
			gates := len(f.h.roomGates)
			f.h.mu.Unlock()
			if gates > 1 {
				t.Fatal("room gate leak")
			}
		})
	}
}

func TestLeaveDoesNotRequireLiveSession(t *testing.T) {
	f := newFixture(t)
	id := f.join(t)
	f.sessions.mu.Lock()
	f.sessions.missing = true
	f.sessions.mu.Unlock()
	cmd := f.cmd
	cmd.MediaPeerID = id
	for i := 0; i < 2; i++ {
		if status, _ := f.request("leave", cmd, f.cfg.InternalSecret); status != 200 {
			t.Fatal("cleanup not idempotent", status)
		}
	}
	if len(f.engine.Bindings()) != 0 {
		t.Fatal("endpoint not removed")
	}
}
