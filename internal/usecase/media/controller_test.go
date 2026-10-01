package media

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type testRegistry struct {
	route domain.Route
	err   error
}

func (r *testRegistry) Workers(context.Context) ([]domain.Worker, error) {
	return []domain.Worker{{ID: r.route.WorkerID, Endpoint: r.route.Endpoint}}, r.err
}
func (r *testRegistry) Claim(context.Context, string, string, time.Duration) (domain.Route, error) {
	return r.route, r.err
}
func (r *testRegistry) GetOwner(context.Context, string) (domain.Route, error) { return r.route, r.err }

type testTickets struct {
	binding domain.Binding
	route   domain.Route
}

func (t *testTickets) Issue(b domain.Binding, r domain.Route) (string, error) {
	t.binding = b
	t.route = r
	return "test-only-ticket", nil
}

type testSessions struct{ err error }

func (s *testSessions) ValidateSession(context.Context, realtime.Session) error { return s.err }

type testPublisher struct {
	events []realtime.Bus
	err    error
}

func (p *testPublisher) Publish(_ context.Context, bus realtime.Bus) error {
	p.events = append(p.events, bus)
	return p.err
}

type testTransport struct {
	mu           sync.Mutex
	commands     []domain.Command
	actions      []string
	peerID       string
	workerID     string
	err          error
	videoCapture *domain.VideoCaptureTarget
}

func (t *testTransport) Call(_ context.Context, action string, c domain.Command) (domain.Result, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.actions = append(t.actions, action)
	t.commands = append(t.commands, c)
	video := domain.VideoCaptureTarget{MaxWidth: 1280, MaxHeight: 720, MaxFrameRate: 30}
	if t.videoCapture != nil {
		video = *t.videoCapture
	}
	return domain.Result{MediaPeerID: t.peerID, WorkerID: t.workerID, MaxPeers: 10, SDP: "server-answer", VideoCapture: video}, t.err
}

type controllerFixture struct {
	controller *Controller
	registry   *testRegistry
	tickets    *testTickets
	sessions   *testSessions
	publisher  *testPublisher
	transport  *testTransport
	session    realtime.Session
	expiry     time.Time
}

func newControllerFixture() *controllerFixture {
	f := &controllerFixture{registry: &testRegistry{route: domain.Route{WorkerID: "test-worker", Endpoint: "http://worker:8091", LeaseID: uuid.NewString()}}, tickets: &testTickets{}, sessions: &testSessions{}, publisher: &testPublisher{}, transport: &testTransport{peerID: uuid.NewString(), workerID: "test-worker"}, expiry: time.Now().UTC().Add(time.Hour)}
	f.session = realtime.Session{ID: uuid.NewString(), ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString()}
	f.controller = NewController(f.registry, f.tickets, f.transport, f.sessions, f.publisher, config.MediaConfig{OperationTimeout: time.Second, OwnershipTTL: 20 * time.Second}, config.RealtimeConfig{SDPBytes: 49152, ICEBytes: 4096})
	return f
}
func (f *controllerFixture) event(kind string, data any) realtime.Envelope {
	return realtime.Event(kind, f.session.ConferenceID, data)
}
func (f *controllerFixture) join(t *testing.T) {
	t.Helper()
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.join", map[string]any{})); err != nil {
		t.Fatal(err)
	}
}

