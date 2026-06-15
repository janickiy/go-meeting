package webrtc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"git.svc-dev.net/board/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	pionrtp "github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// ErrNoMedia означает, что WebRTC-сессия не успела создать media-процесс.
var ErrNoMedia = errors.New("record media was not produced")

// Options содержит настройки WebRTC worker-а.
type Options struct {
	StoragePath string
	FFmpegPath  string
	UDPPort     int
	TCPPort     int
	NATIPs      []string
	Logger      *log.Logger
	OnStarted   func(context.Context, string)
	OnFailed    func(context.Context, string, error)
}

// Manager управляет WebRTC ingest-сессиями.
type Manager struct {
	api      *pionwebrtc.API
	storage  string
	recorder *ffmpeg.SegmentRecorder
	logger   *log.Logger
	iceConn  net.PacketConn
	iceTCP   net.Listener

	onStarted func(context.Context, string)
	onFailed  func(context.Context, string, error)

	mu       sync.Mutex
	sessions map[string]*session
}

// NewManager создает Pion WebRTC manager.
// Параметры:
// - options: storage, FFmpeg, ICE и callbacks.
// Возвращает: Manager или ошибку настройки ICE.
func NewManager(options Options) (*Manager, error) {
	logger := options.Logger
	if logger == nil {
		logger = log.Default()
	}
	mediaEngine := &pionwebrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register codecs: %w", err)
	}
	interceptors := &interceptor.Registry{}
	if err := pionwebrtc.RegisterDefaultInterceptors(mediaEngine, interceptors); err != nil {
		return nil, fmt.Errorf("register interceptors: %w", err)
	}
	settingEngine := pionwebrtc.SettingEngine{}
	var iceConn net.PacketConn
	var iceTCP net.Listener
	networkTypes := make([]pionwebrtc.NetworkType, 0, 2)
	if options.UDPPort > 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: options.UDPPort})
		if err != nil {
			return nil, fmt.Errorf("listen WebRTC UDP mux: %w", err)
		}
		iceConn = conn
		networkTypes = append(networkTypes, pionwebrtc.NetworkTypeUDP4)
		settingEngine.SetICEUDPMux(pionwebrtc.NewICEUDPMux(nil, conn))
		logger.Printf("WebRTC ICE UDP mux is listening on :%d/udp", options.UDPPort)
	}
	if options.TCPPort > 0 {
		listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("0.0.0.0"), Port: options.TCPPort})
		if err != nil {
			if iceConn != nil {
				_ = iceConn.Close()
			}
			return nil, fmt.Errorf("listen WebRTC TCP mux: %w", err)
		}
		iceTCP = listener
		networkTypes = append(networkTypes, pionwebrtc.NetworkTypeTCP4)
		settingEngine.SetICETCPMux(pionwebrtc.NewICETCPMux(nil, listener, 8))
		logger.Printf("WebRTC ICE TCP mux is listening on :%d/tcp", options.TCPPort)
	}
	if len(networkTypes) > 0 {
		settingEngine.SetNetworkTypes(networkTypes)
	}
	if len(options.NATIPs) > 0 {
		settingEngine.SetNAT1To1IPs(options.NATIPs, pionwebrtc.ICECandidateTypeHost)
		logger.Printf("WebRTC ICE NAT 1:1 IPs: %s", strings.Join(options.NATIPs, ","))
	}

	return &Manager{
		api: pionwebrtc.NewAPI(
			pionwebrtc.WithMediaEngine(mediaEngine),
			pionwebrtc.WithInterceptorRegistry(interceptors),
			pionwebrtc.WithSettingEngine(settingEngine),
		),
		storage:   options.StoragePath,
		recorder:  ffmpeg.NewSegmentRecorder(options.FFmpegPath, logger),
		logger:    logger,
		iceConn:   iceConn,
		iceTCP:    iceTCP,
		onStarted: options.OnStarted,
		onFailed:  options.OnFailed,
		sessions:  make(map[string]*session),
	}, nil
}

