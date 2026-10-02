package sfu

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
	pion "github.com/pion/webrtc/v4"
)

// ownPublication подготавливает или проверяет часть тестового сценария «own Publication».
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p (*peer): байты, переданные по контракту io.Writer.
//   - source (media.Source): семантический источник медиа либо входной источник данных.
//
// @return:
//   - результат 1 (*publishedTrack): значение, подготовленное операцией для вызывающей стороны.
func ownPublication(p *peer, source media.Source) *publishedTrack {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, track := range p.publications {
		if track.metadata.Source == source {
			return track
		}
	}
	return nil
}

// TestRepublishSameReceiverAndDeviceReplacement проверяет сценарий «Republish Same Receiver и Device Replacement», фиксируя ошибки поведения как регрессию.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestRepublishSameReceiverAndDeviceReplacement(t *testing.T) {
	h := harness(t, 3)
	conf := uuid.NewString()
	a := h.join(t, conf, "")
	_ = h.join(t, conf, "")
	eventually(t, h, "initial sources", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 4 && h.manager.Snapshot().Subscriptions == 4 })
	server, _ := h.manager.get(a.id)
	a.neg.Lock()
	a.sourceDeclarations = map[string]media.Source{"microphone": media.SourceMicrophone, "camera": media.SourceCamera}
	a.neg.Unlock()
	if err := a.negotiate(); err != nil {
		t.Fatal(err)
	}
	old := ownPublication(server, media.SourceCamera)
	if err := h.manager.Unpublish(context.Background(), a.id, old.metadata.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "source retired", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 3 && h.manager.Snapshot().Subscriptions == 3 })
	time.Sleep(60 * time.Millisecond)
	if ownPublication(server, media.SourceCamera) != nil {
		t.Fatal("RTP alone bypassed stopped publication")
	}
	a.neg.Lock()
	a.declarationGeneration = "-reacquired"
	a.neg.Unlock()
	if err := a.negotiate(); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "same receiver republished", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 4 && h.manager.Snapshot().Subscriptions == 4 })
	republished := ownPublication(server, media.SourceCamera)
	if republished == nil || republished.metadata.ID == old.metadata.ID || republished.remote != old.remote {
		t.Fatal("republish did not replace publication while retaining receiver")
	}
	// Device replacement keeps the sender, source and receiver identity while
	// the transport continues; no new subscription is needed for the same SSRC.
	replacement, _ := pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, "replacement-camera", "publisher")
	if err := a.videoSender.ReplaceTrack(replacement); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_ = replacement.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: uint16(100 + i), Timestamp: uint32(100+i) * 1800, Marker: true}, Payload: []byte{0x10, 1, 2, 3}})
	}
	if got := h.manager.Snapshot(); got.Tracks != 4 || got.Subscriptions != 4 {
		t.Fatalf("replacement duplicated tracks: %+v", got)
	}
}

