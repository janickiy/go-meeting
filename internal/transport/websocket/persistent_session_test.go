package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

const persistentSocketSecret = "isolated-persistent-websocket-test-secret"

// This fixture uses real signed JWTs, an HTTP upgrade, the production Hub, and
// the production socket loops. Only PostgreSQL/Redis and media commands are
// replaced, so expiry and heartbeat tests run without external services.
type persistentSocketVerifier struct {
	*security.TokenService
	mu            sync.Mutex
	userID, sid   string
	authorizeErr  error
	checks        int
	boundedChecks bool
	nextDelay     time.Duration
	delayStarted  chan time.Time
}

func (v *persistentSocketVerifier) VerifyAuthorization(ctx context.Context, raw string) (string, string, string, time.Time, error) {
	id, scope, sid, expiry, err := v.TokenService.VerifyAuthorization(raw)
	if err == nil && sid != "" {
		err = v.AuthorizeSession(ctx, id, sid)
	}
	return id, scope, sid, expiry, err
}

func (v *persistentSocketVerifier) AuthorizeSession(ctx context.Context, userID, sid string) error {
	v.mu.Lock()
	v.checks++
	deadline, ok := ctx.Deadline()
	v.boundedChecks = v.boundedChecks && ok && time.Until(deadline) <= 5*time.Second
	if userID != v.userID || sid != v.sid {
		v.mu.Unlock()
		return apperrors.ErrUnauthorized
	}
	delay, started, err := v.nextDelay, v.delayStarted, v.authorizeErr
	v.nextDelay = 0
	v.mu.Unlock()
	if delay > 0 {
		started <- time.Now()
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (v *persistentSocketVerifier) fail(err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.authorizeErr = err
}

type persistentSocketRepository struct {
	mu          sync.Mutex
	participant conferences.Participant
	opened      []domain.Session
	closed      map[string]bool
}

func (r *persistentSocketRepository) Authorize(_ context.Context, conferenceID, userID string) (conferences.Participant, error) {
	if conferenceID != r.participant.ConferenceID || userID != *r.participant.UserID {
		return conferences.Participant{}, apperrors.ErrForbidden
	}
	return r.participant, nil
}

func (r *persistentSocketRepository) Open(_ context.Context, session domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opened = append(r.opened, session)
	return nil
}

func (r *persistentSocketRepository) Close(_ context.Context, connectionID string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed[connectionID] = true
	return nil
}

func (r *persistentSocketRepository) Stale(context.Context, time.Time, string) ([]domain.Session, error) {
	return nil, nil
}

func (r *persistentSocketRepository) Roster(context.Context, string) (conferences.Status, []conferences.Participant, error) {
	return conferences.Active, []conferences.Participant{r.participant}, nil
}

type persistentSocketLease struct {
	session domain.Session
	until   time.Time
}

type persistentSocketStore struct {
	mu       sync.Mutex
	sessions map[string]persistentSocketLease
	events   chan domain.Bus
	touches  chan time.Time
	closed   chan struct{}
	once     sync.Once
}

func (s *persistentSocketStore) Register(_ context.Context, session domain.Session, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ConnectionID] = persistentSocketLease{session, time.Now().Add(ttl)}
	return nil
}

func (s *persistentSocketStore) Unregister(_ context.Context, connectionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, connectionID)
	return nil
}

func (s *persistentSocketStore) Touch(_ context.Context, connectionID string, seen time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.sessions[connectionID]
	if !ok || !lease.until.After(time.Now()) {
		return apperrors.ErrNotFound
	}
	lease.session.LastSeenAt, lease.until = seen, seen.Add(ttl)
	s.sessions[connectionID] = lease
	select {
	case s.touches <- seen:
	default:
	}
	return nil
}

func (s *persistentSocketStore) Get(_ context.Context, connectionID string) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.sessions[connectionID]
	if !ok || !lease.until.After(time.Now()) {
		return domain.Session{}, apperrors.ErrNotFound
	}
	return lease.session, nil
}

func (s *persistentSocketStore) Missing(ctx context.Context, connectionIDs []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var missing []string
	for _, connectionID := range connectionIDs {
		lease, ok := s.sessions[connectionID]
		if !ok || !lease.until.After(now) {
			missing = append(missing, connectionID)
		}
	}
	return missing, nil
}

func (s *persistentSocketStore) Active(_ context.Context, conferenceID string) ([]domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var active []domain.Session
	for _, lease := range s.sessions {
		if lease.session.ConferenceID == conferenceID && lease.until.After(time.Now()) {
			active = append(active, lease.session)
		}
	}
	return active, nil
}

func (s *persistentSocketStore) Prune(context.Context) error { return nil }