// Prepare создает ingest-сессию под будущий SDP offer.
// Параметры:
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента.
// Возвращает: ошибку подготовки директории.
func (m *Manager) Prepare(recordID string, segmentDurationSec int) error {
	if segmentDurationSec <= 0 {
		segmentDurationSec = 5
	}
	recordDir := filepath.Join(m.storage, "records", recordID)
	tempDir := filepath.Join(m.storage, "tmp", recordID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.sessions[recordID]; ok {
		existing.segmentDurationSec = segmentDurationSec
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.sessions[recordID] = &session{
		recordID:           recordID,
		ctx:                ctx,
		cancel:             cancel,
		manager:            m,
		recordDir:          recordDir,
		tempDir:            tempDir,
		segmentDurationSec: segmentDurationSec,
		processReady:       make(chan struct{}),
	}

	return nil
}

// HandleOffer принимает browser SDP offer и возвращает SDP answer.
// Параметры:
// - ctx: HTTP context.
// - recordID: UUID записи.
// - request: SDP offer.
// Возвращает: SDP answer или ошибку signaling.
func (m *Manager) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	s, err := m.session(recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return s.handleOffer(ctx, request)
}

// Stop завершает WebRTC/FFmpeg сессию.
// Параметры:
// - recordID: UUID записи.
// Возвращает: ошибку остановки или ErrNoMedia.
func (m *Manager) Stop(recordID string) error {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	if ok {
		delete(m.sessions, recordID)
	}
	m.mu.Unlock()
	if !ok {
		return ErrNoMedia
	}

	return s.stop()
}

func (m *Manager) session(recordID string) (*session, error) {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("worker session is not prepared")
	}

	return s, nil
}

type session struct {
	recordID           string
	ctx                context.Context
	cancel             context.CancelFunc
	manager            *Manager
	recordDir          string
	tempDir            string
	segmentDurationSec int

	mu           sync.Mutex
	pc           *pionwebrtc.PeerConnection
	process      *ffmpeg.SegmentProcess
	tracks       []ffmpeg.RTPTrack
	expected     map[string]int
	received     map[string]int
	processSince time.Time
	waitTimer    *time.Timer
	processReady chan struct{}
	startOnce    sync.Once
	failOnce     sync.Once
}

func (s *session) handleOffer(ctx context.Context, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	if request.Type == "" {
		request.Type = "offer"
	}
	if strings.TrimSpace(request.SDP) == "" {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("sdp is required")
	}
	expected := offerMediaKinds(request.SDP)
	if len(expected) == 0 {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("offer must contain audio or video media section")
	}
	pc, err := s.manager.api.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("create peer connection: %w", err)
	}

	s.mu.Lock()
	if s.pc != nil {
		_ = s.pc.Close()
	}
	s.pc = pc
	s.expected = expected
	s.received = make(map[string]int, len(expected))
	s.mu.Unlock()

	pc.OnTrack(func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
		s.handleTrack(track)
	})
	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		s.manager.logger.Printf("record %s WebRTC state: %s", s.recordID, state.String())
		if state == pionwebrtc.PeerConnectionStateFailed {
			s.fail(fmt.Errorf("webrtc connection failed"))
		}
	})

	offer := pionwebrtc.SessionDescription{Type: pionwebrtc.SDPTypeOffer, SDP: request.SDP}
	if err := pc.SetRemoteDescription(offer); err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("set remote description: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("create answer: %w", err)
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("set local description: %w", err)
	}

	select {
	case <-pionwebrtc.GatheringCompletePromise(pc):
	case <-time.After(3 * time.Second):
		s.manager.logger.Printf("record %s WebRTC answer ICE gathering timeout; returning partial answer", s.recordID)
	case <-ctx.Done():
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, ctx.Err()
	}
	local := pc.LocalDescription()
	if local == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("local SDP answer is empty")
	}
	s.armTrackWaitTimeout(20 * time.Second)

	return records.WebRTCAnswerResponse{Type: "answer", SDP: local.SDP}, nil
}

func (s *session) handleTrack(track *pionwebrtc.TrackRemote) {
	port, err := reserveUDPPort()
	if err != nil {
		s.fail(fmt.Errorf("reserve RTP port: %w", err))
		return
	}
	codec := track.Codec()
	ffTrack := ffmpeg.RTPTrack{
		Kind:        codecKind(track.Kind()),
		MimeType:    codec.MimeType,
		PayloadType: uint8(codec.PayloadType),
		ClockRate:   codec.ClockRate,
		Channels:    uint16(codec.Channels),
		Port:        port,
		FmtpLine:    codec.SDPFmtpLine,
	}

	s.mu.Lock()
	s.tracks = append(s.tracks, ffTrack)
	if s.received == nil {
		s.received = make(map[string]int)
	}
	s.received[ffTrack.Kind]++
	ready := s.hasExpectedTracksLocked()
	s.mu.Unlock()

	s.manager.logger.Printf("record %s received %s track codec=%s port=%d", s.recordID, ffTrack.Kind, ffTrack.MimeType, port)
	go s.forwardRTP(track, port)
	s.requestVideoKeyframes(track)

	if ready {
		s.startOnce.Do(func() {
			if err := s.startFFmpeg(); err != nil {
				s.fail(err)
			}
		})
	}
}

func (s *session) startFFmpeg() error {
	s.mu.Lock()
	if s.process != nil {
		s.mu.Unlock()
		return nil
	}
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	tracks := append([]ffmpeg.RTPTrack(nil), s.tracks...)
	s.mu.Unlock()

	process, err := s.manager.recorder.Start(s.ctx, s.recordID, tracks, s.tempDir, s.recordDir, s.segmentDurationSec)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.process = process
	s.processSince = time.Now()
	processReady := s.processReady
	s.mu.Unlock()
	if processReady != nil {
		close(processReady)
	}
	if s.manager.onStarted != nil {
		s.manager.onStarted(context.Background(), s.recordID)
	}

	return nil
}

