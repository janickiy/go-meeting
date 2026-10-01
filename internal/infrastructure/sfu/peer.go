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

// outboundEvent передаёт событие пира в ограниченную очередь исходящей сигнализации.
//   - kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - data: полезная нагрузка события или байты обрабатываемого содержимого.
type outboundEvent struct {
	kind string
	data any
}

// peer хранит физическое WebRTC-соединение, сигнализацию, политику и подписки участника.
//   - id: идентификатор обрабатываемого ресурса.
//   - binding: проверенная идентичность медиа-подключения, назначенная сервером.
//   - manager: значение manager типа *Manager, используемое согласно назначению этой операции.
//   - room: значение room типа *room, используемое согласно назначению этой операции.
//   - pc: значение pc типа *pion.PeerConnection, используемое согласно назначению этой операции.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - negotiation: значение negotiation типа sync.Mutex, используемое согласно назначению этой операции.
//   - offers: значение offers типа sync.Mutex, используемое согласно назначению этой операции.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - closing: логический признак closing, управляющий соответствующей веткой обработки.
//   - publications: индекс значений publications для поиска и согласования состояния.
//   - subscriptions: индекс значений subscriptions для поиска и согласования состояния.
//   - events: получатель или издатель событий прикладного сценария.
//   - notify: канал «notify» для передачи данных или завершения ожидания.
//   - revision: значение revision типа atomic.Uint64, используемое согласно назначению этой операции.
//   - wg: счётчик принадлежащих компоненту фоновых горутин для ожидания завершения.
//   - closeOnce: значение closeOnce типа sync.Once, используемое согласно назначению этой операции.
//   - stopRequested: значение stopRequested типа atomic.Bool, используемое согласно назначению этой операции.
//   - allowedSources: индекс значений allowedSources для поиска и согласования состояния.
//   - receivers: индекс значений receivers для поиска и согласования состояния.
//   - suspendedSources: индекс значений suspendedSources для поиска и согласования состояния.
//   - offeredTrackIDs: идентификаторы связанных ресурсов для пакетной операции.
//   - pendingRemoteICE: набор значений pendingRemoteICE для последовательной или пакетной обработки.
//   - remoteICECount: значение remoteICECount типа int, используемое согласно назначению этой операции.
//   - ready: значение ready типа atomic.Bool, используемое согласно назначению этой операции.
//   - forwarding: значение forwarding типа sync.RWMutex, используемое согласно назначению этой операции.
//   - pendingSubscriptions: набор значений pendingSubscriptions для последовательной или пакетной обработки.
//   - negotiationID: идентификатор связанного ресурса, заданного параметром negotiationID.
//   - pendingReadyAt: временная отметка pendingReadyAt; указатель допускает отсутствие значения.
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
	failed               atomic.Bool // исключает двойной учёт ошибки callback-ом и watchdog-ом
	classified           atomic.Bool // маршрут ICE учитывается один раз для физического подключения
	allowedSources       map[string]media.Source
	receivers            map[string]*receiver
	suspendedSources     map[string]string
	offeredTrackIDs      map[string]string
	pendingRemoteICE     []pion.ICECandidateInit // guarded by negotiation
	remoteICECount       int                     // guarded by negotiation; bounded for this PeerConnection
	ready                atomic.Bool
	forwarding           sync.RWMutex    // guards readiness changes against in-flight RTP writes
	pendingSubscriptions []*subscription // exact answered set, guarded by negotiation
	negotiationID        string          // guarded by mu
	pendingReadyAt       time.Time       // guarded by mu
}

// view собирает внешний снимок текущего состояния без раскрытия внутренних ресурсов.
//
// @return:
//   - результат 1 (media.PeerView): значение, подготовленное операцией для вызывающей стороны.
func (p *peer) view() media.PeerView {
	return media.PeerView{MediaPeerID: p.id, ConferenceID: p.binding.ConferenceID, ParticipantID: p.binding.ParticipantID, SessionID: p.binding.SessionID, ConnectionID: p.binding.ConnectionID}
}

