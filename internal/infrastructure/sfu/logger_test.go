package sfu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	pion "github.com/pion/webrtc/v4"
)

// safeLogBuffer хранит изолированное состояние тестового компонента «безопасный Log буфер».
// @params:
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - b: контекст измерения производительности теста.
type safeLogBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

// Write принимает байты вывода в ограниченный буфер и соблюдает контракт io.Writer.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - raw ([]byte): исходные байты JSON, пакета или сериализованного значения.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (b *safeLogBuffer) Write(raw []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(raw)
}

// String возвращает строковое представление накопленного значения или ограниченного диагностического вывода.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// unsafeLogArgument хранит изолированное состояние тестового компонента «unsafe Log Argument».
type unsafeLogArgument struct{}

// String возвращает строковое представление накопленного значения или ограниченного диагностического вывода.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (unsafeLogArgument) String() string { panic("upstream String invoked") }

// Error обрабатывает сообщение уровня ошибки через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (unsafeLogArgument) Error() string { panic("upstream Error invoked") }

// Format обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (fmt.State): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (rune): значение для проверки, нормализации или преобразования.
func (unsafeLogArgument) Format(fmt.State, rune) { panic("upstream Format invoked") }

// TestPionLoggerNeverFormatsOrRetainsUpstreamData проверяет сценарий «Pion Logger Never Formats Or Retains Upstream Data», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPionLoggerNeverFormatsOrRetainsUpstreamData(t *testing.T) {
	const sensitive = "synthetic-sensitive-logger-marker"
	output := &safeLogBuffer{}
	factory := safePionLoggerFactory{logger: slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	for _, scope := range []string{"pc", strings.Repeat(sensitive, 256)} {
		logger := factory.NewLogger(scope)
		for _, method := range []func(string){logger.Trace, logger.Debug, logger.Info, logger.Warn, logger.Error} {
			method(sensitive)
		}
		for _, method := range []func(string, ...any){logger.Tracef, logger.Debugf, logger.Infof, logger.Warnf, logger.Errorf} {
			method(sensitive+" %v %s", unsafeLogArgument{}, sensitive)
		}
	}
	raw := output.String()
	if strings.Contains(raw, sensitive) || strings.Contains(raw, "%v") {
		t.Fatal("upstream data leaked through safe logger")
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 12 { // Trace/Debug are intentionally disabled.
		t.Fatalf("unexpected bounded event count: %d", len(lines))
	}
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal("safe logger did not produce structured records")
		}
		if event["component"] != "pc" && event["component"] != "other" {
			t.Fatal("unbounded component logged")
		}
		switch event["level"] {
		case "INFO":
			if event["event_type"] != "pion_info" {
				t.Fatal("info marker missing")
			}
		case "WARN":
			if event["event_type"] != "pion_warning" {
				t.Fatal("warning marker missing")
			}
		case "ERROR":
			if event["event_type"] != "pion_error" {
				t.Fatal("error marker missing")
			}
		default:
			t.Fatal("unexpected safe event severity")
		}
	}
}

// TestMalformedSDPCandidateCannotLeakIntoPionLogs проверяет сценарий «Malformed SDP Candidate Cannot раскрытие Into Pion Logs», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestMalformedSDPCandidateCannotLeakIntoPionLogs(t *testing.T) {
	const sensitive = "synthetic-sensitive-candidate-marker"
	output := &safeLogBuffer{}
	m, err := NewManager(Options{Logger: slog.New(slog.NewJSONHandler(output, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := m.Shutdown(ctx); err != nil {
				t.Fatal("test manager cleanup failed")
			}
		})
	// Build a real supported offer with the same API used by the worker. Do
	// not expose its ICE credentials or whole SDP in assertions/failure output.
	client, err := m.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.AddTransceiverFromKind(pion.RTPCodecTypeAudio); err != nil {
		t.Fatal(err)
	}
	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	malformed := offer.SDP + "a=candidate:" + sensitive + "\r\n"
	view, err := m.Join(context.Background(), media.Binding{
		ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(),
		SessionID: uuid.NewString(), ConnectionID: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Offer(context.Background(), view.MediaPeerID, uuid.NewString(), malformed); !errors.Is(err, media.ErrInvalid) {
		t.Fatal("embedded malformed candidate was not rejected safely")
	}
	p, _ := m.get(view.MediaPeerID)
	if p.pc.RemoteDescription() != nil {
		t.Fatal("invalid candidate reached remote description")
	}
	componentTwo := offer.SDP + "a=candidate:1 2 udp 2122260223 127.0.0.1 50000 typ host\r\n"
	if _, err := validateOffer(componentTwo, m.opts.MaxPeers); err != nil {
		t.Fatal("Pion's redundant full-SDP component-2 candidate was rejected")
	}
	unsupportedComponent := offer.SDP + "a=candidate:1 3 udp 2122260223 127.0.0.1 50000 typ host\r\n"
	if _, err := validateOffer(unsupportedComponent, m.opts.MaxPeers); !errors.Is(err, media.ErrInvalid) {
		t.Fatal("unsupported candidate component was accepted")
	}
	flood := offer.SDP + strings.Repeat("a=candidate:1 1 udp 2122260223 127.0.0.1 50000 typ host\r\n", 129)
	if _, err := validateOffer(flood, m.opts.MaxPeers); !errors.Is(err, media.ErrLimit) {
		t.Fatal("embedded candidate flood was not bounded")
	}
	// Exercise the upstream warning itself as a defense-in-depth regression:
	// bypassing our validator still must not pass candidate/credentials to slog.
	probe, err := m.api.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()
	if err := probe.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: malformed}); err == nil {
		t.Fatal("upstream malformed candidate probe unexpectedly succeeded")
	}
	logged := output.String()
	if strings.Contains(logged, sensitive) || strings.Contains(logged, "a=candidate:") || strings.Contains(logged, "a=ice-pwd:") || strings.Contains(logged, "a=ice-ufrag:") {
		t.Fatal("raw SDP/candidate content leaked from upstream logger")
	}
	for _, line := range strings.Split(offer.SDP, "\r\n") {
		if strings.HasPrefix(line, "a=ice-pwd:") || strings.HasPrefix(line, "a=ice-ufrag:") {
			if credential := strings.SplitN(line, ":", 2)[1]; credential != "" && strings.Contains(logged, credential) {
				t.Fatal("ICE credential leaked from upstream logger")
			}
		}
	}
	if !strings.Contains(logged, "pion_warning") || !strings.Contains(logged, `"component":"pc"`) {
		t.Fatal("safe upstream warning metadata missing")
	}
}
