package sfu

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
)

var errEgressOverflow = errors.New("recording_egress_overflow")

// egress is a passive encoded-packet observer. RTP forwarding never waits for
// recording I/O: a full bounded queue fails only this recording subscription.
type egress struct {
	manager   *Manager
	room      *room
	id        string
	mu        sync.Mutex
	frames    chan media.EgressFrame
	done      chan struct{}
	err       error
	closed    bool
	sequence  uint64
	tracks    map[string]bool
	closeOnce sync.Once
}

func (m *Manager) SubscribeRecording(ctx context.Context, conferenceID, recordingID string) (media.EgressSubscription, error) {
	for _, id := range []string{conferenceID, recordingID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return nil, media.ErrInvalid
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.roomClosures[conferenceID] != nil {
		return nil, media.ErrUnavailable
	}
	r := m.rooms[conferenceID]
	if r == nil {
		if len(m.rooms) >= m.opts.MaxRooms {
			return nil, media.ErrLimit
		}
		r = newRoom(conferenceID)
		m.rooms[conferenceID] = r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.egresses) > 0 {
		return nil, media.ErrNegotiation
	}
	e := &egress{manager: m, room: r, id: recordingID, frames: make(chan media.EgressFrame, m.opts.EgressQueueSize), done: make(chan struct{}), tracks: map[string]bool{}}
	e.mu.Lock()
	e.enqueueLocked(media.EgressFrame{Type: "hello"})
	e.mu.Unlock()
	for _, t := range r.tracks {
		e.addTrack(t)
		t.requestPLI()
	}
	r.egresses[recordingID] = e
	return e, nil
}

func (e *egress) Frames() <-chan media.EgressFrame { return e.frames }
func (e *egress) Done() <-chan struct{}            { return e.done }
func (e *egress) Err() error                       { e.mu.Lock(); defer e.mu.Unlock(); return e.err }

func (e *egress) enqueueLocked(frame media.EgressFrame) {
	if e.closed {
		return
	}
	e.sequence++
	frame.Sequence = e.sequence
	if frame.CapturedAt == 0 {
		frame.CapturedAt = time.Now().UnixNano()
	}
	select {
	case e.frames <- frame:
	default:
		e.manager.egressDropped.Add(1)
		e.closed = true
		e.err = errEgressOverflow
		close(e.done)
	}
}

func (e *egress) addTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.tracks[t.metadata.ID] || !t.permitted.Load() {
		return
	}
	e.tracks[t.metadata.ID] = true
	codec := t.remote.Codec()
	descriptor := media.EgressTrack{Track: t.metadata, MimeType: codec.MimeType, ClockRate: codec.ClockRate, Channels: codec.Channels, PayloadType: uint8(t.remote.PayloadType()), SSRC: uint32(t.remote.SSRC())}
	e.enqueueLocked(media.EgressFrame{Type: "track", Track: &descriptor, TrackID: t.metadata.ID})
}

func (e *egress) endTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] {
		return
	}
	delete(e.tracks, t.metadata.ID)
	e.enqueueLocked(media.EgressFrame{Type: "track.end", TrackID: t.metadata.ID})
}

func (e *egress) packet(t *publishedTrack, raw []byte, at int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] || !t.permitted.Load() {
		return
	}
	e.enqueueLocked(media.EgressFrame{Type: "rtp", TrackID: t.metadata.ID, RTP: raw, CapturedAt: at})
}

func (e *egress) fail(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.err = err
		e.closed = true
		close(e.done)
	}
}

func (e *egress) Close() {
	e.closeOnce.Do(func() {
		e.fail(nil)
		e.manager.mu.Lock()
		e.room.mu.Lock()
		if e.room.egresses[e.id] == e {
			delete(e.room.egresses, e.id)
		}
		if len(e.room.peers) == 0 && len(e.room.egresses) == 0 && e.manager.rooms[e.room.id] == e.room {
			delete(e.manager.rooms, e.room.id)
		}
		e.room.mu.Unlock()
		e.manager.mu.Unlock()
	})
}

func (e *egress) Keyframes() {
	e.room.mu.Lock()
	defer e.room.mu.Unlock()
	for _, t := range e.room.tracks {
		t.requestPLI()
	}
}

func (e *egress) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enqueueLocked(media.EgressFrame{Type: "ping"})
}

func (m *Manager) recordPacket(t *publishedTrack, packet *rtp.Packet) {
	t.publisher.room.mu.Lock()
	egresses := make([]*egress, 0, len(t.publisher.room.egresses))
	for _, e := range t.publisher.room.egresses {
		egresses = append(egresses, e)
	}
	t.publisher.room.mu.Unlock()
	if len(egresses) == 0 {
		return
	}
	raw, err := packet.Marshal()
	if err != nil {
		return
	}
	at := time.Now().UnixNano()
	for _, e := range egresses {
		e.packet(t, raw, at)
	}
}
