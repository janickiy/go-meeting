package sfu

import (
	"context"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"sync"
)

// SetPolicy is called only by the authenticated service boundary. A persisted
// monotonically increasing version prevents delayed joins/offers undoing mute.
func (m *Manager) SetPolicy(ctx context.Context, conferenceID, participantID string, policy media.ParticipantPolicy) error {
	if policy.Version < 0 {
		return media.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	r := m.rooms[conferenceID]
	m.mu.Unlock()
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if previous, ok := r.policies[participantID]; ok {
		if previous.Version > policy.Version {
			r.mu.Unlock()
			return nil
		}
		if previous.Version == policy.Version && previous != policy {
			r.mu.Unlock()
			return media.ErrNegotiation
		}
	}
	r.policies[participantID] = policy
	peers := []*peer{}
	retire := []*publishedTrack{}
	for _, p := range r.peers {
		if p.binding.ParticipantID == participantID {
			peers = append(peers, p)
			if policy.ScreenBlocked || policy.CameraBlocked || policy.Kicked {
				delete(r.screens, p.id)
			}
		}
	}
	for _, t := range r.tracks {
		if t.metadata.ParticipantID == participantID && !policy.Allows(t.metadata.Source) {
			t.permitted.Store(false)
			retire = append(retire, t)
		}
	}
	r.mu.Unlock()
	for _, p := range peers {
		p.mu.Lock()
		for mid, source := range p.allowedSources {
			if !policy.Allows(source) {
				delete(p.allowedSources, mid)
			}
		}
		p.mu.Unlock()
		p.emit("media.policy", map[string]any{"mediaPeerId": p.id, "policy": policy})
	}
	for _, t := range retire {
		m.unpublish(t)
	}
	if policy.Kicked {
		// Cancel every tab before any transport cleanup can wait on an event
		// writer, preserving the same immediate fence as conference shutdown.
		m.mu.Lock()
		detached := []*peer{}
		for _, p := range peers {
			if m.peers[p.id] == p {
				m.detachLocked(p)
				detached = append(detached, p)
			}
		}
		m.mu.Unlock()
		done := make(chan struct{})
		go func() {
			var workers sync.WaitGroup
			for _, p := range detached {
				workers.Add(1)
				go func(p *peer) { defer workers.Done(); m.finishDetached(p) }(p)
			}
			workers.Wait()
			close(done)
		}()
		return waitClosed(ctx, done)
	}
	return nil
}

func (m *Manager) ParticipantPolicy(conferenceID, participantID string) (media.ParticipantPolicy, bool) {
	m.mu.Lock()
	r := m.rooms[conferenceID]
	m.mu.Unlock()
	if r == nil {
		return media.ParticipantPolicy{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.policies[participantID], true
}

func (p *peer) reserveSources(sources map[string]media.Source) error {
	p.room.mu.Lock()
	defer p.room.mu.Unlock()
	policy := p.room.policies[p.binding.ParticipantID]
	wantsScreen := false
	for _, source := range sources {
		if !policy.Allows(source) {
			return media.ErrPolicy
		}
		if source == media.SourceVideoScreen {
			wantsScreen = true
		}
	}
	if wantsScreen && !p.room.screens[p.id] && len(p.room.screens) >= p.manager.opts.MaxScreenSharers {
		return media.ErrScreenConflict
	}
	if wantsScreen {
		p.room.screens[p.id] = true
	} else {
		delete(p.room.screens, p.id)
	}
	return nil
}
