package sfu

import (
	"context"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"sync"
)

// SetPolicy применяет версионную политику медиа; устаревшее изменение не должно отменять более новую модерацию.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//   - policy (media.ParticipantPolicy): актуальные ограничения медиа и версия модерации участника.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
		// Отменяем все вкладки до ожидания обработчика событий при очистке транспорта,
		// сохраняя такое же немедленное прекращение доступа, как при завершении конференции.
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
		go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

		 */func() {
			var workers sync.WaitGroup
			for _, p := range detached {
				workers.Add(1)
				go /* Вложенный обработчик выполняет выделенный шаг обработки в пересылке WebRTC-медиа через SFU, используя состояние окружающей функции.

				@args
				  - p (*peer): байты, переданные по контракту io.Writer.
				*/func(p *peer) { defer workers.Done(); m.finishDetached(p) }(p)
			}
			workers.Wait()
			close(done)
		}()
		return waitClosed(ctx, done)
	}
	return nil
}

// ParticipantPolicy возвращает актуальную политику конкретного участника для проверки медиа.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - participantID (string): идентификатор членства участника внутри конференции.
//
// @return:
//   - результат 1 (media.ParticipantPolicy): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
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

// reserveSources резервирует источники участника с учётом политики и ограничений комнаты.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - sources (map[string]media.Source): набор источников медиа для публикации или композиции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