func TestControllerTrustedIdentityAndReplies(t *testing.T) {
	f := newControllerFixture()
	join := f.event("media.join", map[string]any{})
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, join); err != nil {
		t.Fatal(err)
	}
	if f.tickets.binding.ConferenceID != f.session.ConferenceID || f.tickets.binding.ParticipantID != f.session.ParticipantID || f.tickets.binding.SessionID != f.session.ID || f.tickets.binding.ConnectionID != f.session.ConnectionID || !f.tickets.binding.AuthorizationExpiresAt.Equal(f.expiry) {
		t.Fatal("media identity did not bind authenticated session")
	}
	bus := f.publisher.events[len(f.publisher.events)-1]
	if bus.ConnectionID != f.session.ConnectionID || bus.ConferenceID != f.session.ConferenceID || bus.Event.Type != "media.joined" || bus.Event.ReplyTo != join.ID {
		t.Fatal("joined response routing/correlation wrong")
	}
	var joined map[string]json.RawMessage
	if json.Unmarshal(bus.Event.Data, &joined) != nil || string(joined["tracks"]) != "[]" || string(joined["iceServers"]) != "[]" {
		t.Fatal("joined must explicitly contain empty arrays")
	}
	var target domain.VideoCaptureTarget
	if json.Unmarshal(joined["videoCapture"], &target) != nil || target.MaxWidth != 1280 || target.MaxHeight != 720 || target.MaxFrameRate != 30 {
		t.Fatal("joined lost configured video capture target")
	}
	offer := f.event("media.offer", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString(), SDP: "browser-offer"})
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, offer); err != nil {
		t.Fatal(err)
	}
	answer := f.publisher.events[len(f.publisher.events)-1].Event
	var payload domain.Signal
	_ = json.Unmarshal(answer.Data, &payload)
	if answer.Type != "media.answer" || answer.ReplyTo != offer.ID || payload.SDP != "server-answer" || payload.MediaPeerID != f.transport.peerID {
		t.Fatal("answer routing/correlation wrong")
	}
}

func TestControllerRejectsSpoofingUnsupportedMessagesAndLeaseLoss(t *testing.T) {
	f := newControllerFixture()
	f.join(t)
	cases := []struct {
		kind string
		data any
	}{
		{"media.join", map[string]string{"participantId": uuid.NewString()}},
		{"media.offer", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: "bad", SDP: "x"}},
		{"media.offer", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString(), SDP: ""}},
		{"media.ice", domain.Signal{MediaPeerID: f.transport.peerID, Candidate: json.RawMessage(`{"candidate":"x","unexpected":true}`)}},
		{"media.ice", domain.Signal{MediaPeerID: f.transport.peerID, Candidate: json.RawMessage(`"string"`)}},
		{"media.answer", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString(), SDP: "x"}},
	}
	before := len(f.transport.actions)
	for _, tc := range cases {
		if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event(tc.kind, tc.data)); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%s accepted malformed command: %v", tc.kind, err)
		}
	}
	if len(f.transport.actions) != before {
		t.Fatal("invalid command reached worker")
	}
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.leave", domain.Signal{MediaPeerID: uuid.NewString()})); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("foreign media peer accepted")
	}
	f.registry.route.LeaseID = uuid.NewString()
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.leave", domain.Signal{MediaPeerID: f.transport.peerID})); !errors.Is(err, domain.ErrOwnership) {
		t.Fatal("stale ownership accepted")
	}
}

func TestControllerICEEndOfCandidatesLeaveAndIdempotentCleanup(t *testing.T) {
	f := newControllerFixture()
	f.join(t)
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.ice", domain.Signal{MediaPeerID: f.transport.peerID, Candidate: json.RawMessage("null")})); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		f.controller.Disconnected(context.Background(), f.session)
	}
	if len(f.transport.actions) != 3 || f.transport.actions[2] != "leave" {
		t.Fatal("disconnect cleanup was not idempotent")
	}
	if len(f.controller.peers) != 0 {
		t.Fatal("controller retained peer")
	}
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.leave", domain.Signal{MediaPeerID: f.transport.peerID})); err != nil {
		t.Fatal("repeated leave must remain idempotent", err)
	}
}

func TestControllerSessionBoundJoinCancellation(t *testing.T) {
	f := newControllerFixture()
	f.join(t)
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.leave", map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if len(f.controller.peers) != 0 || len(f.transport.actions) != 2 || f.transport.actions[1] != "leave" {
		t.Fatal("session-bound cancellation retained media peer")
	}
	if f.transport.commands[1].MediaPeerID != f.transport.peerID {
		t.Fatal("cancellation did not bind the current media peer")
	}
}

