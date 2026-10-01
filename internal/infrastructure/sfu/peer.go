package sfu

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
	pion "github.com/pion/webrtc/v4"
)

type outboundEvent struct {
	kind string
	data any
}

type peer struct {
	id      string
	binding media.Binding
	manager *Manager
	room    *room
	pc      *pion.PeerConnection
	ctx     context.Context
	cancel  context.CancelFunc
	// negotiation serializes ALL AddTrack/RemoveTrack and SDP operations.
	negotiation sync.Mutex
	// offers spans publication cleanup without holding negotiation while
	// cleanup touches other peers' senders, preventing cross-peer deadlocks.
	offers               sync.Mutex
	mu                   sync.Mutex
	closing              bool
	publications         map[string]*publishedTrack
	subscriptions        map[string]*subscription
	events               chan outboundEvent
	notify               chan struct{}
	revision             atomic.Uint64
	wg                   sync.WaitGroup
	closeOnce            sync.Once
	stopRequested        atomic.Bool
	allowedKinds         map[media.Kind]bool
	pendingRemoteICE     []pion.ICECandidateInit // guarded by negotiation
	remoteICECount       int                     // guarded by negotiation; bounded for this PeerConnection
	ready                atomic.Bool
	forwarding           sync.RWMutex    // guards readiness changes against in-flight RTP writes
	pendingSubscriptions []*subscription // exact answered set, guarded by negotiation
	negotiationID        string          // guarded by mu
	pendingReadyAt       time.Time       // guarded by mu
}

func (p *peer) view() media.PeerView {
	return media.PeerView{MediaPeerID: p.id, ConferenceID: p.binding.ConferenceID, ParticipantID: p.binding.ParticipantID, SessionID: p.binding.SessionID, ConnectionID: p.binding.ConnectionID}
}

// start and close use the same lifecycle mutex, so a delayed Pion callback
// cannot add a goroutine after WaitGroup.Wait has started.
func (p *peer) start(fn func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return false
	}
	p.wg.Add(1)
	go func() { defer p.wg.Done(); fn() }()
	return true
}

func (p *peer) install() {
	p.start(p.emitLoop)
	p.start(p.negotiationNotifications)
	p.pc.OnTrack(func(t *pion.TrackRemote, _ *pion.RTPReceiver) {
		p.start(func() { p.manager.publish(p, t) })
	})
	p.pc.OnICECandidate(func(candidate *pion.ICECandidate) {
		var value *pion.ICECandidateInit
		if candidate != nil {
			c := candidate.ToJSON()
			value = &c
		}
		p.emit("media.ice", map[string]any{"mediaPeerId": p.id, "candidate": value})
	})
	p.pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		p.manager.log(p, "peer_connection_state", []any{"state", state.String()})
		p.emit("media.state", map[string]string{"mediaPeerId": p.id, "state": state.String()})
		if state == pion.PeerConnectionStateFailed {
			p.manager.failures.Add(1)
			p.stopAsync()
		}
	})
	p.pc.OnICEConnectionStateChange(func(state pion.ICEConnectionState) {
		p.manager.log(p, "ice_state", []any{"state", state.String()})
	})
	p.start(func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		created := time.Now()
		var disconnectedAt time.Time
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				pendingReadyAt := p.pendingReadyAt
				p.mu.Unlock()
				if !pendingReadyAt.IsZero() && time.Since(pendingReadyAt) >= p.manager.opts.NegotiationTimeout {
					p.stopAsync()
					return
				}
				state := p.pc.ConnectionState()
				if state == pion.PeerConnectionStateConnected {
					disconnectedAt = time.Time{}
					continue
				}
				if state == pion.PeerConnectionStateDisconnected {
					if disconnectedAt.IsZero() {
						disconnectedAt = time.Now()
					}
					if time.Since(disconnectedAt) < p.manager.opts.ICEDisconnectedTimeout {
						continue
					}
				} else if state != pion.PeerConnectionStateFailed && time.Since(created) < p.manager.opts.NegotiationTimeout {
					continue
				}
				p.stopAsync()
				return
			}
		}
	})
}

func (p *peer) emit(kind string, data any) {
	select {
	case <-p.ctx.Done():
		return
	default:
	}
	select {
	case p.events <- outboundEvent{kind, data}:
	default:
		p.stopAsync()
	}
}

func (p *peer) stopAsync() {
	if p.stopRequested.CompareAndSwap(false, true) {
		go func() { _ = p.manager.Leave(context.Background(), p.id) }()
	}
}

