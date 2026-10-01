// Package sfu owns media transport only. Business permissions, tickets and
// distributed ownership are validated by the media-worker control boundary.
package sfu

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/interceptor"
	pion "github.com/pion/webrtc/v4"
)

type Options struct {
	WorkerID                                                                           string
	ICE                                                                                []pion.ICEServer
	UDPPort, UDPMinPort, UDPMaxPort, TCPPort                                           int
	NATIPs                                                                             []string
	MaxPeers, MaxRooms, MaxPublishedTracks, MaxAudioTracks, MaxVideoTracks, QueueSize  int
	ICEDisconnectedTimeout, ICEFailedTimeout, ICEKeepaliveInterval, NegotiationTimeout time.Duration
	Logger                                                                             *slog.Logger
	// Emit must have a bounded implementation. It runs without SFU locks from
	// an owned, bounded per-peer event worker, never from the RTP forwarding loop.
	// Lifecycle operations triggered by Emit must be scheduled asynchronously:
	// synchronous Leave would wait for the event worker currently calling Emit.
	Emit func(media.Binding, string, any)
}

type Stats struct {
	Rooms           int    `json:"roomsActive"`
	Peers           int    `json:"mediaPeersActive"`
	Tracks          int    `json:"tracksPublished"`
	Subscriptions   int    `json:"subscriptions"`
	PeerConnections uint64 `json:"peerConnectionTotal"`
	Failures        uint64 `json:"peerConnectionFailures"`
	Packets         uint64 `json:"packetsForwarded"`
	Bytes           uint64 `json:"bytesForwarded"`
	Dropped         uint64 `json:"packetsDropped"`
}

type Manager struct {
	opts                                     Options
	api                                      *pion.API
	mu                                       sync.Mutex
	rooms                                    map[string]*room
	peers                                    map[string]*peer
	connections                              map[string]*peer
	roomClosures                             map[string]chan struct{}
	closing                                  bool
	closed                                   chan struct{}
	shutdownOnce                             sync.Once
	closeWG                                  sync.WaitGroup
	udp                                      net.PacketConn
	tcp                                      net.Listener
	total, failures, packets, bytes, dropped atomic.Uint64
}

type room struct {
	id     string
	mu     sync.Mutex
	peers  map[string]*peer
	tracks map[string]*publishedTrack
}