func TestControllerReadyHasStrictBindingAndNegotiationCorrelation(t *testing.T) {
	f := newControllerFixture()
	f.join(t)
	ready := f.event("media.ready", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString()})
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, ready); err != nil {
		t.Fatal(err)
	}
	last := f.transport.commands[len(f.transport.commands)-1]
	if f.transport.actions[len(f.transport.actions)-1] != "ready" || last.NegotiationID == "" || last.RequestID != ready.ID {
		t.Fatal("ready operation lost negotiation correlation")
	}
	ack := f.publisher.events[len(f.publisher.events)-1].Event
	if ack.Type != "ack" || ack.ReplyTo != ready.ID {
		t.Fatal("ready acknowledgement lost command correlation")
	}
	bad := f.event("media.ready", domain.Signal{MediaPeerID: f.transport.peerID, NegotiationID: uuid.NewString(), SDP: "not allowed"})
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, bad); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("ready accepted extraneous SDP")
	}
}

func TestControllerPublishFailureCleansPreparedMedia(t *testing.T) {
	f := newControllerFixture()
	f.publisher.err = errors.New("redis unavailable")
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.join", map[string]any{})); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("failed publish should be reported")
	}
	if len(f.transport.actions) != 2 || f.transport.actions[1] != "leave" || len(f.controller.peers) != 0 {
		t.Fatal("undeliverable join created orphan media peer")
	}
}

func TestControllerRequiresLiveAuthorizedSession(t *testing.T) {
	f := newControllerFixture()
	f.sessions.err = errors.New("membership revoked")
	if err := f.controller.Handle(context.Background(), f.session, f.expiry, f.event("media.join", map[string]any{})); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("revoked session accepted")
	}
	f.sessions.err = nil
	if err := f.controller.Handle(context.Background(), f.session, time.Now().Add(-time.Second), f.event("media.join", map[string]any{})); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("expired auth accepted")
	}
	if len(f.transport.actions) != 0 {
		t.Fatal("unauthorized request reached worker")
	}
}

func TestControllerRequiresObjectPayload(t *testing.T) {
	f := newControllerFixture()
	for _, raw := range []string{"null", "[]", `"string"`} {
		event := f.event("media.join", map[string]any{})
		event.Data = json.RawMessage(raw)
		if err := f.controller.Handle(context.Background(), f.session, f.expiry, event); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("non-object media payload accepted", raw, err)
		}
	}
}

func TestControllerValidatesAndPropagatesCustomCaptureTarget(t *testing.T) {
	f := newControllerFixture()
	f.transport.videoCapture = &domain.VideoCaptureTarget{MaxWidth: 640, MaxHeight: 360, MaxFrameRate: 15}
	f.join(t)
	var joined struct {
		VideoCapture domain.VideoCaptureTarget `json:"videoCapture"`
	}
	if json.Unmarshal(f.publisher.events[0].Event.Data, &joined) != nil || joined.VideoCapture != *f.transport.videoCapture {
		t.Fatal("controller replaced custom target with hardcoded dimensions")
	}
	for _, target := range []domain.VideoCaptureTarget{{MaxWidth: 1920, MaxHeight: 720, MaxFrameRate: 30}, {MaxWidth: 1280, MaxHeight: 1080, MaxFrameRate: 30}, {MaxWidth: 1280, MaxHeight: 720, MaxFrameRate: 60}, {}} {
		bad := newControllerFixture()
		bad.transport.videoCapture = &target
		if err := bad.controller.Handle(context.Background(), bad.session, bad.expiry, bad.event("media.join", map[string]any{})); !errors.Is(err, domain.ErrUnavailable) {
			t.Fatal("unsafe worker capture target accepted", target, err)
		}
		if len(bad.transport.actions) != 2 || bad.transport.actions[1] != "leave" || len(bad.controller.peers) != 0 {
			t.Fatal("invalid target orphaned a prepared peer")
		}
	}
}
