package webrtc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	pionrtp "github.com/pion/rtp"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// ErrNoMedia означает, что WebRTC-сессия не успела создать media-процесс.
var ErrNoMedia = errors.New("record media was not produced")

// Options собирает зависимости и настройки создания компонента.
// @params
//   - StoragePath: корневой каталог локального хранения артефактов записи.
//   - FFmpegPath: значение FFmpegPath типа string, используемое согласно назначению этой операции.
//   - UDPPort: значение UDPPort типа int, используемое согласно назначению этой операции.
//   - TCPPort: значение TCPPort типа int, используемое согласно назначению этой операции.
//   - NATIPs: набор значений NATIPs для последовательной или пакетной обработки.
//   - Logger: значение Logger типа *log.Logger, используемое согласно назначению этой операции.
//   - OnStarted: операция OnStarted с контрактом, описанным у метода.
//   - OnFailed: операция OnFailed с контрактом, описанным у метода.
type Options struct {
	StoragePath string
	FFmpegPath  string
	UDPPort     int
	TCPPort     int
	NATIPs      []string
	Logger      *log.Logger
	OnStarted   func(context.Context, string)
	OnFailed    func(context.Context, string, error)
	MaxSessions int // Максимум одновременных legacy-записей; 0 выбирает безопасный предел 2.
}

// Manager владеет локальными медиа-ресурсами и синхронизирует их создание, использование и завершение.
// @params
//   - api: значение api типа *pionwebrtc.API, используемое согласно назначению этой операции.
//   - storage: хранилище приватных файлов и метаданных объектов.
//   - recorder: значение recorder типа *ffmpeg.SegmentRecorder, используемое согласно назначению этой операции.
//   - logger: значение logger типа *log.Logger, используемое согласно назначению этой операции.
//   - iceConn: значение iceConn типа net.PacketConn, используемое согласно назначению этой операции.
//   - iceTCP: значение iceTCP типа net.Listener, используемое согласно назначению этой операции.
//   - onStarted: операция onStarted с контрактом, описанным у метода.
//   - onFailed: операция onFailed с контрактом, описанным у метода.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - sessions: хранилище и авторизация физических сессий подключения.
type Manager struct {
	api      *pionwebrtc.API
	storage  string
	recorder *ffmpeg.SegmentRecorder
	logger   *log.Logger
	iceConn  net.PacketConn
	iceTCP   net.Listener

	onStarted func(context.Context, string)
	onFailed  func(context.Context, string, error)

	mu          sync.Mutex
	sessions    map[string]*session
	closed      bool
	maxSessions int
}

// Active возвращает число сессий записи, которые требуют завершения перед обновлением.
func (m *Manager) Active() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.sessions) }

// NewManager создаёт менеджер WebRTC на основе Pion.
// @args
// - options: хранилище, FFmpeg, ICE и обработчики событий.
// @return Manager или ошибку настройки ICE.
func NewManager(options Options) (*Manager, error) {
	if options.MaxSessions <= 0 {
		options.MaxSessions = 2
	}
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
		// Сохраняем как обычные внешние IP, так и старый формат external/local.
		// Replace переписывает host-адреса только настроенного семейства IP.
		rules := make([]pionwebrtc.ICEAddressRewriteRule, 0, len(options.NATIPs)+1)
		externalIPs := make([]string, 0, len(options.NATIPs))
		for _, address := range options.NATIPs {
			external, local, mapped := strings.Cut(address, "/")
			externalIPs = append(externalIPs, external)
			if mapped {
				rules = append(rules, pionwebrtc.ICEAddressRewriteRule{
					External:        []string{external},
					Local:           local,
					AsCandidateType: pionwebrtc.ICECandidateTypeHost,
					Mode:            pionwebrtc.ICEAddressRewriteReplace,
				})
			}
		}
		rules = append(rules, pionwebrtc.ICEAddressRewriteRule{
			External:        externalIPs,
			AsCandidateType: pionwebrtc.ICECandidateTypeHost,
			Mode:            pionwebrtc.ICEAddressRewriteReplace,
		})
		if err := settingEngine.SetICEAddressRewriteRules(rules...); err != nil {
			if iceConn != nil {
				_ = iceConn.Close()
			}
			if iceTCP != nil {
				_ = iceTCP.Close()
			}
			return nil, fmt.Errorf("configure WebRTC ICE address rewriting: %w", err)
		}
		logger.Printf("WebRTC ICE NAT 1:1 IPs: %s", strings.Join(options.NATIPs, ","))
	}

	return &Manager{
		api: pionwebrtc.NewAPI(
			pionwebrtc.WithMediaEngine(mediaEngine),
			pionwebrtc.WithInterceptorRegistry(interceptors),
			pionwebrtc.WithSettingEngine(settingEngine),
		),
		storage:     options.StoragePath,
		recorder:    ffmpeg.NewSegmentRecorder(options.FFmpegPath, logger),
		logger:      logger,
		iceConn:     iceConn,
		iceTCP:      iceTCP,
		onStarted:   options.OnStarted,
		onFailed:    options.OnFailed,
		sessions:    make(map[string]*session),
		maxSessions: options.MaxSessions,
	}, nil
}

