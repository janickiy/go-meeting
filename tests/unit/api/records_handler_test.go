package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	recordsapp "github.com/janickiy/go-recorder/internal/app/records"
	"github.com/janickiy/go-recorder/internal/domain/records"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
)

func TestRecordsStartValidationErrorResponse(t *testing.T) {
	router, service := newRecordsRouter("")

	response := performJSON(router, http.MethodPost, "/api/v1/records/start", `{"conferenceId":"bad","qualityMode":"auto"}`)

	assertStatus(t, response.Code, http.StatusBadRequest)
	assertJSONField(t, response.Body.String(), "status", "failed")
	assertJSONField(t, response.Body.String(), "message", "conferenceId must be valid UUID")
	if service.startCalled {
		t.Fatal("Start() was called for invalid request")
	}
}

func TestRecordsLegacyStartRouteIsRemoved(t *testing.T) {
	router, service := newRecordsRouter("")

	response := performJSON(router, http.MethodPost, "/api/records/start", `{
		"conferenceId":"11111111-1111-4111-8111-111111111111",
		"qualityMode":"auto"
	}`)

	assertStatus(t, response.Code, http.StatusNotFound)
	if service.startCalled {
		t.Fatal("Start() was called for removed legacy route")
	}
}

func TestRecordsStartAcceptedResponse(t *testing.T) {
	router, service := newRecordsRouter("")
	service.startResponse = records.StartResponse{
		Status:       records.StatusStarting,
		Message:      "Record job accepted",
		RecordID:     "22222222-2222-4222-8222-222222222222",
		ConferenceID: "11111111-1111-4111-8111-111111111111",
	}

	response := performJSON(router, http.MethodPost, "/api/v1/records/start", `{
		"conferenceId":"11111111-1111-4111-8111-111111111111",
		"requestedBy":"33333333-3333-4333-8333-333333333333",
		"qualityMode":"auto",
		"segmentDurationSec":7
	}`)

	assertStatus(t, response.Code, http.StatusAccepted)
	assertJSONField(t, response.Body.String(), "recordId", "22222222-2222-4222-8222-222222222222")
	if !service.startCalled {
		t.Fatal("Start() was not called")
	}
	if service.startRequest.SegmentDurationSec != 7 {
		t.Fatalf("SegmentDurationSec = %d, want 7", service.startRequest.SegmentDurationSec)
	}
	if service.startRequest.Quality != records.DefaultVideoQuality {
		t.Fatalf("Quality = %q, want %q", service.startRequest.Quality, records.DefaultVideoQuality)
	}
}

func TestRecordsStartConferenceLockConflict(t *testing.T) {
	router, service := newRecordsRouter("")
	service.startErr = records.ErrConferenceAlreadyRecording

	response := performJSON(router, http.MethodPost, "/api/v1/records/start", `{
		"conferenceId":"11111111-1111-4111-8111-111111111111",
		"qualityMode":"auto",
		"segmentDurationSec":7
	}`)

	assertStatus(t, response.Code, http.StatusConflict)
	assertJSONField(t, response.Body.String(), "status", "failed")
	assertJSONField(t, response.Body.String(), "message", records.ErrConferenceAlreadyRecording.Error())
}

func TestRecordsEndReadsRecordIDFromBody(t *testing.T) {
	router, service := newRecordsRouter("")

	response := performJSON(router, http.MethodPost, "/api/v1/records/end", `{
		"recordId":"22222222-2222-4222-8222-222222222222",
		"reason":"client_stop"
	}`)

	assertStatus(t, response.Code, http.StatusAccepted)
	assertJSONField(t, response.Body.String(), "status", "success")
	if !service.stopCalled {
		t.Fatal("Stop() was not called")
	}
	if service.stopRequest.RecordID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("RecordID = %q", service.stopRequest.RecordID)
	}
	if service.stopRequest.Reason != "client_stop" {
		t.Fatalf("Reason = %q", service.stopRequest.Reason)
	}
}

