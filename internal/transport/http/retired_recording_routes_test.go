package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// TestLegacyRecordingAPIIsRetiredForEveryIdentity проверяет, что публичный,
// прежний и отладочный пути не выдают данные и не принимают команды, даже если
// клиент предъявляет настоящий JWT владельца, другого аккаунта или гостя.
//
// @args
//   - t: контекст теста с отдельными сценариями метода, пути и учётных данных.
func TestLegacyRecordingAPIIsRetiredForEveryIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := security.NewTokenService(strings.Repeat("test-secret-", 4))
	if err != nil {
		t.Fatal(err)
	}
	owner, err := tokens.Issue("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	other, err := tokens.Issue("22222222-2222-4222-8222-222222222222")
	if err != nil {
		t.Fatal(err)
	}
	guest, err := tokens.IssueGuest("33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444")
	if err != nil {
		t.Fatal(err)
	}
	credentials := map[string]string{
		"anonymous": "", "owner": "Bearer " + owner, "other-account": "Bearer " + other,
		"guest": "Bearer " + guest, "invalid-token": "Bearer invalid",
	}
	paths := []string{
		"/api/v1/records", "/api/v1/records/", "/api/v1/records/start",
		"/api/v1/records/end", "/api/v1/records/count-by-conference?conferenceIds=44444444-4444-4444-8444-444444444444",
		"/api/v1/records/55555555-5555-4555-8555-555555555555",
		"/api/v1/records/55555555-5555-4555-8555-555555555555/webrtc/offer",
		"/api/v1/records/unknown/path", "/api/records", "/api/records/start",
		"/api/records/55555555-5555-4555-8555-555555555555/webrtc/offer",
		"/debug/records", "/debug/records/completed?limit=100", "/debug/records/unknown",
		"/debug/webrtc-smoke?gatewayBase=https://attacker.invalid",
	}
	router := NewRouter()
	for identity, authorization := range credentials {
		for _, path := range paths {
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead} {
				t.Run(identity+"/"+method+path, func(t *testing.T) {
					request := httptest.NewRequest(method, path, strings.NewReader(`{"requestedBy":"11111111-1111-4111-8111-111111111111"}`))
					request.Header.Set("Authorization", authorization)
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != http.StatusGone {
						t.Fatalf("retired endpoint returned %d: %s", response.Code, response.Body.String())
					}
					if response.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("retired endpoint may be cached")
					}
					var payload map[string]string
					if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload) != 2 || payload["status"] != "failed" || payload["message"] == "" {
						t.Fatalf("unexpected response or leaked record data: %s", response.Body.String())
					}
				})
			}
		}
	}
}

// TestRetiredRecordingRoutesPreserveConferenceAuthorization проверяет, что
// закрытие старого API не перекрывает действующие маршруты записи конференции:
// без JWT они запрещены, а проверенный пользователь передаётся прикладному слою.
//
// @args
//   - t: контекст теста совместимости нового и закрытого контрактов.
func TestRetiredRecordingRoutesPreserveConferenceAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := security.NewTokenService(strings.Repeat("test-secret-", 4))
	if err != nil {
		t.Fatal(err)
	}
	const ownerID = "11111111-1111-4111-8111-111111111111"
	const otherID = "22222222-2222-4222-8222-222222222222"
	const conferenceID = "44444444-4444-4444-8444-444444444444"
	const recordID = "55555555-5555-4555-8555-555555555555"
	service := &conferenceRecordingAccessProbe{ownerID: ownerID}
	router := NewRouter()
	RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(service), httpmiddleware.Authenticate(tokens))
	for _, test := range []struct {
		name, userID string
		status       int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"permitted-account", ownerID, http.StatusOK},
		{"other-account", otherID, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, APIV1Prefix+"/conferences/"+conferenceID+"/recordings/"+recordID, nil)
			if test.userID != "" {
				token, err := tokens.Issue(test.userID)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("conference recording returned %d: %s", response.Code, response.Body.String())
			}
		})
	}
	if service.calls != 2 || service.userID != otherID || service.conferenceID != conferenceID || service.recordID != recordID {
		t.Fatalf("authenticated conference scope was not preserved: %+v", service)
	}
}

// conferenceRecordingAccessProbe проверяет передачу подтверждённой личности и
// области конференции в современный прикладной слой; внешних сервисов не вызывает.
// @params
//   - ownerID: единственный аккаунт, которому тестовый сценарий разрешает чтение.
//   - userID, conferenceID, recordID: параметры последнего вызова чтения.
//   - calls: число вызовов прикладного слоя после HTTP-авторизации.
type conferenceRecordingAccessProbe struct {
	ownerID, userID, conferenceID, recordID string
	calls                                   int
}

// Read возвращает тестовую карточку только разрешённому аккаунту.
// @args
//   - ctx: контекст тестового запроса.
//   - userID: аккаунт, извлечённый из проверенного JWT.
//   - conferenceID: конференция из маршрута.
//   - recordID: запись из маршрута.
//
// @return карточка или отказ в доступе другому аккаунту.
func (p *conferenceRecordingAccessProbe) Read(_ context.Context, userID, conferenceID, recordID string) (records.RecordCard, error) {
	p.calls++
	p.userID, p.conferenceID, p.recordID = userID, conferenceID, recordID
	if userID != p.ownerID {
		return records.RecordCard{}, apperrors.ErrForbidden
	}
	return records.RecordCard{Record: records.Record{UUID: recordID, ConferenceID: conferenceID}}, nil
}

// Start не используется в тесте чтения; вызов означает неправильный маршрут.
// @args
//   - ctx, userID, conferenceID, request: параметры интерфейса современного API.
//
// @return ошибка, чтобы случайное выполнение команды не считалось успешным.
func (*conferenceRecordingAccessProbe) Start(context.Context, string, string, records.ConferenceStartRequest) (records.RecordCard, error) {
	return records.RecordCard{}, apperrors.ErrInternal
}

// Stop не используется в тесте чтения; вызов означает неправильный маршрут.
// @args
//   - ctx, userID, conferenceID, recordID: параметры интерфейса современного API.
//
// @return ошибка, чтобы случайная остановка не считалась успешной.
func (*conferenceRecordingAccessProbe) Stop(context.Context, string, string, string) (records.RecordCard, error) {
	return records.RecordCard{}, apperrors.ErrInternal
}

// List не используется в тесте чтения; вызов означает неправильный маршрут.
// @args
//   - ctx, userID, conferenceID, limit, offset: параметры списка современного API.
//
// @return ошибка, чтобы случайное чтение списка не считалось успешным.
func (*conferenceRecordingAccessProbe) List(context.Context, string, string, int, int) ([]records.RecordCard, error) {
	return nil, apperrors.ErrInternal
}