func (s *persistentSocketStore) Publish(ctx context.Context, bus domain.Bus) error {
	select {
	case s.events <- bus:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *persistentSocketStore) Subscribe(context.Context) (domain.Subscription, error) {
	return s, nil
}

func (s *persistentSocketStore) Receive(ctx context.Context) (domain.Bus, error) {
	select {
	case bus := <-s.events:
		return bus, nil
	case <-s.closed:
		return domain.Bus{}, context.Canceled
	case <-ctx.Done():
		return domain.Bus{}, ctx.Err()
	}
}

func (s *persistentSocketStore) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

type persistentSocketTicket struct {
	conferenceID string
	identity     domain.Identity
	until        time.Time
}

type persistentSocketTickets struct {
	mu    sync.Mutex
	items map[string]persistentSocketTicket
}

func (s *persistentSocketTickets) SaveTicket(_ context.Context, ticket, conferenceID string, identity domain.Identity, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[ticket] = persistentSocketTicket{conferenceID, identity, time.Now().Add(ttl)}
	return nil
}

func (s *persistentSocketTickets) ConsumeTicket(_ context.Context, ticket, conferenceID string) (domain.Identity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[ticket]
	delete(s.items, ticket)
	if !ok || item.conferenceID != conferenceID || !item.until.After(time.Now()) || !item.identity.ExpiresAt.After(time.Now()) {
		return domain.Identity{}, apperrors.ErrUnauthorized
	}
	return item.identity, nil
}

type persistentMediaCall struct {
	session domain.Session
	expiry  time.Time
	kind    string
}

type persistentSocketMedia struct{ calls chan persistentMediaCall }

func (m *persistentSocketMedia) Handle(_ context.Context, session domain.Session, expiry time.Time, event domain.Envelope) error {
	m.calls <- persistentMediaCall{session, expiry, event.Type}
	return nil
}

type persistentSocketFixture struct {
	verifier *persistentSocketVerifier
	repo     *persistentSocketRepository
	store    *persistentSocketStore
	tickets  *persistentSocketTickets
	media    *persistentSocketMedia
	hub      *realtimeusecase.Hub
	server   *httptest.Server
	room     string
	userID   string
}

func newPersistentSocketFixture(t *testing.T) *persistentSocketFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &persistentSocketFixture{room: uuid.NewString(), userID: uuid.NewString()}
	tokens, err := security.NewTokenService(persistentSocketSecret)
	if err != nil {
		t.Fatal(err)
	}
	f.verifier = &persistentSocketVerifier{TokenService: tokens, userID: f.userID, sid: uuid.NewString(), boundedChecks: true}
	f.repo = &persistentSocketRepository{participant: conferences.Participant{ID: uuid.NewString(), ConferenceID: f.room, UserID: &f.userID, Role: conferences.ParticipantRole, Status: conferences.Joined, AdmissionState: conferences.AdmissionAdmitted}, closed: map[string]bool{}}
	f.store = &persistentSocketStore{sessions: map[string]persistentSocketLease{}, events: make(chan domain.Bus, 64), touches: make(chan time.Time, 128), closed: make(chan struct{})}
	f.tickets = &persistentSocketTickets{items: map[string]persistentSocketTicket{}}
	f.media = &persistentSocketMedia{calls: make(chan persistentMediaCall, 16)}
	f.hub, err = realtimeusecase.NewHub(f.repo, f.store, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.RealtimeConfig{PingInterval: 50 * time.Millisecond, PongTimeout: 500 * time.Millisecond, WriteTimeout: 300 * time.Millisecond, QueueSize: 64, MessageBytes: 65536, OutboundBytes: 262144, MessagesPerSecond: 100, Burst: 200, TicketTTL: 30 * time.Second}
	router := gin.New()
	NewHandler(f.hub, f.verifier, f.tickets, nil, cfg).SetMedia(f.media).RegisterRoutes(router)
	f.server = httptest.NewServer(router)
	t.Cleanup(func() {
		f.hub.Shutdown()
		f.server.Close()
	})
	return f
}

func (f *persistentSocketFixture) token(t *testing.T, sid, scope string, expiry time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": f.userID, "iss": "go-recorder", "aud": "go-recorder-api", "iat": time.Now().Add(-time.Minute).Unix(), "exp": expiry.Unix()}
	if sid != "" {
		claims["sid"] = sid
	}
	if scope != "" {
		claims["guestConferenceId"] = scope
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(persistentSocketSecret))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (f *persistentSocketFixture) ticket(t *testing.T, token string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/v1/conferences/"+f.room+"/ws-ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct{ Ticket string }
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil || response.StatusCode != 201 || len(body.Ticket) != 43 {
		t.Fatalf("ticket rejected: status=%d err=%v", response.StatusCode, err)
	}
	return body.Ticket
}

type persistentLiveSocket struct {
	conn   *ws.Conn
	state  domain.State
	closed chan error
}

func (f *persistentSocketFixture) connect(t *testing.T, token, ticket string) *persistentLiveSocket {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/api/v1/conferences/" + f.room + "/ws"
	header := http.Header{}
	if ticket != "" {
		url += "?ticket=" + ticket
	} else {
		header.Set("Authorization", "Bearer "+token)
	}
	dialer := ws.Dialer{Subprotocols: []string{"go-recorder.v1"}}
	conn, response, err := dialer.Dial(url, header)
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		t.Fatal(err)
	}
	socket := &persistentLiveSocket{conn: conn, closed: make(chan error, 1)}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var initial domain.Envelope
	if err := conn.ReadJSON(&initial); err != nil || initial.Type != "conference.state" {
		conn.Close()
		t.Fatalf("missing initial state: err=%v type=%s", err, initial.Type)
	}
	if err := json.Unmarshal(initial.Data, &socket.state); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				socket.closed <- err
				return
			}
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return socket
}

