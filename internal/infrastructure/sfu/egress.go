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

// egress пассивно наблюдает закодированные пакеты; переполнение очереди завершает подписку записи и не задерживает RTP SFU.
// @params
//   - manager: значение manager типа *Manager, используемое согласно назначению этой операции.
//   - room: значение room типа *room, используемое согласно назначению этой операции.
//   - id: идентификатор обрабатываемого ресурса.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - frames: канал «frames» для передачи данных или завершения ожидания.
//   - done: канал уведомления о завершении ресурса.
//   - err: сохранённая причина ошибочного завершения.
//   - closed: логический признак closed, управляющий соответствующей веткой обработки.
//   - sequence: серверный монотонный номер сообщения или команды.
//   - tracks: набор дорожек, входящих в операцию.
//   - closeOnce: значение closeOnce типа sync.Once, используемое согласно назначению этой операции.
type egress struct {
	audioOnly bool
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

// SubscribeRecording открывает пассивную ограниченную подписку записи на закодированные пакеты SFU.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - recordingID (string): идентификатор записи конференции.
//
// @return:
//   - результат 1 (media.EgressSubscription): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (m *Manager) SubscribeRecording(ctx context.Context, conferenceID, recordingID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, recordingID, false)
}

// SubscribeAudio создаёт отдельную ограниченную подписку только на аудио; запись не занимает её слот.
// @args ctx — контекст открытия; conferenceID — встреча; sessionID — UUID вспомогательного процесса.
// @return подписка, деградирующая независимо от RTP и записи, либо ошибка лимита/владения.
func (m *Manager) SubscribeAudio(ctx context.Context, conferenceID, sessionID string) (media.EgressSubscription, error) {
	return m.subscribeEgress(ctx, conferenceID, sessionID, true)
}

// subscribeEgress резервирует не более одной подписки каждого назначения в комнате.
// @args ctx — отмена; conferenceID/recordingID — UUID; audioOnly — вспомогательный audio tap.
// @return ограниченный поток или ошибка.
func (m *Manager) subscribeEgress(ctx context.Context, conferenceID, recordingID string, audioOnly bool) (media.EgressSubscription, error) {
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
	for id, existing := range r.egresses {
		if id == recordingID || existing.audioOnly == audioOnly {
			return nil, media.ErrNegotiation
		}
	}
	queueSize := m.opts.EgressQueueSize
	if audioOnly {
		queueSize = min(queueSize, 256)
	}
	e := &egress{audioOnly: audioOnly, manager: m, room: r, id: recordingID, frames: make(chan media.EgressFrame, queueSize), done: make(chan struct{}), tracks: map[string]bool{}}
	e.mu.Lock()
	e.enqueueLocked(media.EgressFrame{Type: "hello"})
	e.mu.Unlock()
	for _, t := range r.tracks {
		e.addTrack(t)
		if !audioOnly {
			t.requestPLI()
		}
	}
	r.egresses[recordingID] = e
	return e, nil
}

// Frames возвращает канал кадров подписки записи.
//
// @return:
//   - результат 1 (<-chan media.EgressFrame): канал данных или уведомления о завершении, принадлежащий жизненному циклу компонента.
func (e *egress) Frames() <-chan media.EgressFrame { return e.frames }

// Done возвращает канал, закрывающийся при завершении жизненного цикла ресурса.
//
// @return:
//   - результат 1 (<-chan struct{}): канал данных или уведомления о завершении, принадлежащий жизненному циклу компонента.
func (e *egress) Done() <-chan struct{} { return e.done }

// Err возвращает сохранённую причину завершения ресурса, если оно было ошибочным.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (e *egress) Err() error { e.mu.Lock(); defer e.mu.Unlock(); return e.err }

// enqueueLocked добавляет пакет в ограниченную очередь записи; переполнение завершает только подписку записи.
//
// @args
//   - frame (media.EgressFrame): значение frame типа media.EgressFrame, используемое согласно назначению этой операции.
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
		if e.audioOnly {
			e.manager.audioTapDropped.Add(1)
		} else {
			e.manager.egressDropped.Add(1)
		}
		e.closed = true
		e.err = errEgressOverflow
		close(e.done)
	}
}

// addTrack добавляет описание дорожки в подписку записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func (e *egress) addTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.tracks[t.metadata.ID] || !t.permitted.Load() || (e.audioOnly && t.metadata.Kind != media.KindAudio) {
		return
	}
	e.tracks[t.metadata.ID] = true
	codec := t.remote.Codec()
	descriptor := media.EgressTrack{Track: t.metadata, MimeType: codec.MimeType, ClockRate: codec.ClockRate, Channels: codec.Channels, PayloadType: uint8(t.remote.PayloadType()), SSRC: uint32(t.remote.SSRC())}
	e.enqueueLocked(media.EgressFrame{Type: "track", Track: &descriptor, TrackID: t.metadata.ID})
}

// endTrack обозначает завершение дорожки в подписке записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func (e *egress) endTrack(t *publishedTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] {
		return
	}
	delete(e.tracks, t.metadata.ID)
	e.enqueueLocked(media.EgressFrame{Type: "track.end", TrackID: t.metadata.ID})
}

// packet передаёт закодированный пакет в ограниченную очередь подписки записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - raw ([]byte): исходные байты JSON, пакета или сериализованного значения.
//   - at (int64): однозначное время планируемой операции; nil означает отсутствие значения, если это допускает тип.
func (e *egress) packet(t *publishedTrack, raw []byte, at int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.tracks[t.metadata.ID] || !t.permitted.Load() {
		return
	}
	e.enqueueLocked(media.EgressFrame{Type: "rtp", TrackID: t.metadata.ID, RTP: raw, CapturedAt: at})
}

// fail фиксирует ошибочное завершение и запускает предусмотренную очистку ресурса.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - err (error): ошибка, которую необходимо классифицировать, сохранить или вернуть клиенту.
func (e *egress) fail(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.err = err
		e.closed = true
		close(e.done)
	}
}

// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
func (e *egress) Close() {
	e.closeOnce.Do( /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() {
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

// Keyframes запрашивает ключевые кадры для источников текущей подписки записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
func (e *egress) Keyframes() {
	e.room.mu.Lock()
	defer e.room.mu.Unlock()
	for _, t := range e.room.tracks {
		t.requestPLI()
	}
}

// Ping проверяет активность ресурса и связь с его владельцем.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
func (e *egress) Ping() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.enqueueLocked(media.EgressFrame{Type: "ping"})
}

// recordPacket передаёт наблюдаемую копию закодированного пакета подписчикам записи.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*publishedTrack): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - packet (*rtp.Packet): закодированный RTP- или управляющий пакет.
func (m *Manager) recordPacket(t *publishedTrack, packet *rtp.Packet) {
	t.publisher.room.mu.Lock()
	egresses := make([]*egress, 0, len(t.publisher.room.egresses))
	for _, e := range t.publisher.room.egresses {
		if e.audioOnly && t.metadata.Kind != media.KindAudio {
			continue
		}
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