// TestScreenReservationAtomicAndPolicyCannotBeBypassed проверяет сценарий «экран Reservation Atomic и политика Cannot Be Bypassed», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestScreenReservationAtomicAndPolicyCannotBeBypassed(t *testing.T) {
	h := harness(t, 3)
	conf := uuid.NewString()
	a := h.join(t, conf, "")
	b := h.join(t, conf, "")
	eventually(t, h, "initial media", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 4 })
	pa, _ := h.manager.get(a.id)
	pb, _ := h.manager.get(b.id)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, p := range []*peer{pa, pb} {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - p (*peer): байты, переданные по контракту io.Writer.
		*/func(p *peer) {
			defer wg.Done()
			results <- p.reserveSources(map[string]media.Source{"screen": media.SourceVideoScreen})
		}(p)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, media.ErrScreenConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("screen claim: wins=%d conflicts=%d", wins, conflicts)
	}
	policy := media.ParticipantPolicy{Version: 2, MicrophoneBlocked: true, ScreenBlocked: true}
	if err := h.manager.SetPolicy(context.Background(), conf, a.binding.ParticipantID, policy); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "microphone moderation", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return ownPublication(pa, media.SourceMicrophone) == nil })
	if err := h.manager.SetPolicy(context.Background(), conf, a.binding.ParticipantID, media.ParticipantPolicy{Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := pa.reserveSources(map[string]media.Source{"audio": media.SourceMicrophone}); !errors.Is(err, media.ErrPolicy) {
		t.Fatalf("stale policy unmuted source: %v", err)
	}
	time.Sleep(60 * time.Millisecond)
	if ownPublication(pa, media.SourceMicrophone) != nil {
		t.Fatal("RTP bypassed forced mute")
	}
	if err := h.manager.SetPolicy(context.Background(), conf, a.binding.ParticipantID, media.ParticipantPolicy{Version: 3, Kicked: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.manager.PeerBinding(a.id); ok {
		t.Fatal("kicked media peer retained")
	}
	if _, err := h.manager.Join(context.Background(), a.binding); !errors.Is(err, media.ErrPolicy) {
		t.Fatalf("kicked participant rejoined: %v", err)
	}
}

// TestScreenTypedPublicationAlongsideCamera проверяет сценарий «экран Typed Publication Alongside Camera», фиксируя ошибки поведения как регрессию.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestScreenTypedPublicationAlongsideCamera(t *testing.T) {
	h := harness(t, 3)
	conf := uuid.NewString()
	a := h.join(t, conf, "")
	b := h.join(t, conf, "")
	a.neg.Lock()
	a.sourceDeclarations = map[string]media.Source{"microphone": media.SourceMicrophone, "camera": media.SourceCamera, "screen": media.SourceVideoScreen}
	screen, _ := pion.NewTrackLocalStaticRTP(pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, "screen", "display")
	sender, err := a.pc.AddTrack(screen)
	a.neg.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err = a.negotiate(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.wg.Add(2)
	a.mu.Unlock()
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer a.wg.Done()
		for {
			if _, _, err := sender.ReadRTCP(); err != nil {
				return
			}
		}
	}()
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer a.wg.Done()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		var n uint16
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				n++
				_ = screen.WriteRTP(&rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: n, Timestamp: uint32(n) * 1800, Marker: true}, Payload: []byte{0x10, 1, 2, 3}})
			}
		}
	}()
	eventually(t, h, "camera plus screen", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 5 && h.manager.Snapshot().Subscriptions == 5 })
	pa, _ := h.manager.get(a.id)
	track := ownPublication(pa, media.SourceVideoScreen)
	if track == nil || track.metadata.StreamID != a.id+"-screen" {
		t.Fatal("screen stream identity missing")
	}
	seen := false
	for _, tr := range h.manager.Tracks(b.id) {
		if tr.Source == media.SourceVideoScreen {
			seen = true
		}
	}
	if !seen {
		t.Fatal("subscriber lost source type")
	}
	if err = h.manager.Unpublish(context.Background(), a.id, track.metadata.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "screen stopped", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 4 && h.manager.Snapshot().Subscriptions == 4 })
	a.neg.Lock()
	a.declarationGeneration = "-new-display"
	a.neg.Unlock()
	if err = a.negotiate(); err != nil {
		t.Fatal(err)
	}
	eventually(t, h, "screen restarted", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 5 && h.manager.Snapshot().Subscriptions == 5 })
}

// TestEgressOrderingAndSlowRecorderIsolation проверяет сценарий «выход медиа порядок и Slow Recorder Isolation», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestEgressOrderingAndSlowRecorderIsolation(t *testing.T) {
	h := harness(t, 3, Options{EgressQueueSize: 32})
	conf := uuid.NewString()
	a := h.join(t, conf, "")
	b := h.join(t, conf, "")
	eventually(t, h, "media established", /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func() bool { return h.manager.Snapshot().Tracks == 4 && h.manager.Snapshot().Subscriptions == 4 })
	sub, err := h.manager.SubscribeRecording(context.Background(), conf, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()
	known := map[string]bool{}
	packets := 0
	var sequence uint64
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for packets < 12 {
		select {
		case frame := <-sub.Frames():
			if frame.Sequence <= sequence || frame.CapturedAt <= 0 {
				t.Fatal("invalid egress ordering/clock")
			}
			sequence = frame.Sequence
			switch frame.Type {
			case "track":
				if frame.Track == nil || frame.Track.ClockRate == 0 {
					t.Fatal("missing codec descriptor")
				}
				known[frame.TrackID] = true
			case "rtp":
				if !known[frame.TrackID] {
					t.Fatal("RTP before descriptor")
				}
				var packet rtp.Packet
				if packet.Unmarshal(frame.RTP) != nil {
					t.Fatal("invalid RTP")
				}
				packets++
			}
		case <-timeout.C:
			t.Fatal("recording received no encoded packets")
		}
	}
	// The consumer now stalls, filling only its bounded egress queue.
	select {
	case <-sub.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("slow recording did not fail")
	}
	if sub.Err() == nil {
		t.Fatal("overflow reported success")
	}
	if s := h.manager.Snapshot(); s.Peers != 2 || s.Tracks != 4 || s.RecordingDrops == 0 {
		t.Fatalf("recorder failure affected media: %+v", s)
	}
	sub.Close()
	active, err := h.manager.SubscribeRecording(context.Background(), conf, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	a.close()
	b.close()
	if !h.manager.HasConference(conf) {
		t.Fatal("empty recording room lost its lease")
	}
	active.Close()
	if s := h.manager.Snapshot(); s.Rooms != 0 || s.RecordingOutputs != 0 {
		t.Fatalf("egress cleanup: %+v", s)
	}
}