func (s *persistentLiveSocket) send(t *testing.T, kind string) {
	t.Helper()
	if err := s.conn.WriteJSON(domain.Event(kind, s.state.Participants[0].ConferenceID, map[string]string{"mediaPeerId": "test-peer"})); err != nil {
		t.Fatal(err)
	}
}

func (s *persistentLiveSocket) expectClose(t *testing.T, reason string) {
	t.Helper()
	select {
	case err := <-s.closed:
		var closeError *ws.CloseError
		if !errors.As(err, &closeError) || closeError.Code != ws.ClosePolicyViolation || closeError.Text != reason {
			t.Fatalf("unexpected close: %v; want 1008 %s", err, reason)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("socket remained authorized; want %s", reason)
	}
}

func receivePersistentMedia(t *testing.T, calls <-chan persistentMediaCall) persistentMediaCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("authorized media command was not forwarded")
		return persistentMediaCall{}
	}
}

func waitPersistentCleanup(t *testing.T, f *persistentSocketFixture, connectionID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.repo.mu.Lock()
		closed := f.repo.closed[connectionID]
		f.repo.mu.Unlock()
		if f.hub.LocalCount() == 0 && closed {
			if _, err := f.store.Get(context.Background(), connectionID); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatal("closed socket retained its media authorization lease")
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("closed socket retained its physical session")
}

func TestPersistentSocketKeepsConnectionAndMediaAfterAccessTokenExpiry(t *testing.T) {
	f := newPersistentSocketFixture(t)
	expiry := time.Now().Truncate(time.Second).Add(2 * time.Second)
	token := f.token(t, f.verifier.sid, "", expiry)
	ticket := f.ticket(t, token)
	f.tickets.mu.Lock()
	identity := f.tickets.items[ticket].identity
	f.tickets.mu.Unlock()
	if identity.AuthSessionID != f.verifier.sid || !identity.ExpiresAt.Equal(expiry) {
		t.Fatal("ticket lost its account session or short access-token expiry")
	}
	socket := f.connect(t, "", ticket)
	socket.send(t, "media.join")
	before := receivePersistentMedia(t, f.media.calls)
	<-time.NewTimer(time.Until(expiry) + 100*time.Millisecond).C
	if _, _, _, _, err := f.verifier.TokenService.VerifyAuthorization(token); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Fatal("test JWT did not really expire")
	}
	socket.send(t, "media.offer")
	after := receivePersistentMedia(t, f.media.calls)
	if before.session.ConnectionID != socket.state.ConnectionID || after.session.ConnectionID != before.session.ConnectionID || after.session.ID != before.session.ID || !before.expiry.Equal(after.expiry) || !after.expiry.After(time.Now().Add(time.Hour)) {
		t.Fatal("silent access-token expiry changed the physical connection or media binding")
	}
	if f.hub.LocalCount() != 1 {
		t.Fatal("persistent socket lost presence after JWT expiry")
	}
	f.repo.mu.Lock()
	opened := len(f.repo.opened)
	f.repo.mu.Unlock()
	if opened != 1 {
		t.Fatal("access-token expiry recreated the physical session")
	}
	f.verifier.mu.Lock()
	checks, bounded := f.verifier.checks, f.verifier.boundedChecks
	f.verifier.mu.Unlock()
	if checks < 5 || !bounded {
		t.Fatal("persistent authorization was not rechecked with bounded contexts")
	}
}

