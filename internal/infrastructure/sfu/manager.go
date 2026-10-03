// Пакет sfu отвечает за транспорт медиа. Прикладные права, билеты и распределённое
// владение проверяются внутренними обработчиками управления media-worker.
package sfu

import (
	"context"
	"fmt"
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

// Options собирает зависимости и настройки создания компонента.
// @params
//   - WorkerID: идентификатор воркера-владельца операции.
//   - ICE: набор значений ICE для последовательной или пакетной обработки.
//   - UDPPort: значение UDPPort типа int, используемое согласно назначению этой операции.
//   - UDPMinPort: значение UDPMinPort типа int, используемое согласно назначению этой операции.
//   - UDPMaxPort: значение UDPMaxPort типа int, используемое согласно назначению этой операции.
//   - TCPPort: значение TCPPort типа int, используемое согласно назначению этой операции.
//   - NATIPs: набор значений NATIPs для последовательной или пакетной обработки.
//   - MaxPeers: значение MaxPeers типа int, используемое согласно назначению этой операции.
//   - MaxRooms: значение MaxRooms типа int, используемое согласно назначению этой операции.
//   - MaxPublishedTracks: значение MaxPublishedTracks типа int, используемое согласно назначению этой операции.
//   - MaxAudioTracks: значение MaxAudioTracks типа int, используемое согласно назначению этой операции.
//   - MaxVideoTracks: значение MaxVideoTracks типа int, используемое согласно назначению этой операции.
//   - QueueSize: значение QueueSize типа int, используемое согласно назначению этой операции.
//   - MaxScreenSharers: значение MaxScreenSharers типа int, используемое согласно назначению этой операции.
//   - EgressQueueSize: значение EgressQueueSize типа int, используемое согласно назначению этой операции.
//   - ICEDisconnectedTimeout: значение ICEDisconnectedTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - ICEFailedTimeout: значение ICEFailedTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - ICEKeepaliveInterval: значение ICEKeepaliveInterval типа time.Duration, используемое согласно назначению этой операции.
//   - NegotiationTimeout: значение NegotiationTimeout типа time.Duration, используемое согласно назначению этой операции.
//   - Logger: значение Logger типа *slog.Logger, используемое согласно назначению этой операции.
//   - Emit: операция Emit с контрактом, описанным у метода.
type Options struct {
	WorkerID                                                                           string
	ICE                                                                                []pion.ICEServer
	UDPPort, UDPMinPort, UDPMaxPort, TCPPort                                           int
	NATIPs                                                                             []string
	MaxPeers, MaxRooms, MaxPublishedTracks, MaxAudioTracks, MaxVideoTracks, QueueSize  int
	MaxScreenSharers, EgressQueueSize                                                  int
	ICEDisconnectedTimeout, ICEFailedTimeout, ICEKeepaliveInterval, NegotiationTimeout time.Duration
	Logger                                                                             *slog.Logger
	// Реализация Emit должна быть ограниченной. Она вызывается без блокировок SFU
	// собственным ограниченным обработчиком событий соединения, вне цикла пересылки RTP.
	// Операции жизненного цикла, запущенные из Emit, планируются асинхронно:
	// синхронный Leave ожидал бы тот обработчик событий, который сейчас вызывает Emit.
	Emit func(media.Binding, string, any)
}

// Stats собирает счётчики подключений, дорожек и ограниченных очередей для диагностики.
// @params
//   - Rooms: значение Rooms типа int, используемое согласно назначению этой операции.
//   - Peers: значение Peers типа int, используемое согласно назначению этой операции.
//   - Tracks: набор дорожек, входящих в операцию.
//   - Subscriptions: значение Subscriptions типа int, используемое согласно назначению этой операции.
//   - PeerConnections: значение PeerConnections типа uint64, используемое согласно назначению этой операции.
//   - Failures: значение Failures типа uint64, используемое согласно назначению этой операции.
//   - Packets: значение Packets типа uint64, используемое согласно назначению этой операции.
//   - Bytes: значение Bytes типа uint64, используемое согласно назначению этой операции.
//   - Dropped: значение Dropped типа uint64, используемое согласно назначению этой операции.
//   - RecordingOutputs: значение RecordingOutputs типа int, используемое согласно назначению этой операции.
//   - RecordingDrops: значение RecordingDrops типа uint64, используемое согласно назначению этой операции.
type Stats struct {
	Rooms             int    `json:"roomsActive"`
	Peers             int    `json:"mediaPeersActive"`
	Tracks            int    `json:"tracksPublished"`
	Subscriptions     int    `json:"subscriptions"`
	PeerConnections   uint64 `json:"peerConnectionTotal"`
	Failures          uint64 `json:"peerConnectionFailures"`
	RelayConnections  uint64 `json:"relayConnections"`  // выбранные пары с TURN, накопительный счётчик
	DirectConnections uint64 `json:"directConnections"` // выбранные пары без TURN, накопительный счётчик
	Packets           uint64 `json:"packetsForwarded"`
	Bytes             uint64 `json:"bytesForwarded"`
	Dropped           uint64 `json:"packetsDropped"`
	RecordingOutputs  int    `json:"recordingOutputs"`
	RecordingDrops    uint64 `json:"recordingDrops"`
	AudioTaps         int    `json:"audioTaps"`
	AudioTapDrops     uint64 `json:"audioTapDrops"`
}

// Manager владеет локальными медиа-ресурсами и синхронизирует их создание, использование и завершение.
// @params
//   - opts: значение opts типа Options, используемое согласно назначению этой операции.
//   - api: значение api типа *pion.API, используемое согласно назначению этой операции.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - rooms: индекс значений rooms для поиска и согласования состояния.
//   - peers: индекс значений peers для поиска и согласования состояния.
//   - connections: индекс значений connections для поиска и согласования состояния.
//   - roomClosures: канал завершения закрываемых комнат.
//   - closing: логический признак closing, управляющий соответствующей веткой обработки.
//   - closed: канал «закрытый» для передачи данных или завершения ожидания.
//   - shutdownOnce: значение shutdownOnce типа sync.Once, используемое согласно назначению этой операции.
//   - closeWG: значение closeWG типа sync.WaitGroup, используемое согласно назначению этой операции.
//   - udp: значение udp типа net.PacketConn, используемое согласно назначению этой операции.
//   - tcp: значение tcp типа net.Listener, используемое согласно назначению этой операции.
//   - total: значение total типа atomic.Uint64, используемое согласно назначению этой операции.
//   - failures: значение failures типа atomic.Uint64, используемое согласно назначению этой операции.
//   - packets: значение packets типа atomic.Uint64, используемое согласно назначению этой операции.
//   - bytes: значение bytes типа atomic.Uint64, используемое согласно назначению этой операции.
//   - dropped: значение dropped типа atomic.Uint64, используемое согласно назначению этой операции.
//   - egressDropped: значение egressDropped типа atomic.Uint64, используемое согласно назначению этой операции.
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
	egressDropped                            atomic.Uint64
	audioTapDropped                          atomic.Uint64
	relay, direct                            atomic.Uint64
}