func (p *peer) emitLoop() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case event := <-p.events:
			if p.manager.opts.Emit != nil {
				p.manager.opts.Emit(p.binding, event.kind, event.data)
			}
		}
	}
}

func (p *peer) changed() {
	p.revision.Add(1)
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

func (p *peer) tracks() []media.Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	tracks := make([]media.Track, 0, len(p.subscriptions))
	for _, sub := range p.subscriptions {
		tracks = append(tracks, sub.source.metadata)
	}
	return tracks
}

func (p *peer) negotiationNotifications() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.notify:
			// Coalescing is only a hint optimization. Revision is monotonic and every
			// mutation during an in-flight offer leaves another pending notification.
			timer := time.NewTimer(30 * time.Millisecond)
			select {
			case <-p.ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			select {
			case <-p.notify:
			default:
			}
			revision := p.revision.Load()
			p.emit("media.tracks", map[string]any{"mediaPeerId": p.id, "revision": revision, "tracks": p.tracks()})
			p.emit("media.renegotiate", map[string]any{"mediaPeerId": p.id, "revision": revision})
		}
	}
}

func (p *peer) close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closing = true
		pubs := make([]*publishedTrack, 0, len(p.publications))
		for _, t := range p.publications {
			pubs = append(pubs, t)
		}
		subs := make([]*subscription, 0, len(p.subscriptions))
		for _, s := range p.subscriptions {
			subs = append(subs, s)
		}
		p.mu.Unlock()
		p.cancel()
		// Closing transport first unblocks every RTP and RTCP reader. No peer or
		// room lock is held while Pion closes its network transports.
		_ = p.pc.Close()
		for _, t := range pubs {
			p.manager.unpublish(t)
		}
		for _, s := range subs {
			p.manager.unsubscribe(s)
		}
		p.wg.Wait()
	})
}

func (m *Manager) Offer(ctx context.Context, id, negotiationID, raw string) (pion.SessionDescription, error) {
	if parsed, err := uuid.Parse(negotiationID); err != nil || parsed == uuid.Nil {
		return pion.SessionDescription{}, media.ErrInvalid
	}
	p, err := m.get(id)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	active, err := validateOffer(raw, m.opts.MaxPeers)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	if err = ctx.Err(); err != nil {
		return pion.SessionDescription{}, err
	}
	p.offers.Lock()
	defer p.offers.Unlock()
	p.negotiation.Lock()
	p.mu.Lock()
	closed := p.closing
	p.mu.Unlock()
	if closed {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrPeerNotFound
	}
	if p.pc.SignalingState() != pion.SignalingStateStable {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	p.forwarding.Lock()
	p.mu.Lock()
	// An answer write is not proof the recipient applied it. Gate newly bound
	// SSRCs until its correlated Ready arrives after SetRemoteDescription.
	p.ready.Store(false)
	p.pendingSubscriptions = nil
	for _, sub := range p.subscriptions {
		sub.ready.Store(false)
	}
	p.negotiationID = negotiationID
	p.pendingReadyAt = time.Now()
	p.allowedKinds = map[media.Kind]bool{media.KindAudio: active["audio"], media.KindVideo: active["video"]}
	p.mu.Unlock()
	p.forwarding.Unlock()
	if err = p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: raw}); err != nil {
		p.negotiation.Unlock()
		// A failed SRD may have partially changed Pion's signaling/ICE state.
		// Do not retain a transport with an indeterminate negotiation lifecycle.
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrInvalid
	}
	pending := p.pendingRemoteICE
	p.pendingRemoteICE = nil
	for _, candidate := range pending {
		if err = p.pc.AddICECandidate(candidate); err != nil {
			p.negotiation.Unlock()
			p.stopAsync()
			return pion.SessionDescription{}, media.ErrInvalid
		}
	}
	answer, err := p.pc.CreateAnswer(nil)
	if err == nil {
		err = p.pc.SetLocalDescription(answer)
	}
	if err != nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	local := p.pc.LocalDescription()
	if local == nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	result := *local
	declared, parseErr := answeredSSRCs(result.SDP)
	if parseErr != nil {
		p.negotiation.Unlock()
		p.stopAsync()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	p.mu.Lock()
	for _, sub := range p.subscriptions {
		// Sender parameters allocate SSRCs before binding, so they are not
		// negotiation proof. Only a primary SSRC actually declared in an active
		// sending answer section belongs to this applied-answer barrier.
		parameters := sub.sender.GetParameters()
		if len(parameters.Encodings) > 0 && declared[uint32(parameters.Encodings[0].SSRC)] {
			p.pendingSubscriptions = append(p.pendingSubscriptions, sub)
		}
	}
	p.mu.Unlock()
	p.negotiation.Unlock()
	// Reconciliation cannot run holding publisher.negotiation: removing a
	// publication also locks subscribers, and two simultaneous offers must
	// never deadlock A -> B and B -> A.
	p.mu.Lock()
	retire := []*publishedTrack{}
	for _, t := range p.publications {
		if !p.allowedKinds[t.metadata.Kind] {
			retire = append(retire, t)
		}
	}
	p.mu.Unlock()
	for _, t := range retire {
		m.unpublish(t)
	}
	return result, nil
}

