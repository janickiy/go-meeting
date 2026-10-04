package recordings_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"github.com/janickiy/go-recorder/internal/usecase/recordings"
)

// startProbe изолирует сценарий старта от БД и считает создание, чтение и публикацию.
// Неиспользуемые методы встроенного Repository намеренно не реализованы: их вызов
// означал бы неожиданное расширение проверяемого сценария.
// @params Repository — остальные методы хранилища; mu — защита конкурентного доступа;
// owner — единственный разрешённый пользователь; record — текущая запись;
// starts/reads/events — наблюдаемые эффекты успешного старта и остановки.
type startProbe struct {
	recordings.Repository
	mu     sync.Mutex
	owner  string
	record records.Record
	starts int
	reads  int
	events []realtime.Envelope
}

// Start проверяет совместимый путь запуска составной записи.
// @args ctx — срок операции; user/conference — инициатор и встреча; seconds — длительность фрагмента.
// @return созданная запись, признак создания и ошибка запрета или конфликта.
func (p *startProbe) Start(ctx context.Context, user, conference string, seconds int) (records.Record, bool, error) {
	return p.StartMode(ctx, user, conference, seconds, records.ModeComposite)
}

// StartMode имитирует атомарный запрет повторной записи независимо от её режима.
// @args ctx — срок операции; user/conference — инициатор и встреча; seconds — секунды фрагмента; mode — стратегия.
// @return единственная запись либо конфликт; чужому пользователю — запрет без сведений о записи.
func (p *startProbe) StartMode(_ context.Context, user, conference string, seconds int, mode string) (records.Record, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if user != p.owner {
		return records.Record{}, false, apperrors.ErrForbidden
	}
	if p.record.UUID != "" {
		return p.record, false, apperrors.New(apperrors.ErrConflict, "recording is already active")
	}
	p.record = records.Record{UUID: uuid.NewString(), ConferenceID: conference, RequestedBy: &p.owner, Status: records.StatusStarting, Mode: mode, SegmentDurationSec: seconds}
	p.starts++
	return p.record, true, nil
}

// Stop сохраняет обычный переход в stopping, не заменяя первоначального инициатора.
// @args ctx — срок операции; user/conference/id — участник, встреча и запись.
// @return состояние остановки либо запрет доступа.
func (p *startProbe) Stop(_ context.Context, user, _, _ string) (records.Record, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if user != p.owner {
		return records.Record{}, apperrors.ErrForbidden
	}
	p.record.Status = records.StatusStopping
	return p.record, nil
}

// ReadComposite учитывает построение карточки только успешно запрошенной записи.
// @args ctx — срок операции; id — внешний UUID записи.
// @return карточка сохранённой записи.
func (p *startProbe) ReadComposite(_ context.Context, _ string) (records.RecordCard, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return records.RecordCard{Record: p.record}, nil
}

// Broadcast сохраняет каждое опубликованное уведомление для проверки дублей.
// @args ctx — срок операции; event — конверт уведомления участникам.
// @return nil: транспорт теста принимает событие.
func (p *startProbe) Broadcast(_ context.Context, event realtime.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

// TestStartConflictDoesNotReturnCardOrPublish проверяет, что повторный старт не
// превращается в успех, не читает карточку и не создаёт второе уведомление.
// @args t — исполнитель теста с проверкой побочных эффектов.
func TestStartConflictDoesNotReturnCardOrPublish(t *testing.T) {
	p := &startProbe{owner: uuid.NewString()}
	service := recordings.NewConferenceService(p, p, nil, nil, p)
	conference := uuid.NewString()
	card, err := service.Start(context.Background(), p.owner, conference, records.ConferenceStartRequest{})
	if err != nil || card.UUID == "" || card.Mode != records.ModeComposite || card.SegmentDurationSec != 5 {
		t.Fatalf("initial start: %+v / %v", card, err)
	}
	for _, mode := range []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
		duplicate, err := service.Start(context.Background(), p.owner, conference, records.ConferenceStartRequest{Mode: mode})
		if !errors.Is(err, apperrors.ErrConflict) || err.Error() != "recording is already active" || duplicate.UUID != "" {
			t.Fatalf("duplicate %s: %+v / %v", mode, duplicate, err)
		}
	}
	if p.starts != 1 || p.reads != 1 || len(p.events) != 1 {
		t.Fatalf("duplicate effects: starts=%d reads=%d events=%d", p.starts, p.reads, len(p.events))
	}
	assertStartEvent(t, p.events[0], "recording.starting", card, p.owner)
	stopped, err := service.Stop(context.Background(), p.owner, conference, card.UUID)
	if err != nil || stopped.Status != records.StatusStopping || len(p.events) != 2 {
		t.Fatalf("stop semantics changed: %+v / %v", stopped, err)
	}
	assertStartEvent(t, p.events[1], "recording.stopping", stopped, p.owner)
}

