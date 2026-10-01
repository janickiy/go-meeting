package sfu

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

type pionHarness struct {
	manager *Manager
	mu      sync.Mutex
	peers   map[string]*testPeer
	errors  chan error
}
type testPeer struct {
	harness                  *pionHarness
	binding                  media.Binding
	id                       string
	pc                       *pion.PeerConnection
	audio, video             *pion.TrackLocalStaticRTP
	audioSender, videoSender *pion.RTPSender
	ctx                      context.Context
	cancel                   context.CancelFunc
	neg                      sync.Mutex
	mu                       sync.Mutex
	pending                  []*pion.ICECandidateInit
	received                 map[string]string
	packets                  uint64
	pli                      uint64
	renegotiate              chan struct{}
	wg                       sync.WaitGroup
	closed                   bool
	publishingPaused         atomic.Bool
}

func harness(t *testing.T, maxPeers int, configured ...Options) *pionHarness {
	t.Helper()
	h := &pionHarness{peers: map[string]*testPeer{}, errors: make(chan error, 128)}
	options := Options{MaxRooms: 2, NegotiationTimeout: 10 * time.Second}
	if len(configured) > 0 {
		options = configured[0]
	}
	options.MaxPeers = maxPeers
	options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	intercept := options.Emit
	options.Emit = func(binding media.Binding, kind string, data any) {
		if intercept != nil {
			intercept(binding, kind, data)
		}
		h.emit(binding, kind, data)
	}
	m, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	h.manager = m
	t.Cleanup(func() {
		h.mu.Lock()
		peers := make([]*testPeer, 0, len(h.peers))
		for _, p := range h.peers {
			peers = append(peers, p)
		}
		h.mu.Unlock()
		for _, p := range peers {
			p.close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Errorf("manager shutdown: %v", err)
		}
	})
	return h
}

func (h *pionHarness) emit(binding media.Binding, kind string, data any) {
	h.mu.Lock()
	p := h.peers[binding.ConnectionID]
	h.mu.Unlock()
	if p == nil {
		return
	}
	switch kind {
	case "media.ice":
		candidate, _ := data.(map[string]any)["candidate"].(*pion.ICECandidateInit)
		if candidate == nil {
			return
		}
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		if p.pc.RemoteDescription() == nil {
			p.pending = append(p.pending, candidate)
			p.mu.Unlock()
			return
		}
		p.mu.Unlock()
		if err := p.pc.AddICECandidate(*candidate); err != nil {
			h.problem(err)
		}
	case "media.renegotiate":
		select {
		case p.renegotiate <- struct{}{}:
		default:
		}
	}
}

func (h *pionHarness) problem(err error) {
	select {
	case h.errors <- err:
	default:
	}
}

func (h *pionHarness) join(t *testing.T, conference string, participant string) *testPeer {
	return h.joinSlots(t, conference, participant, h.manager.opts.MaxPeers-1)
}