// room объединяет пиры и публикации одной локальной SFU-комнаты и её жизненный цикл.
// @params
//   - id: идентификатор обрабатываемого ресурса.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - peers: индекс значений peers для поиска и согласования состояния.
//   - tracks: набор дорожек, входящих в операцию.
//   - policies: индекс значений policies для поиска и согласования состояния.
//   - screens: индекс значений screens для поиска и согласования состояния.
//   - egresses: индекс значений egresses для поиска и согласования состояния.
type room struct {
	id       string
	mu       sync.Mutex
	peers    map[string]*peer
	tracks   map[string]*publishedTrack
	policies map[string]media.ParticipantPolicy
	screens  map[string]bool
	egresses map[string]*egress
}

// defaults подставляет допустимые значения по умолчанию для отсутствующих настроек.
//
// @args
//   - o (Options): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (Options): значение, подготовленное операцией для вызывающей стороны.
func defaults(o Options) Options {
	if o.MaxPeers == 0 {
		o.MaxPeers = 10
	}
	if o.MaxRooms == 0 {
		o.MaxRooms = 100
	}
	if o.MaxPublishedTracks == 0 {
		o.MaxPublishedTracks = 4
	}
	if o.MaxAudioTracks == 0 {
		o.MaxAudioTracks = 2
	}
	if o.MaxVideoTracks == 0 {
		o.MaxVideoTracks = 2
	}
	if o.MaxScreenSharers == 0 {
		o.MaxScreenSharers = 1
	}
	if o.EgressQueueSize == 0 {
		o.EgressQueueSize = 2048
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

// NewManager создаёт и связывает зависимости компонента Manager, используемого в пересылке WebRTC-медиа через SFU.
//
// @args
//   - options (Options): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (*Manager): созданный компонент с переданными зависимостями.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NewManager(options Options) (*Manager, error) {
	o := defaults(options)
	if o.MaxPeers < 2 || o.MaxPeers > 100 || o.MaxRooms < 1 || o.MaxRooms > 10000 ||
		o.MaxPublishedTracks < 1 || o.MaxPublishedTracks > 4 || o.MaxAudioTracks < 1 || o.MaxAudioTracks > 2 || o.MaxVideoTracks < 1 || o.MaxVideoTracks > 2 ||
		o.MaxScreenSharers < 1 || o.MaxScreenSharers > 4 || o.EgressQueueSize < 1 || o.EgressQueueSize > 8192 ||
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
	// Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
	//
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
		// Replace сохраняет NAT 1:1 для используемых SFU IPv4 host-кандидатов.
		if err := settings.SetICEAddressRewriteRules(pion.ICEAddressRewriteRule{
			External:        o.NATIPs,
			AsCandidateType: pion.ICECandidateTypeHost,
			Mode:            pion.ICEAddressRewriteReplace,
		}); err != nil {
			cleanup()
			return nil, fmt.Errorf("configure SFU ICE address rewriting: %w", err)
		}
	}
	m.api = pion.NewAPI(pion.WithMediaEngine(engine), pion.WithInterceptorRegistry(registry), pion.WithSettingEngine(settings))
	// Служебный элемент защищает добавление в WaitGroup, пока Shutdown не отсоединит
	// все зарегистрированные соединения; соединение добавляется в closeWG до удаления из карты.
	m.closeWG.Add(1)
	return m, nil
}