// Ready is issued only AFTER the recipient's SetRemoteDescription(answer)
// resolves. This explicit barrier avoids unknown-SSRC probing racing receiver
// creation in Pion/SRTP, and stale acknowledgements cannot release a new offer.
func (m *Manager) Ready(ctx context.Context, id, negotiationID string) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	p.negotiation.Lock()
	p.forwarding.Lock()
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return media.ErrPeerNotFound
	}
	if p.negotiationID != negotiationID || negotiationID == "" || p.pc.SignalingState() != pion.SignalingStateStable {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return media.ErrNegotiation
	}
	if p.ready.Load() {
		p.mu.Unlock()
		p.forwarding.Unlock()
		p.negotiation.Unlock()
		return nil
	}
	p.ready.Store(true)
	p.pendingReadyAt = time.Time{}
	subs := make([]*subscription, 0, len(p.pendingSubscriptions))
	for _, sub := range p.pendingSubscriptions {
		if p.subscriptions[sub.source.metadata.ID] == sub {
			sub.ready.Store(true)
			subs = append(subs, sub)
		}
	}
	p.pendingSubscriptions = nil
	p.mu.Unlock()
	p.forwarding.Unlock()
	p.negotiation.Unlock()
	for _, sub := range subs {
		sub.source.requestPLI()
	}
	return nil
}

func (m *Manager) ICE(ctx context.Context, id string, candidate *pion.ICECandidateInit) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if candidate == nil {
		candidate = &pion.ICECandidateInit{}
	}
	if len(candidate.Candidate) > 4096 || strings.ContainsAny(candidate.Candidate, "\r\n") {
		return media.ErrInvalid
	}
	if candidate.Candidate != "" {
		parsed, parseErr := ice.UnmarshalCandidate(strings.TrimPrefix(candidate.Candidate, "candidate:"))
		if parseErr != nil {
			return media.ErrInvalid
		}
		if parsed.Component() != 1 {
			return media.ErrInvalid
		}
	}
	if candidate.SDPMid != nil && len(*candidate.SDPMid) > 64 {
		return media.ErrInvalid
	}
	if candidate.UsernameFragment != nil && len(*candidate.UsernameFragment) > 256 {
		return media.ErrInvalid
	}
	p.negotiation.Lock()
	defer p.negotiation.Unlock()
	p.mu.Lock()
	closed := p.closing
	p.mu.Unlock()
	if closed {
		return media.ErrPeerNotFound
	}
	if p.remoteICECount >= 128 {
		return media.ErrLimit
	}
	if p.pc.RemoteDescription() == nil {
		if len(p.pendingRemoteICE) >= 64 {
			return media.ErrLimit
		}
		// Copy caller-owned optional pointers as well as the struct so buffering
		// cannot retain mutable HTTP decoder objects across requests.
		buffered := *candidate
		if candidate.SDPMid != nil {
			value := *candidate.SDPMid
			buffered.SDPMid = &value
		}
		if candidate.SDPMLineIndex != nil {
			value := *candidate.SDPMLineIndex
			buffered.SDPMLineIndex = &value
		}
		if candidate.UsernameFragment != nil {
			value := *candidate.UsernameFragment
			buffered.UsernameFragment = &value
		}
		p.pendingRemoteICE = append(p.pendingRemoteICE, buffered)
		p.remoteICECount++
		return nil
	}
	if err = p.pc.AddICECandidate(*candidate); err != nil {
		return media.ErrInvalid
	}
	p.remoteICECount++
	return nil
}

func (m *Manager) Unpublish(ctx context.Context, id, trackID string) error {
	p, err := m.get(id)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	track := p.publications[trackID]
	p.mu.Unlock()
	if track != nil {
		m.unpublish(track)
	}
	return nil
}

