package sfu

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type publishedTrack struct {
	metadata    media.Track
	publisher   *peer
	remote      *pion.TrackRemote
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	active      bool
	subscribers map[string]*subscription
	lastPLI     time.Time
	pli         chan struct{}
	stopOnce    sync.Once
}

type subscription struct {
	source   *publishedTrack
	peer     *peer
	local    *pion.TrackLocalStaticRTP
	sender   *pion.RTPSender
	ctx      context.Context
	cancel   context.CancelFunc
	queue    chan *rtp.Packet
	stopOnce sync.Once
	ready    atomic.Bool
}

func (m *Manager) publish(p *peer, remote *pion.TrackRemote) {
	if !supported(remote) || remote.RID() != "" {
		p.emit("media.error", map[string]string{"mediaPeerId": p.id, "code": "unsupported_codec_or_simulcast"})
		p.stopAsync()
		return
	}
	kind, source := media.KindAudio, media.SourceMicrophone
	if remote.Kind() == pion.RTPCodecTypeVideo {
		kind, source = media.KindVideo, media.SourceCamera
	}
	ctx, cancel := context.WithCancel(p.ctx)
	t := &publishedTrack{metadata: media.Track{ID: uuid.NewString(), StreamID: p.id, MediaPeerID: p.id, ParticipantID: p.binding.ParticipantID, Kind: kind, Source: source},
		publisher: p, remote: remote, ctx: ctx, cancel: cancel, active: true, subscribers: map[string]*subscription{}, pli: make(chan struct{}, 1)}
	p.mu.Lock()
	if p.closing || !p.allowedKinds[kind] {
		p.mu.Unlock()
		cancel()
		return
	}
	count := 0
	for _, old := range p.publications {
		if old.metadata.Kind == kind {
			count++
		}
	}
	if p.closing || len(p.publications) >= m.opts.MaxPublishedTracks || count >= 1 {
		p.mu.Unlock()
		cancel()
		p.emit("media.error", map[string]string{"mediaPeerId": p.id, "code": "media_limit_exceeded"})
		p.stopAsync()
		return
	}
	p.publications[t.metadata.ID] = t
	// Publication registration and the closing snapshot share the peer lock.
	// Keep it until BOTH registries are updated, otherwise unpublish can run
	// between the writes and a delayed room insert creates an inactive zombie.
	// Lock order is peer.mu -> room.mu; no room->peer nested acquisition exists.
	p.room.mu.Lock()
	p.room.tracks[t.metadata.ID] = t
	peers := make([]*peer, 0, len(p.room.peers))
	for _, target := range p.room.peers {
		peers = append(peers, target)
	}
	p.room.mu.Unlock()
	p.mu.Unlock()
	if kind == media.KindVideo {
		p.start(t.keyframes)
	}
	m.log(p, "track_published", []any{"track_id", t.metadata.ID, "kind", kind, "source", source})
	p.emit("media.published", map[string]any{"mediaPeerId": p.id, "track": t.metadata})
	for _, target := range peers {
		m.subscribe(t, target)
	}
	defer m.unpublish(t)
	for {
		packet, _, err := remote.ReadRTP()
		if err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		// An SFU must not copy publisher-specific MID/RID/TWCC extension IDs
		// into a differently negotiated subscriber transport. Pion rewrites
		// SSRC/PT for each local sender; encoded payload/timestamps are retained.
		packet.Header.Extension = false
		packet.Header.ExtensionProfile = 0
		packet.Header.Extensions = nil
		if len(packet.Payload) > 4096 {
			m.dropped.Add(1)
			continue
		}
		t.mu.Lock()
		subscriptions := make([]*subscription, 0, len(t.subscribers))
		for _, sub := range t.subscribers {
			subscriptions = append(subscriptions, sub)
		}
		t.mu.Unlock()
		for _, sub := range subscriptions {
			if !sub.peer.ready.Load() || !sub.ready.Load() {
				continue
			}
			select {
			case <-sub.ctx.Done():
				continue
			default:
			}
			select {
			case sub.queue <- packet:
			default:
				m.dropped.Add(1)
				t.requestPLI()
			}
		}
	}
}

func (m *Manager) syncPeer(p *peer) {
	p.room.mu.Lock()
	tracks := make([]*publishedTrack, 0, len(p.room.tracks))
	for _, t := range p.room.tracks {
		tracks = append(tracks, t)
	}
	p.room.mu.Unlock()
	for _, t := range tracks {
		m.subscribe(t, p)
	}
}