// Prepare создаёт сессию приёма для будущего SDP-предложения.
// @args
// - recordID: UUID записи.
// - segmentDurationSec: длительность сегмента.
// @return ошибку подготовки директории.
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
	if _, ok := m.sessions[recordID]; ok {
		// Повторные команды запуска не меняют конфигурацию действующей сессии.
		return nil
	}
	if m.closed || (m.maxSessions > 0 && len(m.sessions) >= m.maxSessions) {
		return fmt.Errorf("recorder draining or session limit reached")
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

// HandleOffer принимает SDP-предложение браузера и возвращает SDP-ответ.
// @args
// - ctx: контекст HTTP-запроса.
// - recordID: UUID записи.
// - request: SDP-предложение.
// @return SDP answer или ошибку signaling.
func (m *Manager) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	s, err := m.session(recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}

	return s.handleOffer(ctx, request)
}

// Stop завершает WebRTC/FFmpeg сессию.
// @args
// - recordID: UUID записи.
// @return ошибку остановки или ErrNoMedia.
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

// session находит подготовленную WebRTC-сессию записи по её идентификатору.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - recordID (string): внешний UUID задачи записи.
//
// @return:
//   - результат 1 (*session): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) session(recordID string) (*session, error) {
	m.mu.Lock()
	s, ok := m.sessions[recordID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("worker session is not prepared")
	}

	return s, nil
}

// session задаёт согласованное представление данных «сессия» для WebRTC-приёме медиа для записи.
// @params:
//   - recordID: внешний UUID задачи записи.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - manager: значение manager типа *Manager, используемое согласно назначению этой операции.
//   - recordDir: каталог локальных артефактов конкретной записи.
//   - tempDir: значение tempDir типа string, используемое согласно назначению этой операции.
//   - segmentDurationSec: плановая длительность сегмента записи в секундах.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - pc: значение pc типа *pionwebrtc.PeerConnection, используемое согласно назначению этой операции.
//   - process: значение process типа *ffmpeg.SegmentProcess, используемое согласно назначению этой операции.
//   - tracks: набор дорожек, входящих в операцию.
//   - expected: индекс значений expected для поиска и согласования состояния.
//   - received: индекс значений received для поиска и согласования состояния.
//   - processSince: временная отметка processSince; указатель допускает отсутствие значения.
//   - waitTimer: временная отметка waitTimer; указатель допускает отсутствие значения.
//   - processReady: канал «процесс готовность» для передачи данных или завершения ожидания.
//   - startOnce: значение startOnce типа sync.Once, используемое согласно назначению этой операции.
//   - failOnce: значение failOnce типа sync.Once, используемое согласно назначению этой операции.
//   - failed: логический признак failed, управляющий соответствующей веткой обработки.
type session struct {
	recordID           string
	ctx                context.Context
	cancel             context.CancelFunc
	manager            *Manager
	recordDir          string
	tempDir            string
	segmentDurationSec int

	mu            sync.Mutex
	pc            *pionwebrtc.PeerConnection
	process       *ffmpeg.SegmentProcess
	tracks        []ffmpeg.RTPTrack
	expected      map[string]int
	received      map[string]int
	processSince  time.Time
	waitTimer     *time.Timer
	processReady  chan struct{}
	startDone     chan struct{}
	startOnce     sync.Once
	failOnce      sync.Once
	failed        bool
	stopping      bool
	stopRequested bool
}