func defaults(o Options) Options {
	if o.MaxPeers == 0 {
		o.MaxPeers = 10
	}
	if o.MaxRooms == 0 {
		o.MaxRooms = 100
	}
	if o.MaxPublishedTracks == 0 {
		o.MaxPublishedTracks = 2
	}
	if o.MaxAudioTracks == 0 {
		o.MaxAudioTracks = 1
	}
	if o.MaxVideoTracks == 0 {
		o.MaxVideoTracks = 1
	}
	if o.QueueSize == 0 {
		o.QueueSize = 128
	}
	if o.ICEDisconnectedTimeout == 0 {
		o.ICEDisconnectedTimeout = 10 * time.Second
	}
	if o.ICEFailedTimeout == 0 {
		o.ICEFailedTimeout = 20 * time.Second
	}
	if o.ICEKeepaliveInterval == 0 {
		o.ICEKeepaliveInterval = 2 * time.Second
	}
	if o.NegotiationTimeout == 0 {
		o.NegotiationTimeout = 15 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

func NewManager(options Options) (*Manager, error) {
	o := defaults(options)
	if o.MaxPeers < 2 || o.MaxPeers > 100 || o.MaxRooms < 1 || o.MaxRooms > 10000 ||
		o.MaxPublishedTracks < 1 || o.MaxPublishedTracks > 2 || o.MaxAudioTracks != 1 || o.MaxVideoTracks != 1 ||
		o.QueueSize < 1 || o.QueueSize > 4096 || o.NegotiationTimeout < 100*time.Millisecond ||
		o.UDPPort < 0 || o.UDPPort > 65535 || o.TCPPort < 0 || o.TCPPort > 65535 ||
		o.UDPMinPort < 0 || o.UDPMaxPort > 65535 || o.UDPMinPort > o.UDPMaxPort ||
		(o.UDPMinPort == 0) != (o.UDPMaxPort == 0) || (o.UDPPort > 0 && o.UDPMinPort > 0) ||
		o.ICEDisconnectedTimeout <= 0 || o.ICEFailedTimeout <= 0 || o.ICEKeepaliveInterval <= 0 {
		return nil, media.ErrInvalid
	}
	engine := &pion.MediaEngine{}
	if err := engine.RegisterCodec(pion.RTPCodecParameters{RTPCodecCapability: pion.RTPCodecCapability{
		MimeType: pion.MimeTypeOpus, ClockRate: 48000, Channels: 2, SDPFmtpLine: "minptime=10;useinbandfec=1",
	}, PayloadType: 111}, pion.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	feedback := []pion.RTCPFeedback{{Type: "nack"}, {Type: "nack", Parameter: "pli"}, {Type: "ccm", Parameter: "fir"}}
	if err := engine.RegisterCodec(pion.RTPCodecParameters{RTPCodecCapability: pion.RTPCodecCapability{
		MimeType: pion.MimeTypeVP8, ClockRate: 90000, RTCPFeedback: feedback,
	}, PayloadType: 96}, pion.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	registry := &interceptor.Registry{}
	loggerFactory := safePionLoggerFactory{logger: o.Logger}
	if err := pion.RegisterDefaultInterceptorsWithOptions(engine, registry, pion.WithInterceptorLoggerFactory(loggerFactory)); err != nil {
		return nil, err
	}
	settings := pion.SettingEngine{LoggerFactory: loggerFactory}
	settings.SetICETimeouts(o.ICEDisconnectedTimeout, o.ICEFailedTimeout, o.ICEKeepaliveInterval)
	m := &Manager{opts: o, rooms: map[string]*room{}, peers: map[string]*peer{}, connections: map[string]*peer{}, roomClosures: map[string]chan struct{}{}, closed: make(chan struct{})}
	cleanup := func() {
		if m.udp != nil {
			_ = m.udp.Close()
		}
		if m.tcp != nil {
			_ = m.tcp.Close()
		}
	}
	networks := []pion.NetworkType{pion.NetworkTypeUDP4}
	if o.UDPPort > 0 {
		conn, err := net.ListenPacket("udp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(o.UDPPort)))
		if err != nil {
			return nil, err
		}
		m.udp = conn
		settings.SetICEUDPMux(pion.NewICEUDPMux(loggerFactory.NewLogger("ice"), conn))
	} else if o.UDPMinPort > 0 {
		if err := settings.SetEphemeralUDPPortRange(uint16(o.UDPMinPort), uint16(o.UDPMaxPort)); err != nil {
			return nil, err
		}
	}
	if o.TCPPort > 0 {
		listener, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(o.TCPPort)))
		if err != nil {
			cleanup()
			return nil, err
		}
		m.tcp = listener
		settings.SetICETCPMux(pion.NewICETCPMux(loggerFactory.NewLogger("ice"), listener, 8))
		networks = append(networks, pion.NetworkTypeTCP4)
	}
	settings.SetNetworkTypes(networks)
	if len(o.NATIPs) > 0 {
		for _, ip := range o.NATIPs {
			if net.ParseIP(ip) == nil {
				cleanup()
				return nil, media.ErrInvalid
			}
		}
		settings.SetNAT1To1IPs(o.NATIPs, pion.ICECandidateTypeHost)
	}
	m.api = pion.NewAPI(pion.WithMediaEngine(engine), pion.WithInterceptorRegistry(registry), pion.WithSettingEngine(settings))
	// The sentinel keeps WaitGroup additions safe until Shutdown has detached
	// every registered peer; a peer is added to closeWG before it leaves the map.
	m.closeWG.Add(1)
	return m, nil
}

func (m *Manager) Join(ctx context.Context, binding media.Binding) (media.PeerView, error) {
	for _, id := range []string{binding.ConferenceID, binding.ParticipantID, binding.SessionID, binding.ConnectionID} {
		if parsed, err := uuid.Parse(id); err != nil || parsed == uuid.Nil {
			return media.PeerView{}, media.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return media.PeerView{}, err
	}
	m.mu.Lock()
	if m.closing || m.roomClosures[binding.ConferenceID] != nil {
		m.mu.Unlock()
		return media.PeerView{}, media.ErrUnavailable
	}
	if old := m.connections[binding.ConnectionID]; old != nil {
		m.mu.Unlock()
		if sameBinding(old.binding, binding) {
			return old.view(), nil
		}
		return media.PeerView{}, media.ErrUnauthorized
	}
	// Construction happens outside the global lock, but Shutdown must still
	// wait for an in-flight NewPeerConnection to be either registered or closed.
	m.closeWG.Add(1)
	m.mu.Unlock()
	defer m.closeWG.Done()
	pc, err := m.api.NewPeerConnection(pion.Configuration{ICEServers: m.opts.ICE})
	if err != nil {
		return media.PeerView{}, media.ErrUnavailable
	}
	ctxPeer, cancel := context.WithCancel(context.Background())
	p := &peer{id: uuid.NewString(), binding: binding, manager: m, pc: pc, ctx: ctxPeer, cancel: cancel,
		publications: map[string]*publishedTrack{}, subscriptions: map[string]*subscription{},
		events: make(chan outboundEvent, 128), notify: make(chan struct{}, 1)}
	m.mu.Lock()
	if old := m.connections[binding.ConnectionID]; old != nil {
		m.mu.Unlock()
		cancel()
		_ = pc.Close()
		if sameBinding(old.binding, binding) {
			return old.view(), nil
		}
		return media.PeerView{}, media.ErrUnauthorized
	}
	if m.closing || ctx.Err() != nil || m.roomClosures[binding.ConferenceID] != nil {
		m.mu.Unlock()
		cancel()
		_ = pc.Close()
		return media.PeerView{}, media.ErrUnavailable
	}
	r := m.rooms[binding.ConferenceID]
	if r == nil {
		if len(m.rooms) >= m.opts.MaxRooms {
			m.mu.Unlock()
			cancel()
			_ = pc.Close()
			return media.PeerView{}, media.ErrLimit
		}
		r = &room{id: binding.ConferenceID, peers: map[string]*peer{}, tracks: map[string]*publishedTrack{}}
		m.rooms[r.id] = r
	}
	r.mu.Lock()
	if len(r.peers) >= m.opts.MaxPeers {
		r.mu.Unlock()
		m.mu.Unlock()
		cancel()
		_ = pc.Close()
		return media.PeerView{}, media.ErrLimit
	}
	p.room = r
	r.peers[p.id] = p
	r.mu.Unlock()
	m.peers[p.id] = p
	m.connections[binding.ConnectionID] = p
	m.total.Add(1)
	// Set handlers/start workers while registered and protected from Shutdown.
	p.install()
	m.mu.Unlock()
	m.syncPeer(p)
	m.log(p, "media_join", nil)
	return p.view(), nil
}

func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID
}

func (m *Manager) get(id string) (*peer, error) {
	m.mu.Lock()
	p := m.peers[id]
	m.mu.Unlock()
	if p == nil {
		return nil, media.ErrPeerNotFound
	}
	return p, nil
}

func (m *Manager) PeerBinding(id string) (media.Binding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.peers[id]
	if p == nil {
		return media.Binding{}, false
	}
	return p.binding, true
}

func (m *Manager) Bindings() []media.Binding {
	m.mu.Lock()
	defer m.mu.Unlock()
	bindings := make([]media.Binding, 0, len(m.peers))
	for _, p := range m.peers {
		bindings = append(bindings, p.binding)
	}
	return bindings
}

func (m *Manager) Leave(_ context.Context, id string) error {
	m.mu.Lock()
	p := m.peers[id]
	if p == nil {
		m.mu.Unlock()
		return nil
	}
	m.detachLocked(p)
	m.mu.Unlock()
	m.finishDetached(p)
	return nil
}

// Caller holds manager.mu. Set every lifecycle flag/cancellation before cleanup
// waits: an ownership fence stops all forwarding paths immediately. No Pion or
// network operation occurs under manager/room locks.
func (m *Manager) detachLocked(p *peer) {
	m.closeWG.Add(1)
	p.mu.Lock()
	p.closing = true
	p.cancel()
	delete(m.peers, p.id)
	delete(m.connections, p.binding.ConnectionID)
	p.room.mu.Lock()
	delete(p.room.peers, p.id)
	empty := len(p.room.peers) == 0
	p.room.mu.Unlock()
	p.mu.Unlock()
	if empty {
		delete(m.rooms, p.room.id)
	}
}

func (m *Manager) finishDetached(p *peer) {
	defer m.closeWG.Done()
	p.close()
	m.log(p, "media_leave", nil)
}

func (m *Manager) LeaveConnection(ctx context.Context, connectionID string) {
	m.mu.Lock()
	p := m.connections[connectionID]
	m.mu.Unlock()
	if p != nil {
		_ = m.Leave(ctx, p.id)
	}
}

func (m *Manager) CloseConference(ctx context.Context, id string) error {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return waitClosed(ctx, m.closed)
	}
	if done := m.roomClosures[id]; done != nil {
		m.mu.Unlock()
		return waitClosed(ctx, done)
	}
	peers := []*peer{}
	for _, p := range m.peers {
		if p.binding.ConferenceID == id {
			peers = append(peers, p)
		}
	}
	if len(peers) == 0 {
		m.mu.Unlock()
		return nil
	}
	done := make(chan struct{})
	m.roomClosures[id] = done
	// Track coordinator as well as peer cleanup; Shutdown waits for any
	// conference close that started before its global closing gate.
	m.closeWG.Add(1)
	for _, p := range peers {
		m.detachLocked(p)
	}
	m.mu.Unlock()
	go func() {
		defer m.closeWG.Done()
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go func(p *peer) { defer wg.Done(); m.finishDetached(p) }(p)
		}
		wg.Wait()
		m.mu.Lock()
		delete(m.roomClosures, id)
		m.mu.Unlock()
		close(done)
	}()
	return waitClosed(ctx, done)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do(func() {
		m.mu.Lock()
		m.closing = true
		peers := make([]*peer, 0, len(m.peers))
		for _, p := range m.peers {
			peers = append(peers, p)
		}
		for _, p := range peers {
			m.detachLocked(p)
		}
		m.mu.Unlock()
		go func() {
			for _, p := range peers {
				go m.finishDetached(p)
			}
			m.closeWG.Done()
			m.closeWG.Wait()
			if m.udp != nil {
				_ = m.udp.Close()
			}
			if m.tcp != nil {
				_ = m.tcp.Close()
			}
			close(m.closed)
		}()
	})
	return waitClosed(ctx, m.closed)
}

func waitClosed(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) Snapshot() Stats {
	m.mu.Lock()
	rooms := make([]*room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rooms = append(rooms, r)
	}
	s := Stats{Rooms: len(m.rooms), Peers: len(m.peers)}
	m.mu.Unlock()
	for _, r := range rooms {
		r.mu.Lock()
		sources := make([]*publishedTrack, 0, len(r.tracks))
		for _, t := range r.tracks {
			sources = append(sources, t)
		}
		r.mu.Unlock()
		s.Tracks += len(sources)
		for _, t := range sources {
			t.mu.Lock()
			s.Subscriptions += len(t.subscribers)
			t.mu.Unlock()
		}
	}
	s.PeerConnections = m.total.Load()
	s.Failures = m.failures.Load()
	s.Packets = m.packets.Load()
	s.Bytes = m.bytes.Load()
	s.Dropped = m.dropped.Load()
	return s
}

func (m *Manager) log(p *peer, event string, fields []any) {
	args := []any{"worker_id", m.opts.WorkerID, "conference_id", p.binding.ConferenceID, "participant_id", p.binding.ParticipantID, "session_id", p.binding.SessionID, "connection_id", p.binding.ConnectionID, "media_peer_id", p.id, "event_type", event}
	args = append(args, fields...)
	m.opts.Logger.Info("media event", args...)
}

func supported(track *pion.TrackRemote) bool {
	return (track.Kind() == pion.RTPCodecTypeAudio && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeOpus)) ||
		(track.Kind() == pion.RTPCodecTypeVideo && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeVP8))
}