func TestPersistentSocketRevocationClosesConnectionAndRejectsIssuedTicket(t *testing.T) {
	f := newPersistentSocketFixture(t)
	token := f.token(t, f.verifier.sid, "", time.Now().Add(time.Hour))
	unusedTicket := f.ticket(t, token)
	socket := f.connect(t, token, "")
	f.verifier.fail(apperrors.ErrUnauthorized)
	socket.expectClose(t, "authentication_revoked")
	waitPersistentCleanup(t, f, socket.state.ConnectionID)
	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/api/v1/conferences/" + f.room + "/ws?ticket=" + unusedTicket
	conn, response, err := ws.DefaultDialer.Dial(url, nil)
	if conn != nil {
		conn.Close()
	}
	if response != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unconsumed ticket survived logout: response=%v err=%v", response, err)
	}
}

func TestPersistentSocketSlowAuthorizationDoesNotExtendHeartbeatReceipt(t *testing.T) {
	f := newPersistentSocketFixture(t)
	token := f.token(t, f.verifier.sid, "", time.Now().Add(time.Hour))
	_ = f.connect(t, token, "")
	started := make(chan time.Time, 1)
	f.verifier.mu.Lock()
	f.verifier.nextDelay, f.verifier.delayStarted = 120*time.Millisecond, started
	f.verifier.mu.Unlock()
	var receivedAt time.Time
	select {
	case receivedAt = <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not recheck the persistent session")
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		select {
		case confirmed := <-f.store.touches:
			if confirmed.Before(receivedAt.Add(-20 * time.Millisecond)) {
				continue // Ignore a heartbeat completed before the delayed lookup.
			}
			if confirmed.After(receivedAt.Add(40 * time.Millisecond)) {
				t.Fatal("session-store lookup delay was added to the confirmed heartbeat deadline")
			}
			return
		case <-deadline.C:
			t.Fatal("delayed heartbeat did not renew its existing presence lease")
		}
	}
}

func TestPersistentSocketRejectsTicketWithAnotherSessionUser(t *testing.T) {
	f := newPersistentSocketFixture(t)
	token := f.token(t, f.verifier.sid, "", time.Now().Add(time.Hour))
	ticket := f.ticket(t, token)
	f.tickets.mu.Lock()
	item := f.tickets.items[ticket]
	item.identity.UserID = uuid.NewString()
	f.tickets.items[ticket] = item
	f.tickets.mu.Unlock()
	url := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/api/v1/conferences/" + f.room + "/ws?ticket=" + ticket
	conn, response, err := ws.DefaultDialer.Dial(url, nil)
	if conn != nil {
		conn.Close()
	}
	if response != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized || f.hub.LocalCount() != 0 {
		t.Fatalf("ticket crossed account-session identity: response=%v err=%v", response, err)
	}
}

func TestPersistentSocketUnavailableSessionStoreFailsClosedAndCanReconnect(t *testing.T) {
	f := newPersistentSocketFixture(t)
	token := f.token(t, f.verifier.sid, "", time.Now().Add(time.Hour))
	socket := f.connect(t, token, "")
	socket.send(t, "media.join")
	_ = receivePersistentMedia(t, f.media.calls)
	f.verifier.fail(apperrors.ErrUnavailable)
	socket.send(t, "media.offer")
	socket.expectClose(t, "authentication_unavailable")
	waitPersistentCleanup(t, f, socket.state.ConnectionID)
	select {
	case <-f.media.calls:
		t.Fatal("session-store outage bypassed media authorization")
	default:
	}
	f.verifier.fail(nil)
	reconnected := f.connect(t, token, "")
	reconnected.send(t, "media.join")
	call := receivePersistentMedia(t, f.media.calls)
	if call.session.ConnectionID == socket.state.ConnectionID || call.session.UserID != f.userID {
		t.Fatal("recovery lost account identity or reused a revoked physical session")
	}
}

func TestLegacyAndGuestSocketsStillCloseAtAccessTokenExpiry(t *testing.T) {
	for _, guest := range []bool{false, true} {
		name := "legacy account"
		if guest {
			name = "guest"
		}
		t.Run(name, func(t *testing.T) {
			f := newPersistentSocketFixture(t)
			scope := ""
			if guest {
				scope = f.room
			}
			expiry := time.Now().Truncate(time.Second).Add(2 * time.Second)
			token := f.token(t, "", scope, expiry)
			socket := f.connect(t, token, "")
			socket.send(t, "media.join")
			call := receivePersistentMedia(t, f.media.calls)
			if !call.expiry.Equal(expiry) {
				t.Fatal("nonpersistent identity gained indefinite media access")
			}
			socket.expectClose(t, "authentication_expired")
			waitPersistentCleanup(t, f, socket.state.ConnectionID)
		})
	}
}

func TestPersistentVerifierPreservesGuestMeetingScope(t *testing.T) {
	f := newPersistentSocketFixture(t)
	token := f.token(t, "", uuid.NewString(), time.Now().Add(time.Hour))
	req, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/v1/conferences/"+f.room+"/ws-ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("persistent verifier removed guest scope: status=%d", response.StatusCode)
	}
}
