package webrtc

import (
	"context"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"sync"
	"time"
)

// Shutdown прекращает приём записей и закрывает WebRTC, FFmpeg и ICE ресурсы.
// ctx ограничивает суммарное ожидание; завершённые фрагменты сохраняются.
// Возвращает ошибку дедлайна, если процессы не успели завершиться.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	sessions := make([]*session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = map[string]*session{}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range sessions {
		s.mu.Lock()
		s.failed = true
		s.stopping = true
		pc, process := s.pc, s.process
		startDone := s.startDone
		if s.waitTimer != nil {
			s.waitTimer.Stop()
			s.waitTimer = nil
		}
		s.mu.Unlock()
		if pc != nil {
			_ = pc.Close()
		}
		wg.Add(1)
		go func(s *session, p *ffmpeg.SegmentProcess) {
			defer wg.Done()
			if p != nil {
				_ = p.Stop(2 * time.Second)
			}
			s.cancel()
			if startDone != nil {
				<-startDone
			}
		}(s, process)
	}
	if m.iceConn != nil {
		_ = m.iceConn.Close()
	}
	if m.iceTCP != nil {
		_ = m.iceTCP.Close()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