// start запускает обработку ресурсов компонента и подготавливает связанные ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - fn (func()): вызываемый обработчик «fn» с контрактом, указанным в типе.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p *peer) start(fn func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closing {
		return false
	}
	p.wg.Add(1)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

	 */func() { defer p.wg.Done(); fn() }()
	return true
}

// install устанавливает обработчики Pion для сигнализации и изменений дорожек.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
func (p *peer) install() {
	p.start(p.emitLoop)
	p.start(p.negotiationNotifications)
	p.pc.OnTrack( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		@parameters:
		  - t (*pion.TrackRemote): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
		  - r (*pion.RTPReceiver): запрос либо состояние ресурса согласно указанному типу.
		*/func(t *pion.TrackRemote, r *pion.RTPReceiver) {
			p.start( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

				 */func() { p.manager.receive(p, t, r) })
		})
	p.pc.OnICECandidate( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		@parameters:
		  - candidate (*pion.ICECandidate): проверенный кандидат ICE для WebRTC-соединения.
		*/func(candidate *pion.ICECandidate) {
			var value *pion.ICECandidateInit
			if candidate != nil {
				c := candidate.ToJSON()
				value = &c
			}
			p.emit("media.ice", map[string]any{"mediaPeerId": p.id, "candidate": value})
		})
	p.pc.OnConnectionStateChange( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		@parameters:
		  - state (pion.PeerConnectionState): значение state типа pion.PeerConnectionState, используемое согласно назначению этой операции.
		*/func(state pion.PeerConnectionState) {
			p.manager.log(p, "peer_connection_state", []any{"state", state.String()})
			p.emit("media.state", map[string]string{"mediaPeerId": p.id, "state": state.String()})
			if state == pion.PeerConnectionStateConnected {
				p.classifyTransport()
			}
			if state == pion.PeerConnectionStateFailed {
				p.markFailure()
				p.stopAsync()
			}
		})
	p.pc.OnICEConnectionStateChange( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		@parameters:
		  - state (pion.ICEConnectionState): значение state типа pion.ICEConnectionState, используемое согласно назначению этой операции.
		*/func(state pion.ICEConnectionState) {
			p.manager.log(p, "ice_state", []any{"state", state.String()})
		})
	p.start( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
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
						p.markFailure()
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
					if state != pion.PeerConnectionStateClosed {
						p.markFailure()
					}
					p.stopAsync()
					return
				}
			}
		})
}

// markFailure отмечает сбой физического подключения ровно один раз. Аргументов
// нет; нормальный выход после stopRequested не считается аварией. Метод нужен
// и callback-у Pion, и watchdog-у, который может закрыть disconnected раньше failed.
func (p *peer) markFailure() {
	if !p.stopRequested.Load() && p.failed.CompareAndSwap(false, true) {
		p.manager.failures.Add(1)
	}
}

// classifyTransport учитывает выбранную ICE-пару без адресов и идентификаторов
// в метриках. Аргументов нет; relay означает TURN на любой стороне пары, direct
// объединяет host/srflx/prflx. Повторный connected не дублирует физическое соединение.
func (p *peer) classifyTransport() {
	for _, receiver := range p.pc.GetReceivers() {
		transport := receiver.Transport()
		if transport == nil || transport.ICETransport() == nil {
			continue
		}
		pair, err := transport.ICETransport().GetSelectedCandidatePair()
		if err != nil || pair == nil || pair.Local == nil || pair.Remote == nil {
			continue
		}
		if p.classified.CompareAndSwap(false, true) {
			if pair.Local.Typ == pion.ICECandidateTypeRelay || pair.Remote.Typ == pion.ICECandidateTypeRelay {
				p.manager.relay.Add(1)
			} else {
				p.manager.direct.Add(1)
			}
		}
		return
	}
}

// emit формирует и передаёт исходящее событие через принадлежащий компоненту канал доставки.
//
// @parameters:
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
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