// handleOffer устанавливает удалённое SDP-описание и формирует ответ текущей WebRTC-сессии записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - request (records.WebRTCOfferRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (records.WebRTCAnswerResponse): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	if s.failed || s.stopRequested || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("WebRTC session has failed")
	}
	previous := s.pc
	s.pc = pc
	s.expected = expected
	s.received = make(map[string]int, len(expected))
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	pc.OnTrack( /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.

		@args
		  - track (*pionwebrtc.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
		  - _ (*pionwebrtc.RTPReceiver): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		*/func(track *pionwebrtc.TrackRemote, _ *pionwebrtc.RTPReceiver) {
			s.handleTrack(pc, track)
		})
	pc.OnConnectionStateChange( /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.

		@args
		  - state (pionwebrtc.PeerConnectionState): значение state типа pionwebrtc.PeerConnectionState, используемое согласно назначению этой операции.
		*/func(state pionwebrtc.PeerConnectionState) {
			s.manager.logger.Printf("record %s WebRTC state: %s", s.recordID, state.String())
			if state == pionwebrtc.PeerConnectionStateFailed {
				s.mu.Lock()
				current := s.pc == pc
				s.mu.Unlock()
				if current {
					s.fail(fmt.Errorf("webrtc connection failed"))
				}
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
	case <-s.ctx.Done():
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, s.ctx.Err()
	}
	s.mu.Lock()
	active := !s.failed && !s.stopRequested && !s.stopping && s.ctx.Err() == nil && s.pc == pc
	s.mu.Unlock()
	if !active {
		_ = pc.Close()
		return records.WebRTCAnswerResponse{}, fmt.Errorf("WebRTC session stopped during offer")
	}
	local := pc.LocalDescription()
	if local == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("local SDP answer is empty")
	}
	s.armTrackWaitTimeout(20 * time.Second)

	return records.WebRTCAnswerResponse{Type: "answer", SDP: local.SDP}, nil
}

// handleTrack обрабатывает появление входящей дорожки и подключает её к записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - track (*pionwebrtc.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
func (s *session) handleTrack(pc *pionwebrtc.PeerConnection, track *pionwebrtc.TrackRemote) {
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
	if s.failed || s.stopping || s.ctx.Err() != nil || s.pc != pc {
		s.mu.Unlock()
		return
	}
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
		s.startOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.

			 */func() {
				if err := s.startFFmpeg(); err != nil {
					s.fail(err)
				}
			})
	}
}

// startFFmpeg запускает процесс сегментной записи после готовности ожидаемых входных дорожек.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *session) startFFmpeg() error {
	s.mu.Lock()
	if s.failed || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		return fmt.Errorf("WebRTC session has failed")
	}
	if s.process != nil {
		s.mu.Unlock()
		return nil
	}
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	tracks := append([]ffmpeg.RTPTrack(nil), s.tracks...)
	s.startDone = make(chan struct{})
	startDone := s.startDone
	s.mu.Unlock()

	process, err := s.manager.recorder.Start(s.ctx, s.recordID, tracks, s.tempDir, s.recordDir, s.segmentDurationSec)
	if err != nil {
		close(startDone)
		return err
	}
	s.mu.Lock()
	if s.failed || s.stopping || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = process.Stop(time.Second)
		close(startDone)
		return fmt.Errorf("WebRTC session has failed")
	}
	s.process = process
	s.processSince = time.Now()
	processReady := s.processReady
	if processReady != nil {
		close(processReady)
	}
	close(startDone)
	s.mu.Unlock()
	go func() {
		<-process.Done()
		err := process.WaitErr()
		if err == nil {
			err = fmt.Errorf("FFmpeg exited before recording stop")
		}
		s.fail(fmt.Errorf("ffmpeg recording terminated: %w; stderr=%s", err, process.Stderr()))
	}()
	if s.manager.onStarted != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		s.manager.onStarted(ctx, s.recordID)
	}

	return nil
}

// requestVideoKeyframes периодически запрашивает ключевые видеокадры для декодирования и записи.
//
// @args
//   - track (*pionwebrtc.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
func (s *session) requestVideoKeyframes(track *pionwebrtc.TrackRemote) {
	if track.Kind() != pionwebrtc.RTPCodecTypeVideo {
		return
	}
	go /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.

	 */func() {
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

// writePLI отправляет RTCP PLI-запрос ключевого видеокадра.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - track (*pionwebrtc.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// forwardRTP пересылает входящие RTP-пакеты на локальный UDP-вход FFmpeg.
//
// @args
//   - track (*pionwebrtc.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
//   - port (int): локальный сетевой порт передачи RTP.
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
			if !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				s.manager.logger.Printf("record %s track read stopped: %v", s.recordID, err)
			}
			return
		}
		if err := writeRTPPacket(conn, packet); err != nil {
			if errors.Is(err, syscall.ECONNREFUSED) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			s.fail(fmt.Errorf("write RTP packet: %w", err))
			return
		}
	}
}

// minStopDelay вычисляет минимальную задержку остановки, необходимую для появления пригодного сегмента.
//
// @return:
//   - результат 1 (time.Duration): значение, подготовленное операцией для вызывающей стороны.
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

// waitBeforeStop ожидает минимальную длительность записи перед безопасной остановкой.
//
// @args
//   - startedAt (time.Time): момент начала обработки или записи.
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