func TestRecordsListUsesQueryFallbacks(t *testing.T) {
	router, service := newRecordsRouter("")
	service.listResponse = []records.RecordCard{
		{Record: records.Record{UUID: "22222222-2222-4222-8222-222222222222", Status: records.StatusReady}},
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/records?limit=bad&offset=bad", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertStatus(t, response.Code, http.StatusOK)
	assertJSONField(t, response.Body.String(), "status", "success")
	if service.listLimit != 20 {
		t.Fatalf("limit = %d, want fallback 20", service.listLimit)
	}
	if service.listOffset != 0 {
		t.Fatalf("offset = %d, want fallback 0", service.listOffset)
	}
}

func TestRecordsCountByConference(t *testing.T) {
	router, service := newRecordsRouter("")
	duration := 42
	createdAt := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	service.countByConferenceResponse = []records.ConferenceRecordSummary{
		{
			ConferenceID: "11111111-1111-4111-8111-111111111111",
			RecordsCount: 1,
			Records: []records.ConferenceRecordItem{
				{
					RecordID:    "22222222-2222-4222-8222-222222222222",
					Status:      records.StatusReady,
					FinalURL:    "https://storage.local/final.mp4",
					PreviewURL:  "https://storage.local/preview.jpg",
					DurationSec: &duration,
					CreatedAt:   createdAt,
				},
			},
		},
		{ConferenceID: "33333333-3333-4333-8333-333333333333", RecordsCount: 0, Records: []records.ConferenceRecordItem{}},
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/records/count-by-conference?conferenceIds[]=11111111-1111-4111-8111-111111111111&conferenceIds[]=33333333-3333-4333-8333-333333333333&status=ready", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertStatus(t, response.Code, http.StatusOK)
	assertJSONField(t, response.Body.String(), "status", "success")
	if !service.countByConferenceCalled {
		t.Fatal("CountByConference() was not called")
	}
	if service.countByConferenceStatus != records.StatusReady {
		t.Fatalf("status = %q, want %q", service.countByConferenceStatus, records.StatusReady)
	}
	if got, want := len(service.countByConferenceIDs), 2; got != want {
		t.Fatalf("conference IDs len = %d, want %d", got, want)
	}
	assertJSONConferenceSummary(t, response.Body.String(), "11111111-1111-4111-8111-111111111111", 1, "22222222-2222-4222-8222-222222222222", "https://storage.local/final.mp4", "https://storage.local/preview.jpg", 42)
	assertJSONRecordsCount(t, response.Body.String(), "33333333-3333-4333-8333-333333333333", 0)
}

func TestRecordsCountByConferenceSupportsCSVQuery(t *testing.T) {
	router, service := newRecordsRouter("")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/records/count-by-conference?conferenceIds=11111111-1111-4111-8111-111111111111,33333333-3333-4333-8333-333333333333", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertStatus(t, response.Code, http.StatusOK)
	if got, want := len(service.countByConferenceIDs), 2; got != want {
		t.Fatalf("conference IDs len = %d, want %d", got, want)
	}
}

func TestRecordsCountByConferenceValidation(t *testing.T) {
	router, service := newRecordsRouter("")

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/records/count-by-conference", nil))

	assertStatus(t, response.Code, http.StatusBadRequest)
	assertJSONField(t, response.Body.String(), "status", "failed")
	assertJSONField(t, response.Body.String(), "message", "conferenceIds is required")
	if service.countByConferenceCalled {
		t.Fatal("CountByConference() was called for invalid request")
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/records/count-by-conference?conferenceIds[]=bad", nil))

	assertStatus(t, response.Code, http.StatusBadRequest)
	assertJSONField(t, response.Body.String(), "message", "conferenceIds must contain valid UUID values")

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/records/count-by-conference?conferenceIds[]=11111111-1111-4111-8111-111111111111&status=bad", nil))

	assertStatus(t, response.Code, http.StatusBadRequest)
	assertJSONField(t, response.Body.String(), "message", "status must be starting, recording, stopping, finalizing, uploading, ready, partial_ready or failed")
}

func TestRecordsReadNotFoundResponse(t *testing.T) {
	router, service := newRecordsRouter("")
	service.readErr = errors.New("not found")

	request := httptest.NewRequest(http.MethodGet, "/api/v1/records/22222222-2222-4222-8222-222222222222", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertStatus(t, response.Code, http.StatusNotFound)
	assertJSONField(t, response.Body.String(), "status", "failed")
	assertJSONField(t, response.Body.String(), "message", "record not found")
}

func TestRecordsOfferProxiesWorkerAnswer(t *testing.T) {
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("worker method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/records/record-1/webrtc/offer" {
			t.Fatalf("worker path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"answer","sdp":"answer-sdp"}`))
	}))
	defer worker.Close()
	router, _ := newRecordsRouter(worker.URL)

	response := performJSON(router, http.MethodPost, "/api/v1/records/record-1/webrtc/offer", `{"type":"offer","sdp":"offer-sdp"}`)

	assertStatus(t, response.Code, http.StatusOK)
	assertJSONField(t, response.Body.String(), "type", "answer")
	assertJSONField(t, response.Body.String(), "sdp", "answer-sdp")
}

func newRecordsRouter(workerURL string) (*gin.Engine, *fakeRecordService) {
	gin.SetMode(gin.TestMode)
	service := &fakeRecordService{}
	router := gin.New()
	handler := recordsapp.NewHandler(service, workerURL)
	httptransport.RegisterRecordRoutes(router, handler)

	return router, service
}

func performJSON(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	return response
}

func assertStatus(t *testing.T, got int, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func assertJSONField(t *testing.T, body string, key string, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid json %q: %v", body, err)
	}
	got, _ := payload[key].(string)
	if got != want {
		t.Fatalf("%s = %q, want %q; body=%s", key, got, want, body)
	}
}

func assertJSONRecordsCount(t *testing.T, body string, conferenceID string, want int64) {
	t.Helper()
	var payload struct {
		Items []records.ConferenceRecordSummary `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid json %q: %v", body, err)
	}
	for _, item := range payload.Items {
		if item.ConferenceID == conferenceID {
			if item.RecordsCount != want {
				t.Fatalf("recordsCount for %s = %d, want %d", conferenceID, item.RecordsCount, want)
			}
			return
		}
	}
	t.Fatalf("conferenceId %s not found in body=%s", conferenceID, body)
}

func assertJSONConferenceSummary(t *testing.T, body string, conferenceID string, recordsCount int64, recordID string, finalURL string, previewURL string, durationSec int) {
	t.Helper()
	var payload struct {
		Items []records.ConferenceRecordSummary `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid json %q: %v", body, err)
	}
	for _, item := range payload.Items {
		if item.ConferenceID != conferenceID {
			continue
		}
		if item.RecordsCount != recordsCount {
			t.Fatalf("recordsCount for %s = %d, want %d", conferenceID, item.RecordsCount, recordsCount)
		}
		if len(item.Records) != 1 {
			t.Fatalf("records len = %d, want 1; body=%s", len(item.Records), body)
		}
		record := item.Records[0]
		if record.RecordID != recordID {
			t.Fatalf("recordId = %q, want %q", record.RecordID, recordID)
		}
		if record.FinalURL != finalURL {
			t.Fatalf("finalUrl = %q, want %q", record.FinalURL, finalURL)
		}
		if record.PreviewURL != previewURL {
			t.Fatalf("previewUrl = %q, want %q", record.PreviewURL, previewURL)
		}
		if record.DurationSec == nil || *record.DurationSec != durationSec {
			t.Fatalf("durationSec = %v, want %d", record.DurationSec, durationSec)
		}
		return
	}
	t.Fatalf("conferenceId %s not found in body=%s", conferenceID, body)
}

type fakeRecordService struct {
	startCalled   bool
	startRequest  records.StartRequest
	startResponse records.StartResponse
	startErr      error

	stopCalled  bool
	stopRequest records.EndRequest
	stopErr     error

	listLimit    int
	listOffset   int
	listResponse []records.RecordCard
	listErr      error

	countByConferenceCalled   bool
	countByConferenceIDs      []string
	countByConferenceStatus   string
	countByConferenceResponse []records.ConferenceRecordSummary
	countByConferenceErr      error

	readUUID     string
	readResponse records.RecordCard
	readErr      error
}

func (s *fakeRecordService) Start(_ context.Context, request records.StartRequest) (records.StartResponse, error) {
	s.startCalled = true
	s.startRequest = request
	return s.startResponse, s.startErr
}

func (s *fakeRecordService) Stop(_ context.Context, request records.EndRequest) error {
	s.stopCalled = true
	s.stopRequest = request
	return s.stopErr
}

func (s *fakeRecordService) List(_ context.Context, limit int, offset int) ([]records.RecordCard, error) {
	s.listLimit = limit
	s.listOffset = offset
	return s.listResponse, s.listErr
}

func (s *fakeRecordService) CountByConference(_ context.Context, conferenceIDs []string, status string) ([]records.ConferenceRecordSummary, error) {
	s.countByConferenceCalled = true
	s.countByConferenceIDs = conferenceIDs
	s.countByConferenceStatus = status

	return s.countByConferenceResponse, s.countByConferenceErr
}

func (s *fakeRecordService) Read(_ context.Context, uuid string) (records.RecordCard, error) {
	s.readUUID = uuid
	return s.readResponse, s.readErr
}