// assertStartEvent проверяет адресатов и инициатора, передаваемые UI в событии записи.
// @args t — исполнитель теста; event — фактическое уведомление; kind — тип;
// card — ожидаемая запись; owner — первоначальный инициатор.
func assertStartEvent(t *testing.T, event realtime.Envelope, kind string, card records.RecordCard, owner string) {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatal(err)
	}
	if event.Type != kind || event.ConferenceID != card.ConferenceID || data["recordingId"] != card.UUID || data["conferenceId"] != card.ConferenceID || data["requestedBy"] != owner || data["mode"] != card.Mode || data["status"] != card.Status {
		t.Fatalf("unexpected event: %+v / %+v", event, data)
	}
}

// identityVerifier заменяет подпись JWT только в изолированном тесте обработчика.
type identityVerifier struct{}

// Verify принимает UUID тестового пользователя; реальные ключи авторизации не используются.
// @args token — синтетический UUID пользователя.
// @return идентификатор либо ошибка некорректного формата.
func (identityVerifier) Verify(token string) (string, error) {
	id, err := uuid.Parse(token)
	return id.String(), err
}

// TestConcurrentStartHTTPHasOneAcceptance проверяет HTTP-контракт при одновременных
// запросах разных режимов: один 202, остальные 409 и ровно одно событие старта.
// @args t — исполнитель теста конкурентных HTTP-запросов.
func TestConcurrentStartHTTPHasOneAcceptance(t *testing.T) {
	p := &startProbe{owner: uuid.NewString()}
	service := recordings.NewConferenceService(p, p, nil, nil, p)
	router := gin.New()
	router.POST("/conferences/:id/recordings", httpmiddleware.Authenticate(identityVerifier{}), recordingsapp.NewHandler(service).Start)
	path := "/conferences/" + uuid.NewString() + "/recordings"
	modes := []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus}
	responses := make(chan *httptest.ResponseRecorder, 16)
	gate := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(mode string) {
			defer wg.Done()
			<-gate
			responses <- recordingStartRequest(router, path, p.owner, mode)
		}(modes[i%len(modes)])
	}
	close(gate)
	wg.Wait()
	close(responses)
	accepted, conflicts := 0, 0
	for response := range responses {
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		switch response.Code {
		case http.StatusAccepted:
			accepted++
			if body["status"] != "success" || body["item"] == nil {
				t.Fatalf("bad accepted response: %s", response.Body.String())
			}
		case http.StatusConflict:
			conflicts++
			if body["status"] != "failed" || body["message"] != "recording is already active" || body["item"] != nil {
				t.Fatalf("conflict leaked a success/card: %s", response.Body.String())
			}
		default:
			t.Fatalf("unexpected HTTP %d: %s", response.Code, response.Body.String())
		}
	}
	if accepted != 1 || conflicts != 15 || p.starts != 1 || p.reads != 1 || len(p.events) != 1 {
		t.Fatalf("accepted=%d conflicts=%d starts=%d reads=%d events=%d", accepted, conflicts, p.starts, p.reads, len(p.events))
	}
	for _, mode := range modes {
		response := recordingStartRequest(router, path, uuid.NewString(), mode)
		if response.Code != http.StatusForbidden || response.Body.String() != "{\"message\":\"access denied\",\"status\":\"failed\"}" {
			t.Fatalf("unauthorized participant learned recording state: %d %s", response.Code, response.Body.String())
		}
	}
	if p.reads != 1 || len(p.events) != 1 {
		t.Fatal("forbidden request read or published a recording")
	}
}

// recordingStartRequest вызывает настоящий HTTP-обработчик без сетевого порта.
// @args router — проверяемые маршруты; path — адрес конференции; user — тестовый
// пользователь; mode — режим старта.
// @return записанный HTTP-ответ с кодом и телом.
func recordingStartRequest(router http.Handler, path, user, mode string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(records.ConferenceStartRequest{Mode: mode})
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+user)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