func (h *pionHarness) joinSlots(t *testing.T, conference string, participant string, receiveSlots int) *testPeer {
	t.Helper()
	if participant == "" {
		participant = uuid.NewString()
	}
	b := media.Binding{ConferenceID: conference, ParticipantID: participant, SessionID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString(), AuthorizationExpiresAt: time.Now().Add(time.Hour)}
	settings := pion.SettingEngine{}
	settings.SetNetworkTypes([]pion.NetworkType{pion.NetworkTypeUDP4})
	pc, err := pion.NewAPI(pion.WithSettingEngine(settings)).NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &testPeer{harness: h, binding: b, pc: pc, ctx: ctx, cancel: cancel, received: map[string]string{}, renegotiate: make(chan struct{}, 1)}
	p.audio, _ = pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeOpus, ClockRate: 48000, Channels: 2}, "microphone", "publisher")
	p.video, _ = pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, "camera", "publisher")
	p.audioSender, err = pc.AddTrack(p.audio)
	if err != nil {
		t.Fatal(err)
	}
	p.videoSender, err = pc.AddTrack(p.video)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < receiveSlots; i++ {
		for _, kind := range []pion.RTPCodecType{pion.RTPCodecTypeAudio, pion.RTPCodecTypeVideo} {
			if _, err := pc.AddTransceiverFromKind(kind, pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly}); err != nil {
				t.Fatal(err)
			}
		}
	}
	pc.OnTrack(func(remote *pion.TrackRemote, _ *pion.RTPReceiver) {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return
		}
		p.wg.Add(1)
		p.mu.Unlock()
		go func() {
			defer p.wg.Done()
			for {
				packet, _, err := remote.ReadRTP()
				if err != nil {
					return
				}
				if len(packet.Payload) != 4 || packet.Payload[0] != 0x10 {
					h.problem(fmt.Errorf("encoded RTP payload changed"))
					return
				}
				p.mu.Lock()
				p.received[remote.ID()] = remote.Kind().String()
				p.packets++
				p.mu.Unlock()
			}
		}()
	})
	h.mu.Lock()
	h.peers[b.ConnectionID] = p
	h.mu.Unlock()
	view, err := h.manager.Join(context.Background(), b)
	if err != nil {
		p.close()
		t.Fatal(err)
	}
	p.id = view.MediaPeerID
	// Explicitly drain every publisher sender's RTCP, just as a browser does.
	for _, sender := range []*pion.RTPSender{p.audioSender, p.videoSender} {
		p.wg.Add(1)
		go func(s *pion.RTPSender) {
			defer p.wg.Done()
			for {
				packets, _, err := s.ReadRTCP()
				if err != nil {
					return
				}
				for _, packet := range packets {
					if _, ok := packet.(*rtcp.PictureLossIndication); ok {
						p.mu.Lock()
						p.pli++
						p.mu.Unlock()
					}
				}
			}
		}(sender)
	}
	if err := p.negotiate(); err != nil {
		p.close()
		t.Fatal(err)
	}
	p.wg.Add(2)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-p.renegotiate:
				if err := p.negotiate(); err != nil && p.ctx.Err() == nil && !errors.Is(err, media.ErrPeerNotFound) {
					h.problem(err)
				}
			}
		}
	}()
	go p.publish()
	return p
}

func (p *testPeer) negotiate() error {
	p.neg.Lock()
	defer p.neg.Unlock()
	if p.ctx.Err() != nil {
		return nil
	}
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	complete := pion.GatheringCompletePromise(p.pc)
	if err = p.pc.SetLocalDescription(offer); err != nil {
		return err
	}
	select {
	case <-complete:
	case <-p.ctx.Done():
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("client ICE gathering timeout")
	}
	negotiationID := uuid.NewString()
	answer, err := p.harness.manager.Offer(p.ctx, p.id, negotiationID, p.pc.LocalDescription().SDP)
	if err != nil {
		return err
	}
	if err = p.pc.SetRemoteDescription(answer); err != nil {
		return err
	}
	if err = p.harness.manager.Ready(p.ctx, p.id, negotiationID); err != nil {
		return err
	}
	p.mu.Lock()
	pending := p.pending
	p.pending = nil
	p.mu.Unlock()
	for _, c := range pending {
		if err = p.pc.AddICECandidate(*c); err != nil {
			return err
		}
	}
	return nil
}

func (p *testPeer) publish() {
	defer p.wg.Done()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var sequence uint16
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			if p.publishingPaused.Load() {
				continue
			}
			sequence++
			packet := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: sequence, Timestamp: uint32(sequence) * 960, Marker: true}, Payload: []byte{0x10, 0x01, 0x02, 0x03}}
			_ = p.audio.WriteRTP(packet)
			packet.Header.Timestamp = uint32(sequence) * 1800
			_ = p.video.WriteRTP(packet)
		}
	}
}

func (p *testPeer) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()
	p.cancel()
	_ = p.pc.Close()
	p.harness.mu.Lock()
	delete(p.harness.peers, p.binding.ConnectionID)
	p.harness.mu.Unlock()
	if p.id != "" {
		_ = p.harness.manager.Leave(context.Background(), p.id)
	}
	p.wg.Wait()
}

