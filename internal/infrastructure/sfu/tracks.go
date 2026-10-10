package sfu

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

// publishedTrack хранит опубликованную дорожку, её источник и подписчиков.
// @params
//   - metadata: значение metadata типа media.Track, используемое согласно назначению этой операции.
//   - publisher: транспорт публикации событий после сохранения состояния.
//   - remote: значение remote типа *pion.TrackRemote, используемое согласно назначению этой операции.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - active: логический признак active, управляющий соответствующей веткой обработки.
//   - subscribers: индекс значений subscribers для поиска и согласования состояния.
//   - lastPLI: временная отметка lastPLI; указатель допускает отсутствие значения.
//   - pli: канал «pli» для передачи данных или завершения ожидания.
//   - stopOnce: значение stopOnce типа sync.Once, используемое согласно назначению этой операции.
//   - receiver: значение receiver типа *receiver, используемое согласно назначению этой операции.
//   - permitted: значение permitted типа atomic.Bool, используемое согласно назначению этой операции.
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
	receiver    *receiver
	permitted   atomic.Bool
}

// receiver владеет транспортным приёмником, который переживает паузу публикации и может повторно использовать согласованный SSRC.
// @params
//   - mid: идентификатор SDP-секции, связывающий дорожку с источником.
//   - remote: значение remote типа *pion.TrackRemote, используемое согласно назначению этой операции.
//   - publication: значение publication типа *publishedTrack, используемое согласно назначению этой операции.
type receiver struct {
	mid         string
	remote      *pion.TrackRemote
	publication *publishedTrack // защищено publisher.mu
}

// subscription связывает опубликованную дорожку с исходящим потоком конкретного получателя.
// @params
//   - source: семантический источник медиа либо входной источник данных.
//   - peer: значение peer типа *peer, используемое согласно назначению этой операции.
//   - local: значение local типа *pion.TrackLocalStaticRTP, используемое согласно назначению этой операции.
//   - sender: значение sender типа *pion.RTPSender, используемое согласно назначению этой операции.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - cancel: отмена контекста, завершающая принадлежащие ресурсу операции.
//   - queue: канал «очередь» для передачи данных или завершения ожидания.
//   - stopOnce: значение stopOnce типа sync.Once, используемое согласно назначению этой операции.
//   - ready: значение ready типа atomic.Bool, используемое согласно назначению этой операции.
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