// Join создаёт или возвращает медиа-пир комнаты для проверенной серверной идентичности.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - binding (media.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
//
// @return:
//   - результат 1 (media.PeerView): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	// Соединение создаётся вне общей блокировки, но Shutdown должен дождаться, пока
	// выполняющийся NewPeerConnection будет зарегистрирован или закрыт.
	m.closeWG.Add(1)
	m.mu.Unlock()
	defer m.closeWG.Done()
	pc, err := m.api.NewPeerConnection(pion.Configuration{ICEServers: m.opts.ICE})
	if err != nil {
		return media.PeerView{}, media.ErrUnavailable
	}
	ctxPeer, cancel := context.WithCancel(context.Background())
	p := &peer{id: uuid.NewString(), binding: binding, manager: m, pc: pc, ctx: ctxPeer, cancel: cancel,
		publications: map[string]*publishedTrack{}, subscriptions: map[string]*subscription{}, receivers: map[string]*receiver{},
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
		r = newRoom(binding.ConferenceID)
		m.rooms[r.id] = r
	}
	r.mu.Lock()
	if r.policies[binding.ParticipantID].Kicked {
		r.mu.Unlock()
		m.mu.Unlock()
		cancel()
		_ = pc.Close()
		return media.PeerView{}, media.ErrPolicy
	}
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
	// Настраиваем обработчики и запускаем фоновые задачи после регистрации под защитой от Shutdown.
	p.install()
	m.mu.Unlock()
	m.syncPeer(p)
	m.log(p, "media_join", nil)
	return p.view(), nil
}

