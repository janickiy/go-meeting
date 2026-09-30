package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
)

func TestDebugCompletedRecordsReadsFromStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	lister := &fakeCompletedRecordsLister{
		items: []s3storage.CompletedRecord{
			{
				RecordID:     "22222222-2222-4222-8222-222222222222",
				Status:       "ready",
				PreviewURL:   "http://localhost:9000/recordings/preview.jpg",
				FinalURL:     "http://localhost:9000/recordings/final.mp4",
				SizeBytes:    1024,
				LastModified: time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	router := gin.New()
	httptransport.RegisterDebugRoutes(router, lister)

	request := httptest.NewRequest(http.MethodGet, "/debug/records/completed?limit=2", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	assertStatus(t, response.Code, http.StatusOK)
	assertJSONField(t, response.Body.String(), "status", "success")
	if lister.limit != 2 {
		t.Fatalf("limit = %d, want 2", lister.limit)
	}
	if body := response.Body.String(); !containsAll(body, "22222222-2222-4222-8222-222222222222", "previewUrl", "finalUrl") {
		t.Fatalf("unexpected body: %s", body)
	}
}

func containsAll(body string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(body, value) {
			return false
		}
	}

	return true
}

type fakeCompletedRecordsLister struct {
	items []s3storage.CompletedRecord
	limit int
}

func (l *fakeCompletedRecordsLister) ListCompletedRecords(_ context.Context, limit int) ([]s3storage.CompletedRecord, error) {
	l.limit = limit
	return l.items, nil
}
