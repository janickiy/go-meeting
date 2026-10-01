package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	mediadomain "github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	"github.com/janickiy/go-recorder/internal/infrastructure/sfu"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"github.com/janickiy/go-recorder/internal/transport/mediaworker"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	mediausecase "github.com/janickiy/go-recorder/internal/usecase/media"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

const stageThreeTicketSecret = "stage-three-isolated-media-ticket-secret"
const stageThreeInternalSecret = "stage-three-isolated-internal-http-secret"

// startStageThreeMedia uses the real Redis lease, signed ticket, protected HTTP
// worker and both API WebSocket instances. It never touches the running app DB.
func startStageThreeMedia(t *testing.T, f *stageTwoFixture) (*sfu.Manager, config.MediaConfig) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := httptest.NewUnstartedServer(nil)
	cfg := config.MediaConfig{WorkerID: "stage3-test-worker", WorkerInternalURL: "http://" + worker.Listener.Addr().String(), Namespace: f.config.Namespace + ":media", TicketSecret: stageThreeTicketSecret, InternalSecret: stageThreeInternalSecret, TicketTTL: 45 * time.Second, OperationTimeout: 5 * time.Second, HeartbeatInterval: 200 * time.Millisecond, WorkerTTL: 3 * time.Second, OwnershipTTL: 3 * time.Second, SessionCheckInterval: 150 * time.Millisecond, MaxPeers: 10, MaxRooms: 10, MaxPublishedTracks: 2, MaxAudioTracks: 1, MaxVideoTracks: 1, VideoMaxWidth: 1280, VideoMaxHeight: 720, VideoMaxFPS: 30, ICE: realtime.ICEConfig{ICEServers: []realtime.ICEServer{}}}
	registry := redisinfra.NewMediaRegistry(f.redis, cfg.Namespace)
	tickets, err := security.NewMediaTickets(cfg.TicketSecret, cfg.TicketTTL)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := sfu.NewManager(sfu.Options{WorkerID: cfg.WorkerID, MaxPeers: cfg.MaxPeers, MaxRooms: cfg.MaxRooms, Logger: logger, Emit: func(binding mediadomain.Binding, kind string, data any) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		event := realtime.Event(kind, binding.ConferenceID, data)
		_ = f.store.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event})
	}})
	if err != nil {
		t.Fatal(err)
	}
	workerHandler := mediaworker.NewHandler(cfg, registry, f.store, tickets, engine, func() any { return engine.Snapshot() }, logger)
	ctx, cancel := context.WithCancel(context.Background())
	done, err := workerHandler.Start(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	worker.Config.Handler = workerHandler.Routes()
	worker.Start()
	for i, hub := range f.hubs {
		f.servers[i].Close()
		controller := mediausecase.NewController(registry, tickets, mediausecase.NewHTTPClient(cfg.InternalSecret, cfg.OperationTimeout), hub, f.store, cfg, f.config)
		hub.SetDisconnectObserver(controller)
		router := gin.New()
		router.Use(gin.Recovery())
		auth, authErr := authusecase.NewService(pg.NewUserRepository(f.db), security.PasswordHasher{}, f.tokens)
		if authErr != nil {
			t.Fatal(authErr)
		}
		httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(auth), conferencesapp.NewHandler(f.service), httpmiddleware.Authenticate(f.tokens))
		wstransport.NewHandler(hub, f.tokens, f.store, redisinfra.NewRateLimiter(f.redis), f.config).SetMedia(controller).RegisterRoutes(router)
		f.servers[i] = httptest.NewServer(router)
	}
	t.Cleanup(func() {
		cancel()
		<-done
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = workerHandler.Stop(shutdown)
		worker.Close()
	})
	return engine, cfg
}

type mediaTestPeer struct {
	t            *testing.T
	socket       *testSocket
	pc           *webrtc.PeerConnection
	conferenceID string
	mu           sync.Mutex
	peerID       string
	tracks       []mediadomain.Track
	received     map[string]map[string]int
	closing      bool
	writeMu      sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	actions      chan func()
	errors       chan error
	joined       chan struct{}
	local        []*webrtc.TrackLocalStaticRTP
	senders      []*webrtc.RTPSender
}