func eventually(t *testing.T, h *pionHarness, label string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !fn() {
		select {
		case err := <-h.errors:
			t.Fatalf("%s: %v", label, err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout: %s; stats=%+v", label, h.manager.Snapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSFUMediaSmoke(t *testing.T) {
	for _, n := range []int{2, 3, 5} {
		t.Run(fmt.Sprintf("%d_peers", n), func(t *testing.T) {
			beforeG := runtime.NumGoroutine()
			var beforeMem runtime.MemStats
			runtime.ReadMemStats(&beforeMem)
			started := time.Now()
			var beforeCPU syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &beforeCPU)
			h := harness(t, 10)
			conference := uuid.NewString()
			peers := make([]*testPeer, 0, n)
			for i := 0; i < n; i++ {
				peers = append(peers, h.join(t, conference, ""))
				eventually(t, h, "published audio/video", func() bool { return h.manager.Snapshot().Tracks == 2*(i+1) })
			}
			eventually(t, h, "bidirectional Opus/VP8 including late join", func() bool {
				for _, p := range peers {
					p.mu.Lock()
					audio, video := 0, 0
					for _, kind := range p.received {
						if kind == "audio" {
							audio++
						}
						if kind == "video" {
							video++
						}
					}
					p.mu.Unlock()
					if audio != n-1 || video != n-1 {
						return false
					}
				}
				return true
			})
			stats := h.manager.Snapshot()
			if stats.Subscriptions != n*(n-1)*2 {
				t.Fatalf("subscriptions=%d", stats.Subscriptions)
			}
			eventually(t, h, "publisher received video keyframe requests", func() bool {
				for _, p := range peers {
					p.mu.Lock()
					requested := p.pli > 0
					p.mu.Unlock()
					if !requested {
						return false
					}
				}
				return true
			})
			var during runtime.MemStats
			runtime.ReadMemStats(&during)
			// One sustained second after all routes are negotiated, not just a
			// successful first packet. This is a synthetic baseline, not a codec
			// decode or 720p bandwidth/capacity benchmark.
			time.Sleep(time.Second)
			stats = h.manager.Snapshot()
			var duringCPU syscall.Rusage
			_ = syscall.Getrusage(syscall.RUSAGE_SELF, &duringCPU)
			cpuMicros := func(r syscall.Rusage) int64 {
				return r.Utime.Sec*1000000 + int64(r.Utime.Usec) + r.Stime.Sec*1000000 + int64(r.Stime.Usec)
			}
			t.Logf("participants=%d duration=%s cpu=%s goroutines=%d->%d heap=%d->%d packets=%d bytes=%d dropped=%d", n, time.Since(started).Round(time.Millisecond), time.Duration(cpuMicros(duringCPU)-cpuMicros(beforeCPU))*time.Microsecond, beforeG, runtime.NumGoroutine(), beforeMem.HeapAlloc, during.HeapAlloc, stats.Packets, stats.Bytes, stats.Dropped)
			// Stopping an actual publishing m-section must remove its subscriptions.
			first := peers[0]
			first.neg.Lock()
			err := first.pc.RemoveTrack(first.videoSender)
			first.neg.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if err = first.negotiate(); err != nil {
				t.Fatal(err)
			}
			eventually(t, h, "SDP track stop cleanup", func() bool { s := h.manager.Snapshot(); return s.Tracks == n*2-1 && s.Subscriptions == (n*2-1)*(n-1) })
			oldID := first.id
			binding := first.binding
			first.close()
			eventually(t, h, "publisher disconnect cleanup", func() bool { return h.manager.Snapshot().Peers == n-1 && h.manager.Snapshot().Tracks == 2*(n-1) })
			replacement := h.join(t, conference, binding.ParticipantID)
			if replacement.id == oldID {
				t.Fatal("reconnect reused MediaPeer")
			}
			peers[0] = replacement
			eventually(t, h, "reconnect media tracks", func() bool { return h.manager.Snapshot().Tracks == 2*n })
			for _, p := range peers {
				p.close()
			}
			eventually(t, h, "empty room cleanup", func() bool {
				s := h.manager.Snapshot()
				return s.Rooms == 0 && s.Peers == 0 && s.Tracks == 0 && s.Subscriptions == 0
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.manager.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			eventually(t, h, "owned goroutines reclaimed", func() bool { return runtime.NumGoroutine() <= beforeG+10 })
			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			t.Logf("cleanup goroutines=%d heap=%d", runtime.NumGoroutine(), after.HeapAlloc)
		})
	}
}

func TestOfferValidationAndNegotiationSerialization(t *testing.T) {
	h := harness(t, 3)
	p := h.join(t, uuid.NewString(), "")
	p.neg.Lock()
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		p.neg.Unlock()
		t.Fatal(err)
	}
	if err = p.pc.SetLocalDescription(offer); err != nil {
		p.neg.Unlock()
		t.Fatal(err)
	}
	raw := p.pc.LocalDescription().SDP
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := h.manager.Offer(context.Background(), p.id, uuid.NewString(), raw); err != nil {
				h.problem(err)
			}
		}()
	}
	wg.Wait()
	server, _ := h.manager.get(p.id)
	if server.pc.SignalingState() != pion.SignalingStateStable {
		t.Fatal("concurrent offers left non-stable signaling state")
	}
	if err = p.pc.SetRemoteDescription(*server.pc.LocalDescription()); err != nil {
		p.neg.Unlock()
		t.Fatal(err)
	}
	if err = h.manager.Ready(context.Background(), p.id, server.negotiationID); err != nil {
		p.neg.Unlock()
		t.Fatal(err)
	}
	p.neg.Unlock()
	select {
	case err := <-h.errors:
		t.Fatal(err)
	default:
	}
	// No camera/audio decoding is needed to reject extra publishers or an
	// unsupported-only video codec before Pion creates transport resources.
	if _, err = validateOffer(strings.ReplaceAll(raw, "VP8/90000", "UNSUPPORTED/90000"), 3); !errors.Is(err, media.ErrInvalid) {
		t.Fatalf("unsupported codec: %v", err)
	}
	if _, err = validateOffer(raw, 2); !errors.Is(err, media.ErrLimit) {
		t.Fatalf("m-section flood: %v", err)
	}
	if err = h.manager.ICE(context.Background(), p.id, &pion.ICECandidateInit{Candidate: "invalid\r\n"}); !errors.Is(err, media.ErrInvalid) {
		t.Fatalf("malformed candidate: %v", err)
	}
}

func TestJoinShutdownRace(t *testing.T) {
	h := harness(t, 10)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString()}
			view, err := h.manager.Join(context.Background(), b)
			if err == nil {
				_ = h.manager.Leave(context.Background(), view.MediaPeerID)
			} else if !errors.Is(err, media.ErrLimit) && !errors.Is(err, media.ErrUnavailable) {
				h.problem(err)
			}
		}()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.manager.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if s := h.manager.Snapshot(); s.Rooms != 0 || s.Peers != 0 || s.Subscriptions != 0 {
		t.Fatalf("shutdown leaked state: %+v", s)
	}
	select {
	case err := <-h.errors:
		t.Fatal(err)
	default:
	}
}