// stopAsync запускает остановку пира без блокировки вызывающего обработчика.
func (p *peer) stopAsync() {
	if p.stopRequested.CompareAndSwap(false, true) {
		go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		 */func() { _ = p.manager.Leave(context.Background(), p.id) }()
	}
}

// emitLoop последовательно доставляет события пира из ограниченной очереди.
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

// changed сообщает зависимым обработчикам об изменении локального или сохранённого состояния.
func (p *peer) changed() {
	p.revision.Add(1)
	select {
	case p.notify <- struct{}{}:
	default:
	}
}

// tracks собирает снимок дорожек, доступных текущему пиру.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 ([]media.Track): собранные элементы результата; состав ограничивается параметрами операции.
func (p *peer) tracks() []media.Track {
	p.mu.Lock()
	defer p.mu.Unlock()
	tracks := make([]media.Track, 0, len(p.subscriptions))
	for _, sub := range p.subscriptions {
		tracks = append(tracks, sub.source.metadata)
	}
	return tracks
}

// negotiationNotifications уведомляет о необходимости повторного согласования WebRTC при изменении подписок.
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

// close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
func (p *peer) close() {
	p.closeOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
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

// Offer обрабатывает или передаёт SDP-предложение действующего WebRTC-подключения.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - negotiationID (string): идентификатор связанного ресурса, заданного параметром negotiationID.
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (pion.SessionDescription): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) Offer(ctx context.Context, id, negotiationID, raw string) (pion.SessionDescription, error) {
	return m.OfferSources(ctx, id, negotiationID, raw, nil)
}

// OfferSources обрабатывает SDP-предложение с привязкой секций к заявленным источникам медиа.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - negotiationID (string): идентификатор связанного ресурса, заданного параметром negotiationID.
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//   - publications ([]media.Publication): набор значений publications для последовательной или пакетной обработки.
//
// @return:
//   - результат 1 (pion.SessionDescription): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) OfferSources(ctx context.Context, id, negotiationID, raw string, publications []media.Publication) (pion.SessionDescription, error) {
	if parsed, err := uuid.Parse(negotiationID); err != nil || parsed == uuid.Nil {
		return pion.SessionDescription{}, media.ErrInvalid
	}
	p, err := m.get(id)
	if err != nil {
		return pion.SessionDescription{}, err
	}
	sources, err := validateSourceOffer(raw, m.opts.MaxPeers, publications, m.opts.MaxPublishedTracks, m.opts.MaxAudioTracks, m.opts.MaxVideoTracks)
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
	trackIDs := map[string]string{}
	for _, publication := range publications {
		trackIDs[publication.MID] = publication.TrackID
	}
	for mid, previousID := range p.suspendedSources {
		if sources[mid] == "" || (trackIDs[mid] != "" && trackIDs[mid] != previousID) {
			delete(p.suspendedSources, mid)
		} else {
			delete(sources, mid)
		}
	}
	p.mu.Unlock()
	if closed {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrPeerNotFound
	}
	if p.pc.SignalingState() != pion.SignalingStateStable {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, media.ErrNegotiation
	}
	if err = p.reserveSources(sources); err != nil {
		p.negotiation.Unlock()
		return pion.SessionDescription{}, err
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
	p.allowedSources = sources
	p.offeredTrackIDs = trackIDs
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
		if p.allowedSources[t.receiver.mid] != t.metadata.Source {
			retire = append(retire, t)
		}
	}
	p.mu.Unlock()
	for _, t := range retire {
		m.unpublish(t)
	}
	return result, nil
}

// Ready принимает подтверждение установки SDP-ответа клиентом и разрешает дорожки только для текущего согласования.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - negotiationID (string): идентификатор связанного ресурса, заданного параметром negotiationID.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// ICE передаёт проверенного кандидата ICE действующему медиа-соединению.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - candidate (*pion.ICECandidateInit): проверенный кандидат ICE для WebRTC-соединения.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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