func newMediaTestPeer(t *testing.T, f *stageTwoFixture, instance int, token string) *mediaTestPeer {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &mediaTestPeer{t: t, socket: f.connect(t, instance, token, f.conference.ID), pc: pc, conferenceID: f.conference.ID, received: make(map[string]map[string]int), ctx: ctx, cancel: cancel, actions: make(chan func(), 8), errors: make(chan error, 8), joined: make(chan struct{})}
	for _, codec := range []webrtc.RTPCodecCapability{{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2}, {MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}} {
		track, e := webrtc.NewTrackLocalStaticRTP(codec, uuid.NewString(), uuid.NewString())
		if e != nil {
			t.Fatal(e)
		}
		sender, e := pc.AddTrack(track)
		if e != nil {
			t.Fatal(e)
		}
		p.local = append(p.local, track)
		p.senders = append(p.senders, sender)
		p.wg.Add(1)
		go func(sender *webrtc.RTPSender) {
			defer p.wg.Done()
			for {
				if _, _, e := sender.ReadRTCP(); e != nil {
					return
				}
			}
		}(sender)
	}
	for i := 0; i < 9; i++ {
		for _, kind := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeAudio, webrtc.RTPCodecTypeVideo} {
			if _, e := pc.AddTransceiverFromKind(kind, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); e != nil {
				t.Fatal(e)
			}
		}
	}
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		var candidate any
		if c != nil {
			candidate = c.ToJSON()
		}
		p.send("media.ice", map[string]any{"mediaPeerId": p.id(), "candidate": candidate})
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		p.mu.Lock()
		if p.closing {
			p.mu.Unlock()
			return
		}
		p.wg.Add(1)
		p.mu.Unlock()
		go func() {
			defer p.wg.Done()
			for {
				if _, _, e := track.ReadRTP(); e != nil {
					return
				}
				p.mu.Lock()
				if p.received[track.StreamID()] == nil {
					p.received[track.StreamID()] = make(map[string]int)
				}
				p.received[track.StreamID()][track.Kind().String()]++
				p.mu.Unlock()
			}
		}()
	})
	p.wg.Add(2)
	go p.events()
	go p.publish()
	t.Cleanup(p.close)
	p.send("media.join", map[string]any{})
	select {
	case <-p.joined:
	case e := <-p.errors:
		t.Fatal(e)
	case <-time.After(10 * time.Second):
		t.Fatal("media join timeout")
	}
	return p
}
func (p *mediaTestPeer) id() string { p.mu.Lock(); defer p.mu.Unlock(); return p.peerID }
func (p *mediaTestPeer) report(err error) {
	if err == nil {
		return
	}
	select {
	case p.errors <- err:
	default:
	}
}
func (p *mediaTestPeer) send(kind string, data any) {
	if p.ctx.Err() != nil {
		return
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_ = p.socket.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	p.report(p.socket.conn.WriteJSON(realtime.Event(kind, p.conferenceID, data)))
}
func (p *mediaTestPeer) events() {
	defer p.wg.Done()
	pending := ""
	dirty := false
	remoteSet := false
	ice := []webrtc.ICECandidateInit{}
	offer := func() {
		if pending != "" {
			dirty = true
			return
		}
		o, e := p.pc.CreateOffer(nil)
		if e != nil {
			p.report(e)
			return
		}
		if e = p.pc.SetLocalDescription(o); e != nil {
			p.report(e)
			return
		}
		pending = uuid.NewString()
		dirty = false
		p.send("media.offer", map[string]any{"mediaPeerId": p.id(), "negotiationId": pending, "sdp": o.SDP})
	}
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.socket.done:
			return
		case action := <-p.actions:
			action()
			offer()
		case e := <-p.socket.events:
			var data struct {
				MediaPeerID   string                   `json:"mediaPeerId"`
				NegotiationID string                   `json:"negotiationId"`
				SDP           string                   `json:"sdp"`
				Candidate     *webrtc.ICECandidateInit `json:"candidate"`
				Tracks        []mediadomain.Track      `json:"tracks"`
				Code          string                   `json:"code"`
			}
			if json.Unmarshal(e.Data, &data) != nil {
				continue
			}
			if e.Type == "error" {
				p.report(mediadomain.ErrUnavailable)
				continue
			}
			if e.Type == "media.joined" {
				p.mu.Lock()
				p.peerID = data.MediaPeerID
				p.tracks = data.Tracks
				p.mu.Unlock()
				close(p.joined)
				offer()
				continue
			}
			if data.MediaPeerID != p.id() {
				continue
			}
			switch e.Type {
			case "media.answer":
				if data.NegotiationID != pending {
					continue
				}
				if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: data.SDP}); err != nil {
					p.report(err)
					continue
				}
				remoteSet = true
				for _, c := range ice {
					p.report(p.pc.AddICECandidate(c))
				}
				ice = nil
				p.send("media.ready", map[string]any{"mediaPeerId": p.id(), "negotiationId": pending})
				pending = ""
				if dirty {
					offer()
				}
			case "media.ice":
				c := webrtc.ICECandidateInit{}
				if data.Candidate != nil {
					c = *data.Candidate
				}
				if !remoteSet {
					ice = append(ice, c)
				} else {
					p.report(p.pc.AddICECandidate(c))
				}
			case "media.renegotiate":
				offer()
			case "media.tracks":
				p.mu.Lock()
				p.tracks = data.Tracks
				p.mu.Unlock()
			}
		}
	}
}
func (p *mediaTestPeer) publish() {
	defer p.wg.Done()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var sequence uint16
	var audio, video uint32
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			sequence++
			audio += 960
			video += 1800
			for i, track := range p.local {
				timestamp := audio
				payload := []byte{0xf8, 0xff, 0xfe}
				if i == 1 {
					timestamp = video
					payload = []byte{0x10, 0, 0, 0}
				}
				_ = track.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: sequence, Timestamp: timestamp, Marker: true}, Payload: payload})
			}
		}
	}
}
func (p *mediaTestPeer) close() {
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return
	}
	p.closing = true
	p.mu.Unlock()
	p.cancel()
	_ = p.pc.Close()
	_ = p.socket.conn.Close()
	p.wg.Wait()
}
func (p *mediaTestPeer) assertReceived(t *testing.T, publishers ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case e := <-p.errors:
			t.Fatalf("media protocol failed: %v", e)
		default:
		}
		p.mu.Lock()
		ok := true
		for _, id := range publishers {
			if p.received[id]["audio"] < 3 || p.received[id]["video"] < 3 {
				ok = false
			}
		}
		p.mu.Unlock()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("audio/video RTP did not arrive from %v", publishers)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func waitMediaStats(t *testing.T, engine *sfu.Manager, predicate func(sfu.Stats) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		stats := engine.Snapshot()
		if predicate(stats) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("media cleanup/state timed out: %+v", stats)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStageThreeAuthenticatedSFUMedia(t *testing.T) {
	f := stageTwo(t)
	engine, cfg := startStageThreeMedia(t, f)
	a := newMediaTestPeer(t, f, 0, f.ownerToken)
	b := newMediaTestPeer(t, f, 1, f.memberToken)
	a.assertReceived(t, b.id())
	b.assertReceived(t, a.id())
	third := users.User{ID: uuid.NewString(), Email: "third@stage3.example", PasswordHash: f.owner.PasswordHash}
	if _, err := pg.NewUserRepository(f.db).Create(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Join(context.Background(), third.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode}); err != nil {
		t.Fatal(err)
	}
	token, _ := f.tokens.Issue(third.ID)
	c := newMediaTestPeer(t, f, 0, token)
	c.assertReceived(t, a.id(), b.id())
	a.assertReceived(t, b.id(), c.id())
	b.assertReceived(t, a.id(), c.id())
	waitMediaStats(t, engine, func(s sfu.Stats) bool { return s.Peers == 3 && s.Tracks == 6 && s.Subscriptions == 12 })
	// SDP removeTrack must clean publisher and every subscriber, not only UI.
	a.actions <- func() { pErr := a.pc.RemoveTrack(a.senders[1]); a.report(pErr) }
	waitMediaStats(t, engine, func(s sfu.Stats) bool { return s.Tracks == 5 && s.Subscriptions == 10 })
	a.close()
	waitMediaStats(t, engine, func(s sfu.Stats) bool { return s.Peers == 2 && s.Tracks == 4 })
	a = newMediaTestPeer(t, f, 0, f.ownerToken)
	a.assertReceived(t, b.id(), c.id())
	b.assertReceived(t, a.id())
	c.assertReceived(t, a.id())
	oldPeer, oldConnection, participant := b.id(), b.socket.state.ConnectionID, b.socket.state.ParticipantID
	b.close()
	waitMediaStats(t, engine, func(s sfu.Stats) bool { return s.Peers == 2 && s.Tracks == 4 })
	b2 := newMediaTestPeer(t, f, 1, f.memberToken)
	if b2.id() == oldPeer || b2.socket.state.ConnectionID == oldConnection || b2.socket.state.ParticipantID != participant {
		t.Fatal("reconnect replaced membership or reused media endpoint")
	}
	b2.assertReceived(t, a.id(), c.id())
	a.assertReceived(t, b2.id())
	c.assertReceived(t, b2.id())
	// Invalid signed admission and mismatched conference binding are rejected by
	// the real protected HTTP boundary, not a fake signaling transport.
	session, err := f.store.Get(context.Background(), b2.socket.state.ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	route, err := redisinfra.NewMediaRegistry(f.redis, cfg.Namespace).GetOwner(context.Background(), f.conference.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding := mediadomain.Binding{ConferenceID: session.ConferenceID, ParticipantID: session.ParticipantID, SessionID: session.ID, ConnectionID: session.ConnectionID, UserID: session.UserID, AuthorizationExpiresAt: time.Now().Add(time.Hour)}
	client := mediausecase.NewHTTPClient(cfg.InternalSecret, cfg.OperationTimeout)
	if _, err := client.Call(context.Background(), "join", mediadomain.Command{RequestID: uuid.NewString(), Binding: binding, Route: route, Ticket: "invalid"}); err == nil {
		t.Fatal("invalid ticket accepted")
	}
	tickets, _ := security.NewMediaTickets(cfg.TicketSecret, cfg.TicketTTL)
	ticket, _ := tickets.Issue(binding, route)
	binding.ConferenceID = uuid.NewString()
	if _, err := client.Call(context.Background(), "join", mediadomain.Command{RequestID: uuid.NewString(), Binding: binding, Route: route, Ticket: ticket}); err == nil {
		t.Fatal("cross-conference ticket accepted")
	}
	a.close()
	b2.close()
	c.close()
	waitMediaStats(t, engine, func(s sfu.Stats) bool { return s.Rooms == 0 && s.Peers == 0 && s.Tracks == 0 && s.Subscriptions == 0 })
	t.Logf("authenticated API/WS/HTTP/Redis/SFU path: %+v", engine.Snapshot())
}