// sameBinding сравнивает серверную идентичность двух медиа-подключений.
//
// @args
//   - a (media.Binding): значение a типа media.Binding, используемое согласно назначению этой операции.
//   - b (media.Binding): контекст измерения производительности теста.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func sameBinding(a, b media.Binding) bool {
	return a.ConferenceID == b.ConferenceID && a.ParticipantID == b.ParticipantID && a.SessionID == b.SessionID && a.ConnectionID == b.ConnectionID && a.UserID == b.UserID
}

// get читает состояние ресурсов компонента для дальнейшей обработки или ответа.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (*peer): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) get(id string) (*peer, error) {
	m.mu.Lock()
	p := m.peers[id]
	m.mu.Unlock()
	if p == nil {
		return nil, media.ErrPeerNotFound
	}
	return p, nil
}

// PeerBinding возвращает серверную идентичность медиа-пира по физическому соединению.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (media.Binding): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
func (m *Manager) PeerBinding(id string) (media.Binding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.peers[id]
	if p == nil {
		return media.Binding{}, false
	}
	return p.binding, true
}

// Bindings возвращает серверные идентичности подключений в комнате.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 ([]media.Binding): собранные элементы результата; состав ограничивается параметрами операции.
func (m *Manager) Bindings() []media.Binding {
	m.mu.Lock()
	defer m.mu.Unlock()
	bindings := make([]media.Binding, 0, len(m.peers))
	for _, p := range m.peers {
		bindings = append(bindings, p.binding)
	}
	return bindings
}

// Leave отключает медиа-пир и освобождает связанные публикации и подписки.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - _ (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// detachLocked помечает отсоединяемый ресурс и отменяет его активность под блокировкой менеджера до сетевой очистки.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
func (m *Manager) detachLocked(p *peer) {
	m.closeWG.Add(1)
	p.mu.Lock()
	p.closing = true
	p.cancel()
	delete(m.peers, p.id)
	delete(m.connections, p.binding.ConnectionID)
	p.room.mu.Lock()
	delete(p.room.peers, p.id)
	delete(p.room.screens, p.id)
	empty := len(p.room.peers) == 0 && len(p.room.egresses) == 0
	p.room.mu.Unlock()
	p.mu.Unlock()
	if empty {
		delete(m.rooms, p.room.id)
	}
}

// finishDetached освобождает отсоединённые медиа-ресурсы вне блокировок менеджера.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
func (m *Manager) finishDetached(p *peer) {
	defer m.closeWG.Done()
	p.close()
	m.log(p, "media_leave", nil)
}

// LeaveConnection отключает только указанное физическое медиа-соединение.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - connectionID (string): идентификатор физического медиа-соединения.
func (m *Manager) LeaveConnection(ctx context.Context, connectionID string) {
	m.mu.Lock()
	p := m.connections[connectionID]
	m.mu.Unlock()
	if p != nil {
		_ = m.Leave(ctx, p.id)
	}
}

