package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/janickiy/go-recorder/internal/config"
	captions "github.com/janickiy/go-recorder/internal/domain/captions"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/janickiy/go-recorder/internal/infrastructure/liveproviders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	live "github.com/janickiy/go-recorder/internal/usecase/captions"
	"sync"
	"testing"
	"time"
)

// monitoredSTT инъецирует один отказ на дорожку и выявляет одновременные повторные provider sessions.
type monitoredSTT struct {
	mu               sync.Mutex
	active, attempts map[string]int
	duplicate        bool
}

// monitoredSession оборачивает тестовый gateway, сохраняя независимый счётчик закрытия.
type monitoredSession struct {
	captions.Session
	owner *monitoredSTT
	track string
	fail  bool
	bytes int
	once  sync.Once
}

// StartSession отмечает активную сессию и создаёт явно тестовый поставщик текста.
// @args ctx — срок жизни; cfg — одна incarnation аудиодорожки.
// @return сессия с одноразовым сбоем первой попытки.
func (p *monitoredSTT) StartSession(ctx context.Context, cfg captions.SessionConfig) (captions.Session, error) {
	p.mu.Lock()
	p.active[cfg.TrackInstanceID]++
	p.attempts[cfg.TrackInstanceID]++
	if p.active[cfg.TrackInstanceID] > 1 {
		p.duplicate = true
	}
	fail := p.attempts[cfg.TrackInstanceID] == 1
	p.mu.Unlock()
	session, err := (liveproviders.Provider{Mode: "mock", Timeout: time.Second}).StartSession(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &monitoredSession{Session: session, owner: p, track: cfg.TrackInstanceID, fail: fail}, nil
}

// WriteAudio создаёт проверяемый отказ после получения 600 ms, не касаясь SFU.
// @args ctx — deadline; pcm — синтетическое аудио.
// @return ошибка первой попытки либо результат fake STT.
func (s *monitoredSession) WriteAudio(ctx context.Context, pcm []byte) error {
	s.bytes += len(pcm)
	if s.fail && s.bytes > 19200 {
		return errors.New("test provider outage")
	}
	return s.Session.WriteAudio(ctx, pcm)
}

// Close закрывает сессию ровно один раз до возможности reconnect.
// @return безопасный результат закрытия.
func (s *monitoredSession) Close() error {
	err := s.Session.Close()
	s.once.Do(func() { s.owner.mu.Lock(); s.owner.active[s.track]--; s.owner.mu.Unlock() })
	return err
}

// TestStageEightLiveEndToEnd соединяет реальный SFU, HTTP tap, FFmpeg, fake STT, SQL и WebSocket другого участника.
// @args t — исполнитель на изолированных PostgreSQL/Redis; никаких платных вызовов.
func TestStageEightLiveEndToEnd(t *testing.T) {
	f := stageTwo(t)
	binary := stageEightFFmpeg(t)
	fixture := encodedFixture(t, binary)
	engine, mc := startMediaWithLimits(t, f, 4, 2, 2)
	a := newMediaTestPeerWithPublisher(t, f, 0, f.ownerToken, fixture.publish)
	b := newMediaTestPeerWithPublisher(t, f, 1, f.memberToken, fixture.publish)
	a.assertReceived(t, b.id())
	b.assertReceived(t, a.id())
	observer := f.connect(t, 1, f.memberToken, f.conference.ID)
	repo := pg.NewCaptionsRepository(f.db)
	cfg, err := config.LoadStageEight(true)
	if err != nil {
		t.Fatal(err)
	}
	cfg.LiveEnabled = true
	cfg.AnalyticsEnabled = true
	cfg.LiveConferences = 1
	cfg.LiveSessions = 2
	if _, err = repo.Set(context.Background(), f.owner.ID, f.conference.ID, true, "en"); err != nil {
		t.Fatal(err)
	}
	provider := &monitoredSTT{active: map[string]int{}, attempts: map[string]int{}}
	worker := &live.Worker{Repo: repo, Config: cfg, Provider: provider, Decoder: ffmpeg.LiveAudio{Binary: binary}, Tap: liveproviders.NewTap(redisinfra.NewMediaRegistry(f.redis, mc.Namespace), mc.InternalSecret), Events: f.store}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); worker.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Error("live worker cleanup timed out")
		}
	}()
	partial, final, degraded := false, false, false
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for !(partial && final && degraded) {
		select {
		case event := <-observer.events:
			switch event.Type {
			case "caption.partial", "caption.final":
				var c captions.Caption
				if json.Unmarshal(event.Data, &c) != nil || c.ParticipantID == "" || c.Speaker == "" || c.Language != "en" {
					t.Fatalf("caption attribution missing: type=%s participant=%q speaker=%q language=%q decode=%v", event.Type, c.ParticipantID, c.Speaker, c.Language, json.Unmarshal(event.Data, &c))
				}
				if event.Type == "caption.partial" {
					partial = true
				} else {
					final = true
				}
			case "caption.status":
				var state struct{ Status string }
				_ = json.Unmarshal(event.Data, &state)
				degraded = degraded || state.Status == "degraded"
			}
		case <-deadline.C:
			t.Fatalf("live acceptance timed out partial=%v final=%v degraded=%v", partial, final, degraded)
		}
	}
	// Подтверждаем фактическое восстановление провайдера, а не только финал закрываемой первой попытки.
	for until := time.Now().Add(8 * time.Second); ; {
		provider.mu.Lock()
		reconnected := false
		for _, count := range provider.attempts {
			reconnected = reconnected || count > 1
		}
		duplicate := provider.duplicate
		provider.mu.Unlock()
		if duplicate {
			t.Fatal("duplicate live STT session")
		}
		if reconnected {
			break
		}
		if time.Now().After(until) {
			t.Fatal("provider never reconnected")
		}
		time.Sleep(50 * time.Millisecond)
	}
	before := engine.Snapshot().Packets
	time.Sleep(200 * time.Millisecond)
	after := engine.Snapshot()
	if after.Packets <= before || after.Dropped != 0 {
		t.Fatal("STT outage affected SFU", after)
	}
	finals, err := repo.Finals(context.Background(), f.member.ID, f.conference.ID, 0, 100)
	if err != nil || len(finals) == 0 {
		t.Fatal("durable finals lost", err)
	}
	if _, err = repo.Set(context.Background(), f.owner.ID, f.conference.ID, false, "en"); err != nil {
		t.Fatal(err)
	}
	t.Logf("live e2e: partial/final/status delivered, finals=%d RTP packets=%d drops=%d", len(finals), after.Packets, after.Dropped)
}