func TestFailedTransportAndAdmissionTimeoutCleanup(t *testing.T) {
	h := harness(t, 3, Options{ICEDisconnectedTimeout: 100 * time.Millisecond, ICEFailedTimeout: 200 * time.Millisecond, ICEKeepaliveInterval: 50 * time.Millisecond, NegotiationTimeout: time.Second})
	p := h.join(t, uuid.NewString(), "")
	eventually(t, h, "transport connected", func() bool { return p.pc.ConnectionState() == pion.PeerConnectionStateConnected })
	// Abrupt remote transport loss, not an explicit signaling Leave command.
	_ = p.pc.Close()
	eventually(t, h, "failed peer transport cleaned", func() bool { s := h.manager.Snapshot(); return s.Peers == 0 && s.Rooms == 0 && s.Tracks == 0 })
	binding := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString()}
	if _, err := h.manager.Join(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "joined without offer expires", func() bool { return h.manager.Snapshot().Peers == 0 && h.manager.Snapshot().Rooms == 0 })
}

func TestManagerLimitsDuplicatesAndCleanup(t *testing.T) {
	h := harness(t, 2)
	conf := uuid.NewString()
	binding := media.Binding{ConferenceID: conf, ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString()}
	one, err := h.manager.Join(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := h.manager.Join(context.Background(), binding)
	if err != nil || duplicate.MediaPeerID != one.MediaPeerID {
		t.Fatal("duplicate join not idempotent")
	}
	other := binding
	other.ParticipantID = uuid.NewString()
	if _, err = h.manager.Join(context.Background(), other); !errors.Is(err, media.ErrUnauthorized) {
		t.Fatalf("binding mismatch accepted: %v", err)
	}
	other.ConnectionID = uuid.NewString()
	other.SessionID = uuid.NewString()
	if _, err = h.manager.Join(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	third := other
	third.ConnectionID = uuid.NewString()
	third.SessionID = uuid.NewString()
	if _, err = h.manager.Join(context.Background(), third); !errors.Is(err, media.ErrLimit) {
		t.Fatalf("room limit: %v", err)
	}
	if _, err = h.manager.Offer(context.Background(), one.MediaPeerID, uuid.NewString(), "not SDP"); !errors.Is(err, media.ErrInvalid) {
		t.Fatalf("malformed SDP: %v", err)
	}
	if err = h.manager.ICE(context.Background(), one.MediaPeerID, nil); err != nil {
		t.Fatalf("ICE before offer: %v", err)
	}
	if err = h.manager.CloseConference(context.Background(), conf); err != nil {
		t.Fatal(err)
	}
	if s := h.manager.Snapshot(); s.Rooms != 0 || s.Peers != 0 {
		t.Fatalf("cleanup: %+v", s)
	}
	if err = h.manager.Leave(context.Background(), one.MediaPeerID); err != nil {
		t.Fatal(err)
	}
}

func TestSameParticipantEndpointsAreNotEchoed(t *testing.T) {
	h := harness(t, 3)
	conf, participant := uuid.NewString(), uuid.NewString()
	a := h.join(t, conf, participant)
	b := h.join(t, conf, participant)
	eventually(t, h, "two endpoints published", func() bool { return h.manager.Snapshot().Tracks == 4 })
	if s := h.manager.Snapshot(); s.Subscriptions != 0 {
		t.Fatalf("own tracks echoed across tabs: %+v", s)
	}
	a.close()
	b.close()
}

func TestRepeatedRoomCyclesAndExplicitUnpublish(t *testing.T) {
	h := harness(t, 3)
	for i := 0; i < 3; i++ {
		conf := uuid.NewString()
		a := h.join(t, conf, "")
		b := h.join(t, conf, "")
		eventually(t, h, "room ready", func() bool { return h.manager.Snapshot().Tracks == 4 && h.manager.Snapshot().Subscriptions == 4 })
		p, _ := h.manager.get(a.id)
		p.mu.Lock()
		trackID := ""
		for id, track := range p.publications {
			if track.metadata.Kind == "video" {
				trackID = id
			}
		}
		p.mu.Unlock()
		if err := h.manager.Unpublish(context.Background(), a.id, trackID); err != nil {
			t.Fatal(err)
		}
		if err := h.manager.Unpublish(context.Background(), a.id, trackID); err != nil {
			t.Fatal(err)
		}
		eventually(t, h, "explicit track cleanup", func() bool { return h.manager.Snapshot().Tracks == 3 && h.manager.Snapshot().Subscriptions == 3 })
		a.close()
		b.close()
		if s := h.manager.Snapshot(); s.Rooms != 0 {
			t.Fatalf("cycle %d left a room", i)
		}
	}
}

func TestEarlyTrickleICEIsBoundedValidatedAndDrained(t *testing.T) {
	h := harness(t, 3)
	b := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString()}
	view, err := h.manager.Join(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := h.manager.get(view.MediaPeerID)
	ctx := context.Background()
	if err = h.manager.ICE(ctx, p.id, &pion.ICECandidateInit{Candidate: "malformed ICE"}); !errors.Is(err, media.ErrInvalid) {
		t.Fatalf("invalid early candidate: %v", err)
	}
	mid := "0"
	candidate := &pion.ICECandidateInit{Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 50000 typ host", SDPMid: &mid}
	if err = h.manager.ICE(ctx, p.id, candidate); err != nil {
		t.Fatal(err)
	}
	mid = "mutated after request"
	if p.pendingRemoteICE[0].SDPMid == nil || *p.pendingRemoteICE[0].SDPMid != "0" {
		t.Fatal("candidate retained mutable caller pointer")
	}
	for i := 1; i < 64; i++ {
		if err = h.manager.ICE(ctx, p.id, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err = h.manager.ICE(ctx, p.id, nil); !errors.Is(err, media.ErrLimit) {
		t.Fatalf("pending ICE flood not bounded: %v", err)
	}
	client, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.AddTransceiverFromKind(pion.RTPCodecTypeAudio, pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionSendonly}); err != nil {
		t.Fatal(err)
	}
	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.manager.Offer(ctx, p.id, uuid.NewString(), offer.SDP); err != nil {
		t.Fatal(err)
	}
	if len(p.pendingRemoteICE) != 0 {
		t.Fatal("early ICE was not drained")
	}
	for i := 64; i < 128; i++ {
		if err = h.manager.ICE(ctx, p.id, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err = h.manager.ICE(ctx, p.id, nil); !errors.Is(err, media.ErrLimit) {
		t.Fatalf("post-offer ICE flood not bounded: %v", err)
	}
}

func TestPublicationRegistrationConcurrentLeaveDoesNotRetainZombie(t *testing.T) {
	h := harness(t, 3)
	conference := uuid.NewString()
	keeper := h.join(t, conference, "")
	eventually(t, h, "keeper publications", func() bool { return h.manager.Snapshot().Tracks == 2 })
	for i := 0; i < 24; i++ {
		publisher := h.join(t, conference, "")
		// First RTP fires at 20ms; vary leave around the OnTrack/registration
		// boundary while another member keeps the same room alive.
		time.Sleep(time.Duration(17+i%7) * time.Millisecond)
		publisher.close()
		eventually(t, h, "publisher completely detached", func() bool { s := h.manager.Snapshot(); return s.Peers == 1 && s.Tracks == 2 && s.Subscriptions == 0 })
		p, _ := h.manager.get(keeper.id)
		p.room.mu.Lock()
		tracks := make([]*publishedTrack, 0, len(p.room.tracks))
		for _, track := range p.room.tracks {
			tracks = append(tracks, track)
		}
		p.room.mu.Unlock()
		for _, track := range tracks {
			if track.publisher.id != keeper.id {
				t.Fatal("departed publisher retained in room registry")
			}
			track.mu.Lock()
			active := track.active
			track.mu.Unlock()
			if !active {
				t.Fatal("inactive publication retained in room registry")
			}
		}
	}
	keeper.close()
}

func TestBatchCloseFencesAllPeersBeforeBlockedEmitCompletes(t *testing.T) {
	for _, action := range []string{"conference", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			var block atomic.Bool
			entered := make(chan string, 16)
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			h := harness(t, 3, Options{Emit: func(binding media.Binding, _ string, _ any) {
				if block.Load() {
					select {
					case entered <- binding.ConnectionID:
					default:
					}
					<-release
				}
			}})
			conference := uuid.NewString()
			clients := []*testPeer{h.join(t, conference, ""), h.join(t, conference, ""), h.join(t, conference, "")}
			eventually(t, h, "three media endpoints ready", func() bool { return h.manager.Snapshot().Tracks == 6 && h.manager.Snapshot().Subscriptions == 12 })
			serverPeers := make([]*peer, 0, 3)
			for _, client := range clients {
				p, _ := h.manager.get(client.id)
				serverPeers = append(serverPeers, p)
			}
			block.Store(true)
			for _, p := range serverPeers {
				p.emit("media.state", map[string]string{"mediaPeerId": p.id, "state": "connected"})
			}
			waiting := map[string]bool{}
			deadline := time.After(2 * time.Second)
			for len(waiting) < 3 {
				select {
				case id := <-entered:
					waiting[id] = true
				case <-deadline:
					t.Fatal("event workers did not block")
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			var err error
			if action == "conference" {
				err = h.manager.CloseConference(ctx, conference)
			} else {
				err = h.manager.Shutdown(ctx)
			}
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cleanup wait ignored context: %v", err)
			}
			if len(h.manager.Bindings()) != 0 {
				t.Fatal("batch close retained authorized bindings")
			}
			for _, p := range serverPeers {
				if p.ctx.Err() == nil {
					t.Fatal("peer forwarding was not fenced immediately")
				}
			}
			eventually(t, h, "every transport closed while emit blocked", func() bool {
				for _, p := range serverPeers {
					if p.pc.ConnectionState() != pion.PeerConnectionStateClosed {
						return false
					}
				}
				return true
			})
			packets := h.manager.Snapshot().Packets
			time.Sleep(50 * time.Millisecond)
			if h.manager.Snapshot().Packets != packets {
				t.Fatal("media kept forwarding behind blocked event workers")
			}
			if action == "conference" {
				binding := media.Binding{ConferenceID: conference, ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString()}
				if _, err = h.manager.Join(context.Background(), binding); !errors.Is(err, media.ErrUnavailable) {
					t.Fatalf("join entered closing conference: %v", err)
				}
			}
			unblock()
			ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err = h.manager.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAnswerAppliedBarrierAndLateSubscriptionSnapshot(t *testing.T) {
	h := harness(t, 5)
	conf := uuid.NewString()
	a, b := h.join(t, conf, ""), h.join(t, conf, "")
	eventually(t, h, "initial media ready", func() bool { b.mu.Lock(); defer b.mu.Unlock(); return len(b.received) == 2 })
	b.neg.Lock()
	locked := true
	defer func() {
		if locked {
			b.neg.Unlock()
		}
	}()
	offer, err := b.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	negotiationID := uuid.NewString()
	answer, err := h.manager.Offer(context.Background(), b.id, negotiationID, b.pc.LocalDescription().SDP)
	if err != nil {
		t.Fatal(err)
	}
	server, _ := h.manager.get(b.id)
	if server.ready.Load() {
		t.Fatal("server enabled RTP before applied-answer acknowledgement")
	}
	if err = h.manager.Ready(context.Background(), b.id, uuid.NewString()); !errors.Is(err, media.ErrNegotiation) {
		t.Fatalf("wrong acknowledgement accepted: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	b.mu.Lock()
	before := b.packets
	b.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	b.mu.Lock()
	after := b.packets
	b.mu.Unlock()
	if after != before {
		t.Fatal("RTP continued while answer was withheld")
	}
	// C publishes only AFTER B's answer snapshot. Ready for that answer must
	// activate A's streams, never C's unadvertised SSRCs.
	c := h.join(t, conf, "")
	eventually(t, h, "late publisher registered", func() bool { return h.manager.Snapshot().Tracks == 6 && h.manager.Snapshot().Subscriptions == 12 })
	if err = b.pc.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}
	if err = h.manager.Ready(context.Background(), b.id, negotiationID); err != nil {
		t.Fatal(err)
	}
	if err = h.manager.Ready(context.Background(), b.id, negotiationID); err != nil {
		t.Fatal("ready is not idempotent")
	}
	server.mu.Lock()
	lateIDs := []string{}
	for id, sub := range server.subscriptions {
		if sub.source.publisher.id == c.id {
			lateIDs = append(lateIDs, id)
			if sub.ready.Load() {
				server.mu.Unlock()
				t.Fatal("old Ready activated late subscription")
			}
		}
	}
	server.mu.Unlock()
	if len(lateIDs) != 2 {
		t.Fatal("late publisher subscriptions missing")
	}
	time.Sleep(100 * time.Millisecond)
	b.mu.Lock()
	for _, id := range lateIDs {
		if _, exists := b.received[id]; exists {
			b.mu.Unlock()
			t.Fatal("unadvertised late RTP reached recipient")
		}
	}
	b.mu.Unlock()
	b.neg.Unlock()
	locked = false
	eventually(t, h, "new answer and Ready activates late tracks", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, id := range lateIDs {
			if _, exists := b.received[id]; !exists {
				return false
			}
		}
		return true
	})
	a.close()
	b.close()
	c.close()
}

func TestMissingReadyExpiresEvenConnectedPeer(t *testing.T) {
	h := harness(t, 3, Options{NegotiationTimeout: time.Second})
	a, b := h.join(t, uuid.NewString(), ""), (*testPeer)(nil)
	b = h.join(t, a.binding.ConferenceID, "")
	eventually(t, h, "connected recipient", func() bool { return b.pc.ConnectionState() == pion.PeerConnectionStateConnected })
	b.neg.Lock()
	defer b.neg.Unlock()
	offer, err := b.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	if _, err = h.manager.Offer(context.Background(), b.id, uuid.NewString(), offer.SDP); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "missing Ready cleanup", func() bool { _, exists := h.manager.PeerBinding(b.id); return !exists })
	b.cancel()
}

func TestUndersizedReceiveOfferOnlyActivatesDeclaredSSRCs(t *testing.T) {
	h := harness(t, 5)
	conf := uuid.NewString()
	a, c := h.join(t, conf, ""), h.join(t, conf, "")
	eventually(t, h, "existing publishers", func() bool { return h.manager.Snapshot().Tracks == 4 })
	// Only two sendrecv publishing m-lines: there are no extra receive slots
	// to declare all four A/C publications in B's first answer.
	b := h.joinSlots(t, conf, "", 0)
	eventually(t, h, "all desired subscriptions registered", func() bool { return h.manager.Snapshot().Subscriptions == 12 })
	server, _ := h.manager.get(b.id)
	var omitted *subscription
	eventually(t, h, "undersized answer snapshot acknowledged", func() bool {
		server.mu.Lock()
		defer server.mu.Unlock()
		if !server.ready.Load() {
			return false
		}
		active, inactive := 0, 0
		for _, sub := range server.subscriptions {
			if sub.ready.Load() {
				active++
			} else {
				inactive++
				omitted = sub
			}
		}
		return active == 2 && inactive == 2
	})
	for _, p := range []*testPeer{a, b, c} {
		p.publishingPaused.Store(true)
	}
	time.Sleep(150 * time.Millisecond)
	packets := h.manager.Snapshot().Packets
	select {
	case omitted.queue <- &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 123, Timestamp: 456}, Payload: []byte{0x10, 1, 2, 3}}:
	case <-time.After(time.Second):
		t.Fatal("omitted sender queue blocked")
	}
	time.Sleep(50 * time.Millisecond)
	if h.manager.Snapshot().Packets != packets {
		t.Fatal("unbound sender produced phantom forwarded packet metrics")
	}
	b.neg.Lock()
	for _, kind := range []pion.RTPCodecType{pion.RTPCodecTypeAudio, pion.RTPCodecTypeVideo} {
		if _, err := b.pc.AddTransceiverFromKind(kind, pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly}); err != nil {
			b.neg.Unlock()
			t.Fatal(err)
		}
	}
	b.neg.Unlock()
	if err := b.negotiate(); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	for _, sub := range server.subscriptions {
		if !sub.ready.Load() {
			server.mu.Unlock()
			t.Fatal("expanded answer did not release declared subscription")
		}
	}
	server.mu.Unlock()
	for _, p := range []*testPeer{a, b, c} {
		p.publishingPaused.Store(false)
	}
	eventually(t, h, "expanded receive slots deliver existing media", func() bool { b.mu.Lock(); defer b.mu.Unlock(); return len(b.received) == 4 })
	a.close()
	b.close()
	c.close()
}

func TestAnsweredSSRCsExcludesInactiveRejectedAndRepair(t *testing.T) {
	raw := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96\r\na=sendonly\r\na=ssrc:100 cname:camera\r\na=ssrc:101 cname:camera\r\na=ssrc-group:FID 100 101\r\n" +
		"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=inactive\r\na=ssrc:200 cname:microphone\r\n" +
		"m=video 0 UDP/TLS/RTP/SAVPF 96\r\na=sendrecv\r\na=ssrc:300 cname:rejected\r\n" +
		"m=audio 9 UDP/TLS/RTP/SAVPF 111\r\na=recvonly\r\na=ssrc:400 cname:receive\r\n"
	ids, err := answeredSSRCs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || !ids[100] {
		t.Fatalf("not primary active sending SSRCs: %v", ids)
	}
	if _, err = answeredSSRCs("not SDP"); !errors.Is(err, media.ErrNegotiation) {
		t.Fatalf("malformed answer accepted: %v", err)
	}
	if _, err = answeredSSRCs(strings.Replace(raw, "ssrc:100", "ssrc:invalid", 1)); !errors.Is(err, media.ErrNegotiation) {
		t.Fatalf("malformed SSRC accepted: %v", err)
	}
}