func validateOffer(raw string, maxPeers int) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > 49152 {
		return nil, media.ErrInvalid
	}
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return nil, media.ErrInvalid
	}
	if len(description.MediaDescriptions) == 0 || len(description.MediaDescriptions) > 2*maxPeers {
		return nil, media.ErrLimit
	}
	// Validate embedded candidates before passing untrusted SDP into Pion
	// (which may otherwise warn and ignore bad values). Pion's complete SDP
	// offers include redundant component-2 candidates even with rtcp-mux;
	// trickle remains component-1-only, while complete SDP permits 1 or 2.
	attributes := [][]sdp.Attribute{description.Attributes}
	for _, section := range description.MediaDescriptions {
		attributes = append(attributes, section.Attributes)
	}
	candidates := 0
	for _, attrs := range attributes {
		for _, attr := range attrs {
			if attr.Key != "candidate" {
				continue
			}
			candidates++
			if candidates > 128 {
				return nil, media.ErrLimit
			}
			if len(attr.Value) == 0 || len(attr.Value) > 4096 {
				return nil, media.ErrInvalid
			}
			parsed, err := ice.UnmarshalCandidate(attr.Value)
			if err != nil || (parsed.Component() != 1 && parsed.Component() != 2) {
				return nil, media.ErrInvalid
			}
		}
	}
	active := map[string]bool{}
	for _, section := range description.MediaDescriptions {
		kind := section.MediaName.Media
		if kind != "audio" && kind != "video" {
			return nil, media.ErrInvalid
		}
		if section.MediaName.Port.Value == 0 {
			continue
		}
		direction := "sendrecv"
		for _, attr := range description.Attributes {
			if attr.Key == "sendonly" || attr.Key == "sendrecv" || attr.Key == "recvonly" || attr.Key == "inactive" {
				direction = attr.Key
			}
		}
		for _, attr := range section.Attributes {
			if attr.Key == "sendonly" || attr.Key == "sendrecv" || attr.Key == "recvonly" || attr.Key == "inactive" {
				direction = attr.Key
			}
		}
		if direction != "sendonly" && direction != "sendrecv" {
			continue
		}
		if active[kind] {
			return nil, media.ErrLimit
		}
		supported := false
		for _, attr := range section.Attributes {
			if attr.Key == "rtpmap" {
				fields := strings.Fields(attr.Value)
				if len(fields) == 2 && ((kind == "audio" && strings.EqualFold(fields[1], "opus/48000/2")) || (kind == "video" && strings.EqualFold(fields[1], "VP8/90000"))) {
					supported = true
				}
			}
		}
		if !supported {
			return nil, media.ErrInvalid
		}
		active[kind] = true
	}
	return active, nil
}

func answeredSSRCs(raw string) (map[uint32]bool, error) {
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return nil, media.ErrNegotiation
	}
	result := map[uint32]bool{}
	for _, section := range description.MediaDescriptions {
		if section.MediaName.Port.Value == 0 || (section.MediaName.Media != "audio" && section.MediaName.Media != "video") {
			continue
		}
		direction := "sendrecv"
		for _, attrs := range [][]sdp.Attribute{description.Attributes, section.Attributes} {
			for _, attr := range attrs {
				switch attr.Key {
				case "sendonly", "sendrecv", "recvonly", "inactive":
					direction = attr.Key
				}
			}
		}
		if direction != "sendonly" && direction != "sendrecv" {
			continue
		}
		declared := map[uint32]bool{}
		repair := map[uint32]bool{}
		for _, attr := range section.Attributes {
			fields := strings.Fields(attr.Value)
			switch attr.Key {
			case "ssrc":
				if len(fields) < 2 {
					return nil, media.ErrNegotiation
				}
				id, err := strconv.ParseUint(fields[0], 10, 32)
				if err != nil || id == 0 {
					return nil, media.ErrNegotiation
				}
				declared[uint32(id)] = true
			case "ssrc-group":
				if len(fields) < 3 {
					return nil, media.ErrNegotiation
				}
				if fields[0] == "FID" || fields[0] == "FEC" || fields[0] == "FEC-FR" {
					for _, value := range fields[2:] {
						id, err := strconv.ParseUint(value, 10, 32)
						if err != nil || id == 0 {
							return nil, media.ErrNegotiation
						}
						repair[uint32(id)] = true
					}
				}
			}
		}
		for id := range declared {
			if !repair[id] {
				result[id] = true
			}
		}
	}
	return result, nil
}