func (m *Manager) subscribe(t *publishedTrack, p *peer) {
	// Multiple tabs have distinct endpoints, but a user's own microphone and
	// camera are never echoed to their other endpoints.
	if t.publisher.binding.ParticipantID == p.binding.ParticipantID {
		return
	}
	p.negotiation.Lock()
	p.mu.Lock()
	closed := p.closing
	existing := p.subscriptions[t.metadata.ID] != nil
	p.mu.Unlock()
	t.mu.Lock()
	active := t.active
	t.mu.Unlock()
	if closed || existing || !active {
		p.negotiation.Unlock()
		return
	}
	local, err := pion.NewTrackLocalStaticRTP(t.remote.Codec().RTPCodecCapability, t.metadata.ID, t.metadata.StreamID)
	if err != nil {
		p.negotiation.Unlock()
		return
	}
	sender, err := p.pc.AddTrack(local)
	if err != nil {
		p.negotiation.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	sub := &subscription{source: t, peer: p, local: local, sender: sender, ctx: ctx, cancel: cancel, queue: make(chan *rtp.Packet, m.opts.QueueSize)}
	t.mu.Lock()
	p.mu.Lock()
	if !t.active || p.closing || p.subscriptions[t.metadata.ID] != nil {
		p.mu.Unlock()
		t.mu.Unlock()
		cancel()
		_ = p.pc.RemoveTrack(sender)
		p.negotiation.Unlock()
		return
	}
	t.subscribers[p.id] = sub
	p.subscriptions[t.metadata.ID] = sub
	p.mu.Unlock()
	t.mu.Unlock()
	p.negotiation.Unlock()
	if !p.start(sub.forward) || !p.start(sub.readRTCP) {
		m.unsubscribe(sub)
		return
	}
	p.changed()
	t.requestPLI()
}

func (s *subscription) forward() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case packet := <-s.queue:
			if s.ctx.Err() != nil {
				return
			}
			s.peer.forwarding.RLock()
			if s.ctx.Err() != nil || !s.peer.ready.Load() || !s.ready.Load() {
				s.peer.forwarding.RUnlock()
				continue
			}
			if err := s.local.WriteRTP(packet); err != nil {
				s.peer.forwarding.RUnlock()
				if s.ctx.Err() == nil {
					s.peer.manager.dropped.Add(1)
				}
				continue
			}
			s.peer.forwarding.RUnlock()
			s.peer.manager.packets.Add(1)
			s.peer.manager.bytes.Add(uint64(len(packet.Payload)))
		}
	}
}

func (s *subscription) readRTCP() {
	for {
		packets, _, err := s.sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, packet := range packets {
			switch packet.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				s.source.requestPLI()
			}
		}
		select {
		case <-s.ctx.Done():
			return
		default:
		}
	}
}

func (t *publishedTrack) requestPLI() {
	if t.metadata.Kind != media.KindVideo {
		return
	}
	select {
	case t.pli <- struct{}{}:
	default:
	}
}

func (t *publishedTrack) keyframes() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.pli:
			t.mu.Lock()
			delay := time.Until(t.lastPLI.Add(500 * time.Millisecond))
			t.mu.Unlock()
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-t.ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			select {
			case <-t.pli:
			default:
			}
			if t.ctx.Err() != nil {
				return
			}
			_ = t.publisher.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(t.remote.SSRC())}})
			t.mu.Lock()
			t.lastPLI = time.Now()
			t.mu.Unlock()
		}
	}
}

func (m *Manager) unsubscribe(s *subscription) {
	s.stopOnce.Do(func() {
		s.cancel()
		s.peer.negotiation.Lock()
		s.peer.mu.Lock()
		delete(s.peer.subscriptions, s.source.metadata.ID)
		s.peer.mu.Unlock()
		_ = s.peer.pc.RemoveTrack(s.sender)
		// Stop independently as RemoveTrack on a closed PC returns before Stop.
		_ = s.sender.Stop()
		s.peer.negotiation.Unlock()
		s.source.mu.Lock()
		delete(s.source.subscribers, s.peer.id)
		s.source.mu.Unlock()
		s.peer.changed()
	})
}

func (m *Manager) unpublish(t *publishedTrack) {
	t.stopOnce.Do(func() {
		t.mu.Lock()
		t.active = false
		subs := make([]*subscription, 0, len(t.subscribers))
		for _, sub := range t.subscribers {
			subs = append(subs, sub)
		}
		t.mu.Unlock()
		t.cancel()
		_ = t.remote.SetReadDeadline(time.Now())
		t.publisher.mu.Lock()
		delete(t.publisher.publications, t.metadata.ID)
		t.publisher.mu.Unlock()
		t.publisher.room.mu.Lock()
		delete(t.publisher.room.tracks, t.metadata.ID)
		t.publisher.room.mu.Unlock()
		for _, sub := range subs {
			m.unsubscribe(sub)
		}
		m.log(t.publisher, "track_unpublished", []any{"track_id", t.metadata.ID, "kind", t.metadata.Kind, "source", t.metadata.Source})
		t.publisher.emit("media.unpublished", map[string]string{"mediaPeerId": t.publisher.id, "trackId": t.metadata.ID})
	})
}

func (m *Manager) Tracks(id string) []media.Track {
	p, err := m.get(id)
	if err != nil {
		return []media.Track{}
	}
	tracks := p.tracks()
	sort.Slice(tracks, func(i, j int) bool { return tracks[i].ID < tracks[j].ID })
	return tracks
}