// receive читает входящие события или пакеты и передаёт их соответствующим обработчикам.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
//   - remote (*pion.TrackRemote): значение remote типа *pion.TrackRemote, используемое согласно назначению этой операции.
//   - transport (*pion.RTPReceiver): клиент защищённого внутреннего медиа-транспорта.
func (m *Manager) receive(p *peer, remote *pion.TrackRemote, transport *pion.RTPReceiver) {
	if !supported(remote) || remote.RID() != "" {
		p.emit("media.error", map[string]string{"mediaPeerId": p.id, "code": "unsupported_codec_or_simulcast"})
		p.stopAsync()
		return
	}
	mid := ""
	for _, transceiver := range p.pc.GetTransceivers() {
		if transceiver.Receiver() == transport {
			mid = transceiver.Mid()
			break
		}
	}
	if mid == "" {
		p.stopAsync()
		return
	}
	r := &receiver{mid: mid, remote: remote}
	p.mu.Lock()
	if p.closing {
		p.mu.Unlock()
		return
	}
	previous := p.receivers[mid]
	p.receivers[mid] = r
	var old *publishedTrack
	if previous != nil {
		old = previous.publication
	}
	p.mu.Unlock()
	if old != nil {
		m.unpublish(old)
	}
	if previous != nil && previous.remote != remote {
		_ = previous.remote.SetReadDeadline(time.Now())
	}
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
	Синхронизирует доступ к разделяемому состоянию блокировкой.

	*/func() {
		p.mu.Lock()
		t := r.publication
		if p.receivers[mid] == r {
			delete(p.receivers, mid)
		}
		p.mu.Unlock()
		if t != nil {
			m.unpublish(t)
		}
	}()
	for {
		packet, _, err := remote.ReadRTP()
		if err != nil || p.ctx.Err() != nil {
			return
		}
		t := m.activate(p, r)
		if t == nil || !t.permitted.Load() {
			continue
		}
		// SFU не копирует специфичные для издателя ID расширений MID/RID/TWCC в транспорт
		// подписчика с другим согласованием. Pion переписывает SSRC/PT для каждого локального
		// отправителя; закодированная нагрузка и временные отметки сохраняются.
		packet.Header.Extension = false
		packet.Header.ExtensionProfile = 0
		packet.Header.Extensions = nil
		if len(packet.Payload) > 4096 {
			m.dropped.Add(1)
			continue
		}
		m.recordPacket(t, packet)
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

// activate включает публикацию дорожки после проверки политики и согласования.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
//   - r (*receiver): запрос либо состояние ресурса согласно указанному типу.
//
// @return:
//   - результат 1 (*publishedTrack): значение, подготовленное операцией для вызывающей стороны.
func (m *Manager) activate(p *peer, r *receiver) *publishedTrack {
	p.mu.Lock()
	if p.closing || p.receivers[r.mid] != r {
		p.mu.Unlock()
		return nil
	}
	if t := r.publication; t != nil {
		p.mu.Unlock()
		return t
	}
	source := p.allowedSources[r.mid]
	kind := media.SourceKind(source)
	if kind == "" || string(kind) != r.remote.Kind().String() {
		p.mu.Unlock()
		return nil
	}
	p.room.mu.Lock()
	if !p.room.policies[p.binding.ParticipantID].Allows(source) ||
		((source == media.SourceVideoScreen || source == media.SourceAudioScreen) && !p.room.screens[p.id]) {
		p.room.mu.Unlock()
		p.mu.Unlock()
		return nil
	}
	count := 0
	for _, old := range p.publications {
		if old.metadata.Source == source {
			p.room.mu.Unlock()
			p.mu.Unlock()
			return nil
		}
		if old.metadata.Kind == kind {
			count++
		}
	}
	limit := m.opts.MaxAudioTracks
	if kind == media.KindVideo {
		limit = m.opts.MaxVideoTracks
	}
	if len(p.publications) >= m.opts.MaxPublishedTracks || count >= limit {
		p.room.mu.Unlock()
		p.mu.Unlock()
		return nil
	}
	streamID := p.id
	if source == media.SourceVideoScreen || source == media.SourceAudioScreen {
		streamID += "-screen"
	}
	ctx, cancel := context.WithCancel(p.ctx)
	t := &publishedTrack{metadata: media.Track{ID: uuid.NewString(), StreamID: streamID, MediaPeerID: p.id, ParticipantID: p.binding.ParticipantID, Kind: kind, Source: source}, publisher: p, remote: r.remote, receiver: r, ctx: ctx, cancel: cancel, active: true, subscribers: map[string]*subscription{}, pli: make(chan struct{}, 1)}
	t.permitted.Store(true)
	r.publication = t
	p.publications[t.metadata.ID] = t
	p.room.tracks[t.metadata.ID] = t
	peers := make([]*peer, 0, len(p.room.peers))
	for _, target := range p.room.peers {
		peers = append(peers, target)
	}
	for _, e := range p.room.egresses {
		e.addTrack(t)
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
	return t
}

// syncPeer синхронизирует подписки пира с доступными публикациями комнаты.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
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

// subscribe создаёт подписку на опубликованную медиа-дорожку.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - p (*peer): байты, переданные по контракту io.Writer.
func (m *Manager) subscribe(t *publishedTrack, p *peer) {
	// Вкладки имеют разные конечные соединения, но собственные микрофон и камера пользователя
	// никогда не возвращаются в его остальные соединения.
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

// forward передаёт RTP-пакеты подписчикам дорожки.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
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
			if s.ctx.Err() != nil || !s.source.permitted.Load() || !s.peer.ready.Load() || !s.ready.Load() {
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

// readRTCP читает управляющие RTCP-пакеты от подписчика и обрабатывает запросы ключевых кадров.
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

// requestPLI запрашивает ключевой кадр у источника видео через RTCP PLI.
func (t *publishedTrack) requestPLI() {
	if t.metadata.Kind != media.KindVideo {
		return
	}
	select {
	case t.pli <- struct{}{}:
	default:
	}
}

// keyframes периодически запрашивает ключевые кадры, пока дорожка активна.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
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

// unsubscribe освобождает подписку на дорожку и связанные сетевые ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - s (*subscription): значение s типа *subscription, используемое согласно назначению этой операции.
func (m *Manager) unsubscribe(s *subscription) {
	s.stopOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
			s.cancel()
			s.peer.negotiation.Lock()
			s.peer.mu.Lock()
			delete(s.peer.subscriptions, s.source.metadata.ID)
			s.peer.mu.Unlock()
			_ = s.peer.pc.RemoveTrack(s.sender)
			// Останавливаем отдельно: RemoveTrack на закрытом PeerConnection возвращается до вызова Stop.
			_ = s.sender.Stop()
			s.peer.negotiation.Unlock()
			s.source.mu.Lock()
			delete(s.source.subscribers, s.peer.id)
			s.source.mu.Unlock()
			s.peer.changed()
		})
}

// unpublish удаляет публикацию источника из комнаты и обновляет подписчиков.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func (m *Manager) unpublish(t *publishedTrack) {
	t.stopOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
			t.permitted.Store(false)
			t.mu.Lock()
			t.active = false
			subs := make([]*subscription, 0, len(t.subscribers))
			for _, sub := range t.subscribers {
				subs = append(subs, sub)
			}
			t.mu.Unlock()
			t.cancel()
			t.publisher.mu.Lock()
			delete(t.publisher.publications, t.metadata.ID)
			if t.receiver.publication == t {
				t.receiver.publication = nil
			}
			t.publisher.room.mu.Lock()
			delete(t.publisher.room.tracks, t.metadata.ID)
			for _, e := range t.publisher.room.egresses {
				e.endTrack(t)
			}
			otherScreens := []*publishedTrack{}
			replacementScreen := false
			for mid, source := range t.publisher.allowedSources {
				if source == media.SourceVideoScreen && mid != t.receiver.mid {
					replacementScreen = true
				}
			}
			if t.metadata.Source == media.SourceVideoScreen && !replacementScreen && (t.publisher.receivers[t.receiver.mid] == nil || t.publisher.receivers[t.receiver.mid] == t.receiver) {
				delete(t.publisher.room.screens, t.publisher.id)
				for mid, source := range t.publisher.allowedSources {
					if source == media.SourceVideoScreen || source == media.SourceAudioScreen {
						if t.publisher.suspendedSources == nil {
							t.publisher.suspendedSources = map[string]string{}
						}
						t.publisher.suspendedSources[mid] = t.publisher.offeredTrackIDs[mid]
						delete(t.publisher.allowedSources, mid)
					}
				}
				for _, sibling := range t.publisher.publications {
					if sibling.metadata.Source == media.SourceAudioScreen {
						otherScreens = append(otherScreens, sibling)
					}
				}
			}
			t.publisher.room.mu.Unlock()
			t.publisher.mu.Unlock()
			for _, sibling := range otherScreens {
				m.unpublish(sibling)
			}
			for _, sub := range subs {
				m.unsubscribe(sub)
			}
			m.log(t.publisher, "track_unpublished", []any{"track_id", t.metadata.ID, "kind", t.metadata.Kind, "source", t.metadata.Source})
			t.publisher.emit("media.unpublished", map[string]string{"mediaPeerId": t.publisher.id, "trackId": t.metadata.ID})
		})
}

// Tracks возвращает снимок доступных опубликованных дорожек комнаты.
//
// @args
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 ([]media.Track): собранные элементы результата; состав ограничивается параметрами операции.
func (m *Manager) Tracks(id string) []media.Track {
	p, err := m.get(id)
	if err != nil {
		return []media.Track{}
	}
	tracks := p.tracks()
	sort.Slice(tracks, /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		@args
		  - i (int): значение i типа int, используемое согласно назначению этой операции.
		  - j (int): значение j типа int, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(i, j int) bool { return tracks[i].ID < tracks[j].ID })
	return tracks
}
