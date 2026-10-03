package mediaworker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/infrastructure/sfu"
)

// stageFourFixture подготавливает или проверяет часть тестового сценария «этап четыре тестовое окружение».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*fixture): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (*sfu.Manager): значение, подготовленное операцией для вызывающей стороны.
func stageFourFixture(t *testing.T) (*fixture, *sfu.Manager) {
	t.Helper()
	f := newFixture(t)
	engine, err := sfu.NewManager(sfu.Options{NegotiationTimeout: 10 * time.Second, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.cfg
	cfg.OperationTimeout = 2 * time.Second
	cfg.OwnershipTTL = 20 * time.Second
	h := NewHandler(cfg, f.registry, f.sessions, f.tickets, engine, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (any): значение, подготовленное операцией для вызывающей стороны. */func() any { return engine.Snapshot() }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.workerDeadline = time.Now().Add(time.Hour)
	h.ready.Store(true)
	f.h = h
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = engine.Shutdown(context.Background()) })
	return f, engine
}

// TestPolicyJoinOrderingAndCrossLeaseRejection проверяет сценарий «политика Join порядок и Cross аренда Rejection», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestPolicyJoinOrderingAndCrossLeaseRejection(t *testing.T) {
	f, engine := stageFourFixture(t)
	f.cmd.Policy = &media.ParticipantPolicy{Version: 1}
	id := f.join(t)
	// Сохраняем комнату активной при первом выходе и повторном присоединении участника.
	other := f.cmd.Binding
	other.ParticipantID = uuid.NewString()
	other.ConnectionID = uuid.NewString()
	other.SessionID = uuid.NewString()
	if _, err := engine.Join(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	policy := media.Command{RequestID: uuid.NewString(), ConferenceID: f.cmd.Binding.ConferenceID, ParticipantID: f.cmd.Binding.ParticipantID, Route: f.cmd.Route, Policy: &media.ParticipantPolicy{Version: 2, Kicked: true}}
	if status, _ := f.request("policy", policy, "wrong"); status != 401 {
		t.Fatal("unauthenticated policy accepted", status)
	}
	stale := policy
	stale.Route.LeaseID = uuid.NewString()
	if status, _ := f.request("policy", stale, f.cfg.InternalSecret); status != 409 {
		t.Fatal("foreign lease accepted", status)
	}
	if status, _ := f.request("policy", policy, f.cfg.InternalSecret); status != 200 {
		t.Fatal(status)
	}
	if _, ok := engine.PeerBinding(id); ok {
		t.Fatal("kicked peer retained")
	}
	f.cmd.Policy = &media.ParticipantPolicy{Version: 3}
	if resumed := f.join(t); resumed == id {
		t.Fatal("join did not recreate transport")
	}
	// Запоздалая команда версии 2 не может удалить восстановленного участника версии 3.
	if status, _ := f.request("policy", policy, f.cfg.InternalSecret); status != 200 {
		t.Fatal(status)
	}
	if len(engine.Bindings()) != 2 {
		t.Fatal("stale policy overwrote rejoin")
	}
}

// TestEgressAuthenticationLeaseAndLongLivedWriteDeadline проверяет авторизацию выходного потока, аренду и срок длительной записи данных.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestEgressAuthenticationLeaseAndLongLivedWriteDeadline(t *testing.T) {
	f, engine := stageFourFixture(t)
	server := httptest.NewUnstartedServer(f.h.Routes())
	server.Config.WriteTimeout = 50 * time.Millisecond
	server.Start()
	defer server.Close()
	request := media.EgressRequest{RequestID: uuid.NewString(), ConferenceID: f.cmd.Binding.ConferenceID, RecordingID: uuid.NewString(), Route: f.cmd.Route, SegmentDurationSec: 1}
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - secret (string): секрет подписи или внутренней авторизации компонента.
	//   - value (media.EgressRequest): значение для проверки, нормализации или преобразования.
	//
	// @return:
	//   - результат 1 (*http.Response): значение, подготовленное операцией для вызывающей стороны.
	send := func(secret string, value media.EgressRequest) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(value)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/internal/media/egress", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("X-Request-ID", value.RequestID)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := send("wrong", request)
	_ = response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal(response.StatusCode)
	}
	stale := request
	stale.Route.LeaseID = uuid.NewString()
	response = send(f.cfg.InternalSecret, stale)
	_ = response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal(response.StatusCode)
	}
	response = send(f.cfg.InternalSecret, request)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	var frame media.EgressFrame
	if decoder.Decode(&frame) != nil || frame.Type != "hello" {
		t.Fatal("missing egress hello")
	}
	duplicate := send(f.cfg.InternalSecret, request)
	_ = duplicate.Body.Close()
	if duplicate.StatusCode != 409 {
		t.Fatal("duplicate recording subscription admitted", duplicate.StatusCode)
	}
	for i := 0; i < 2; i++ {
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal("stream inherited short HTTP write timeout", err)
		}
		if frame.Type != "ping" {
			t.Fatal(frame.Type)
		}
	}
	f.h.releaseEmpty(context.Background())
	if !engine.HasConference(request.ConferenceID) || !f.h.leaseValid(request.ConferenceID, request.Route) {
		t.Fatal("empty recording room lost ownership")
	}
	_ = response.Body.Close()
	deadline := time.Now().Add(time.Second)
	for engine.Snapshot().RecordingOutputs != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if engine.Snapshot().RecordingOutputs != 0 {
		t.Fatal("disconnected recorder retained egress")
	}
}