// CloseConference закрывает все медиа-подключения и ресурсы конкретной конференции.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	if r := m.rooms[id]; r != nil {
		r.mu.Lock()
		for _, e := range r.egresses {
			e.fail(media.ErrUnavailable)
		}
		r.egresses = map[string]*egress{}
		if len(r.peers) == 0 {
			delete(m.rooms, id)
		}
		r.mu.Unlock()
	}
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
	// Учитываем координатор и очистку соединений; Shutdown ждёт все закрытия конференций,
	// которые начались до установки общего признака завершения.
	m.closeWG.Add(1)
	for _, p := range peers {
		m.detachLocked(p)
	}
	m.mu.Unlock()
	go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
	Синхронизирует доступ к разделяемому состоянию блокировкой.

	*/func() {
		defer m.closeWG.Done()
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

			@args
			  - p (*peer): байты, переданные по контракту io.Writer.
			*/func(p *peer) { defer wg.Done(); m.finishDetached(p) }(p)
		}
		wg.Wait()
		m.mu.Lock()
		delete(m.roomClosures, id)
		m.mu.Unlock()
		close(done)
	}()
	return waitClosed(ctx, done)
}

// Shutdown останавливает менеджер и ожидает завершения принадлежащих ему ресурсов.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
			m.mu.Lock()
			m.closing = true
			for _, r := range m.rooms {
				r.mu.Lock()
				for _, e := range r.egresses {
					e.fail(media.ErrUnavailable)
				}
				r.egresses = map[string]*egress{}
				if len(r.peers) == 0 {
					delete(m.rooms, r.id)
				}
				r.mu.Unlock()
			}
			peers := make([]*peer, 0, len(m.peers))
			for _, p := range m.peers {
				peers = append(peers, p)
			}
			for _, p := range peers {
				m.detachLocked(p)
			}
			m.mu.Unlock()
			go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

			 */func() {
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

// waitClosed ожидает закрытия всех заданных ресурсов в пределах контекста.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - done (<-chan struct{}): канал «done» для передачи данных или завершения ожидания.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func waitClosed(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Snapshot возвращает согласованный снимок состояния и счётчиков медиа-менеджера.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (Stats): значение, подготовленное операцией для вызывающей стороны.
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
		for _, e := range r.egresses {
			if e.audioOnly {
				s.AudioTaps++
			} else {
				s.RecordingOutputs++
			}
		}
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
	s.RelayConnections = m.relay.Load()
	s.DirectConnections = m.direct.Load()
	s.Packets = m.packets.Load()
	s.Bytes = m.bytes.Load()
	s.Dropped = m.dropped.Load()
	s.RecordingDrops = m.egressDropped.Load()
	s.AudioTapDrops = m.audioTapDropped.Load()
	return s
}

// newRoom создаёт изолированное состояние медиа-комнаты и управление её жизненным циклом.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (*room): значение, подготовленное операцией для вызывающей стороны.
func newRoom(id string) *room {
	return &room{id: id, peers: map[string]*peer{}, tracks: map[string]*publishedTrack{}, policies: map[string]media.ParticipantPolicy{}, screens: map[string]bool{}, egresses: map[string]*egress{}}
}

// HasConference проверяет наличие локальной медиа-комнаты конференции.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (m *Manager) HasConference(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[id] != nil
}

// log записывает ограниченную диагностику компонента с указанными параметрами.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
//   - event (string): конверт входящего или публикуемого события.
//   - fields ([]any): набор значений fields для последовательной или пакетной обработки.
func (m *Manager) log(p *peer, event string, fields []any) {
	args := []any{"worker_id", m.opts.WorkerID, "conference_id", p.binding.ConferenceID, "participant_id", p.binding.ParticipantID, "session_id", p.binding.SessionID, "connection_id", p.binding.ConnectionID, "media_peer_id", p.id, "event_type", event}
	args = append(args, fields...)
	m.opts.Logger.Info("media event", args...)
}

// supported проверяет поддерживаемые возможности медиа-подключения.
//
// @args
//   - track (*pion.TrackRemote): медиа-дорожка, которую обрабатывает или подписывает компонент.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func supported(track *pion.TrackRemote) bool {
	return (track.Kind() == pion.RTPCodecTypeAudio && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeOpus)) ||
		(track.Kind() == pion.RTPCodecTypeVideo && strings.EqualFold(track.Codec().MimeType, pion.MimeTypeVP8))
}