// Unpublish останавливает публикацию указанного медиа-источника.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - trackID (string): идентификатор связанного ресурса, заданного параметром trackID.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	if track != nil {
		delete(p.allowedSources, track.receiver.mid)
		if p.suspendedSources == nil {
			p.suspendedSources = map[string]string{}
		}
		p.suspendedSources[track.receiver.mid] = p.offeredTrackIDs[track.receiver.mid]
	}
	p.mu.Unlock()
	if track != nil {
		m.unpublish(track)
	}
	return nil
}

// validateOffer проверяет SDP-предложение и допустимость его дорожек.
//
// @parameters:
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//   - maxPeers (int): значение maxPeers типа int, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (map[string]bool): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func validateOffer(raw string, maxPeers int) (map[string]bool, error) {
	sources, err := validateSourceOffer(raw, maxPeers, nil, 2, 1, 1)
	if err != nil {
		return nil, err
	}
	active := map[string]bool{}
	for _, source := range sources {
		active[string(media.SourceKind(source))] = true
	}
	return active, nil
}

// validateSourceOffer проверяет соответствие SDP-секций заявленным источникам и действующей политике.
//
// @parameters:
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//   - maxPeers (int): значение maxPeers типа int, используемое согласно назначению этой операции.
//   - publications ([]media.Publication): набор значений publications для последовательной или пакетной обработки.
//   - maxTracks (int): значение maxTracks типа int, используемое согласно назначению этой операции.
//   - maxAudio (int): значение maxAudio типа int, используемое согласно назначению этой операции.
//   - maxVideo (int): значение maxVideo типа int, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (map[string]media.Source): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func validateSourceOffer(raw string, maxPeers int, publications []media.Publication, maxTracks, maxAudio, maxVideo int) (map[string]media.Source, error) {
	if len(raw) == 0 || len(raw) > 49152 {
		return nil, media.ErrInvalid
	}
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return nil, media.ErrInvalid
	}
	slotLimit := 4 * maxPeers
	if publications == nil {
		slotLimit = 2 * maxPeers
	}
	if len(description.MediaDescriptions) == 0 || len(description.MediaDescriptions) > slotLimit {
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
	declared := map[string]media.Source{}
	uniqueSources := map[media.Source]bool{}
	for _, publication := range publications {
		if publication.MID == "" || len(publication.MID) > 64 || strings.ContainsAny(publication.MID, "\r\n\x00 ") || len(publication.TrackID) > 128 || strings.ContainsAny(publication.TrackID, "\r\n\x00") || media.SourceKind(publication.Source) == "" || declared[publication.MID] != "" || uniqueSources[publication.Source] {
			return nil, media.ErrInvalid
		}
		declared[publication.MID] = publication.Source
		uniqueSources[publication.Source] = true
	}
	if len(publications) > maxTracks || (uniqueSources[media.SourceAudioScreen] && !uniqueSources[media.SourceVideoScreen]) {
		return nil, media.ErrLimit
	}
	active := map[string]media.Source{}
	kinds := map[string]int{}
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
		mid, _ := section.Attribute("mid")
		if mid == "" || len(mid) > 64 || active[mid] != "" {
			return nil, media.ErrInvalid
		}
		source := declared[mid]
		if publications == nil {
			if kinds[kind] > 0 {
				return nil, media.ErrLimit
			}
			source = media.SourceMicrophone
			if kind == "video" {
				source = media.SourceCamera
			}
		} else if source == "" || string(media.SourceKind(source)) != kind {
			return nil, media.ErrInvalid
		}
		kinds[kind]++
		if (kind == "audio" && kinds[kind] > maxAudio) || (kind == "video" && kinds[kind] > maxVideo) || len(active) >= maxTracks {
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
		active[mid] = source
	}
	if publications != nil && len(active) != len(publications) {
		return nil, media.ErrInvalid
	}
	return active, nil
}

// answeredSSRCs извлекает согласованные идентификаторы RTP-источников из SDP-ответа.
//
// @parameters:
//   - raw (string): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (map[uint32]bool): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
