package httptransport

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
)

type failedCompletedRecords struct{}

func (failedCompletedRecords) ListCompletedRecords(context.Context, int) ([]s3storage.CompletedRecord, error) {
	return nil, errors.New("private storage credentials/path")
}

func TestDebugStorageErrorsAreSafe(t *testing.T) {
	router := gin.New()
	RegisterDebugRoutes(router, failedCompletedRecords{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/debug/records/completed", nil))
	if w.Code != 500 || w.Body.String() != `{"message":"internal server error","status":"failed"}` {
		t.Fatalf("unsafe debug response: %d %s", w.Code, w.Body.String())
	}
}