func (s *session) requestVideoKeyframes(track *pionwebrtc.TrackRemote) {
	if track.Kind() != pionwebrtc.RTPCodecTypeVideo {
		return
	}
	go func() {
		select {
		case <-s.processReady:
		case <-s.ctx.Done():
			return
		}
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			if err := s.writePLI(track); err != nil {
				s.manager.logger.Printf("record %s request video keyframe failed: %v", s.recordID, err)
			}
			select {
			case <-ticker.C:
			case <-timer.C:
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()
}

func (s *session) writePLI(track *pionwebrtc.TrackRemote) error {
	s.mu.Lock()
	pc := s.pc
	s.mu.Unlock()
	if pc == nil {
		return nil
	}

	return pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: uint32(track.SSRC())},
	})
}

func (s *session) forwardRTP(track *pionwebrtc.TrackRemote, port int) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		s.fail(fmt.Errorf("resolve RTP UDP addr: %w", err))
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		s.fail(fmt.Errorf("dial RTP UDP addr: %w", err))
		return
	}
	defer conn.Close()

	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "closed") {
				s.manager.logger.Printf("record %s track read stopped: %v", s.recordID, err)
			}
			return
		}
		if err := writeRTPPacket(conn, packet); err != nil {
			if strings.Contains(err.Error(), "connection refused") {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			s.fail(fmt.Errorf("write RTP packet: %w", err))
			return
		}
	}
}

func (s *session) minStopDelay() time.Duration {
	duration := time.Duration(s.segmentDurationSec) * time.Second
	if duration < 2*time.Second {
		duration = 2 * time.Second
	}
	if duration > 8*time.Second {
		duration = 8 * time.Second
	}

	return duration
}

func (s *session) waitBeforeStop(startedAt time.Time) {
	if startedAt.IsZero() {
		return
	}
	wait := time.Until(startedAt.Add(s.minStopDelay()))
	if wait <= 0 {
		return
	}
	s.manager.logger.Printf("record %s stop requested before first segment window; waiting %s before FFmpeg stop", s.recordID, wait.Round(100*time.Millisecond))
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.ctx.Done():
	}
}

func (s *session) waitForProcessBeforeStop(timeout time.Duration) (*ffmpeg.SegmentProcess, time.Time) {
	if timeout <= 0 {
		return nil, time.Time{}
	}
	s.manager.logger.Printf("record %s stop requested before FFmpeg media process; waiting %s for tracks", s.recordID, timeout)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.processReady:
		s.mu.Lock()
		process := s.process
		processSince := s.processSince
		s.mu.Unlock()
		return process, processSince
	case <-timer.C:
		return nil, time.Time{}
	case <-s.ctx.Done():
		return nil, time.Time{}
	}
}

func (s *session) stop() error {
	s.mu.Lock()
	pc := s.pc
	process := s.process
	processSince := s.processSince
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	s.mu.Unlock()
	if process == nil {
		process, processSince = s.waitForProcessBeforeStop(s.minStopDelay())
	}
	if process == nil {
		if pc != nil {
			_ = pc.Close()
		}
		s.cancel()
		return ErrNoMedia
	}
	s.waitBeforeStop(processSince)
	if err := process.Stop(10 * time.Second); err != nil {
		if pc != nil {
			_ = pc.Close()
		}
		s.cancel()
		return err
	}
	if pc != nil {
		_ = pc.Close()
	}
	s.cancel()

	return nil
}

func (s *session) fail(err error) {
	s.failOnce.Do(func() {
		s.manager.logger.Printf("record %s ingest failed: %v", s.recordID, err)
		if s.manager.onFailed != nil {
			s.manager.onFailed(context.Background(), s.recordID, err)
		}
	})
}

func (s *session) armTrackWaitTimeout(timeout time.Duration) {
	s.mu.Lock()
	if s.waitTimer != nil || s.process != nil {
		s.mu.Unlock()
		return
	}
	s.waitTimer = time.AfterFunc(timeout, func() {
		s.mu.Lock()
		hasProcess := s.process != nil
		s.mu.Unlock()
		if !hasProcess {
			s.fail(fmt.Errorf("%w: expected media tracks were not received", ErrNoMedia))
		}
	})
	s.mu.Unlock()
}

func (s *session) hasExpectedTracksLocked() bool {
	if len(s.expected) == 0 {
		return len(s.tracks) > 0
	}
	for kind, expectedCount := range s.expected {
		if expectedCount > 0 && s.received[kind] == 0 {
			return false
		}
	}

	return true
}

func offerMediaKinds(sdp string) map[string]int {
	result := make(map[string]int)
	for _, rawLine := range strings.Split(sdp, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "m=") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "m="))
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "audio" || fields[0] == "video" {
			result[fields[0]]++
		}
	}

	return result
}

func codecKind(kind pionwebrtc.RTPCodecType) string {
	if kind == pionwebrtc.RTPCodecTypeAudio {
		return "audio"
	}

	return "video"
}

func reserveUDPPort() (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

func writeRTPPacket(conn *net.UDPConn, packet *pionrtp.Packet) error {
	raw, err := packet.Marshal()
	if err != nil {
		return err
	}
	_, err = conn.Write(raw)

	return err
}