// stop останавливает активную обработку ресурсов компонента и освобождает связанные ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *session) stop() error {
	s.mu.Lock()
	s.stopRequested = true
	hasProcess := s.process != nil
	if hasProcess {
		s.stopping = true
	}
	if s.waitTimer != nil {
		s.waitTimer.Stop()
		s.waitTimer = nil
	}
	s.mu.Unlock()
	if !hasProcess {
		// An already accepted PeerConnection may still deliver its first track.
		// Fence new offers while preserving the original bounded media grace.
		timer := time.NewTimer(s.minStopDelay())
		select {
		case <-s.processReady:
		case <-timer.C:
		case <-s.ctx.Done():
		}
		timer.Stop()
	}
	s.mu.Lock()
	s.stopping = true
	pc, process, processSince, startDone := s.pc, s.process, s.processSince, s.startDone
	s.mu.Unlock()
	if process == nil {
		s.cancel()
		// Startup owns an unregistered child until its completion is joined.
		if startDone != nil {
			<-startDone
		}
		if pc != nil {
			_ = pc.Close()
		}
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

// fail фиксирует ошибочное завершение и запускает предусмотренную очистку ресурса.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - err (error): ошибка, которую необходимо классифицировать, сохранить или вернуть клиенту.
func (s *session) fail(err error) {
	s.failOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
			s.mu.Lock()
			if s.failed || s.stopping || s.ctx.Err() != nil {
				s.mu.Unlock()
				return
			}
			s.failed = true
			pc := s.pc
			process := s.process
			startDone := s.startDone
			if s.waitTimer != nil {
				s.waitTimer.Stop()
				s.waitTimer = nil
			}
			s.mu.Unlock()
			// В обработчике сбоя останавливаем медиа до освобождения блокировки конференции.
			s.cancel()
			if pc != nil {
				_ = pc.Close()
			}
			if process != nil {
				_ = process.Stop(time.Second)
			} else if startDone != nil {
				<-startDone
			}
			s.manager.mu.Lock()
			if s.manager.sessions[s.recordID] == s {
				delete(s.manager.sessions, s.recordID)
			}
			s.manager.mu.Unlock()
			s.manager.logger.Printf("record %s ingest failed: %v", s.recordID, err)
			if s.manager.onFailed != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				s.manager.onFailed(ctx, s.recordID, err)
			}
		})
}

// armTrackWaitTimeout запускает таймер отказа, если ожидаемые медиа-дорожки не появились вовремя.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - timeout (time.Duration): максимальное время ожидания операции.
func (s *session) armTrackWaitTimeout(timeout time.Duration) {
	s.mu.Lock()
	if s.failed || s.stopRequested || s.stopping || s.ctx.Err() != nil || s.waitTimer != nil || s.process != nil {
		s.mu.Unlock()
		return
	}
	s.waitTimer = time.AfterFunc(timeout, /* Вложенный обработчик выполняет выделенный шаг обработки в WebRTC-приёме медиа для записи, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
			s.mu.Lock()
			hasProcess := s.process != nil
			s.mu.Unlock()
			if !hasProcess {
				s.fail(fmt.Errorf("%w: expected media tracks were not received", ErrNoMedia))
			}
		})
	s.mu.Unlock()
}

// hasExpectedTracksLocked проверяет готовность ожидаемых дорожек при удерживаемой блокировке сессии.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
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

// offerMediaKinds подсчитывает виды медиа-секций в предложенном SDP.
//
// @args
//   - sdp (string): описание согласуемого WebRTC-сеанса в формате SDP.
//
// @return:
//   - результат 1 (map[string]int): значение, подготовленное операцией для вызывающей стороны.
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

// codecKind преобразует вид кодека Pion в строковое представление.
//
// @args
//   - kind (pionwebrtc.RTPCodecType): тип события, ошибки или медиа, определяющий ветку обработки.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func codecKind(kind pionwebrtc.RTPCodecType) string {
	if kind == pionwebrtc.RTPCodecTypeAudio {
		return "audio"
	}

	return "video"
}

// reserveUDPPort выбирает доступный локальный UDP-порт для RTP-входа.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// writeRTPPacket сериализует RTP-пакет и передаёт его в UDP-соединение.
//
// @args
//   - conn (*net.UDPConn): действующее сетевое соединение операции.
//   - packet (*pionrtp.Packet): закодированный RTP- или управляющий пакет.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func writeRTPPacket(conn *net.UDPConn, packet *pionrtp.Packet) error {
	raw, err := packet.Marshal()
	if err != nil {
		return err
	}
	_, err = conn.Write(raw)

	return err
}
