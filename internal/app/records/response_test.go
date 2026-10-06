package recordsapp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLegacyFailuresKeepStatusButHideInfrastructureDetails(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		failed(c, status, "postgres password=private or FFmpeg /private/path")
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if w.Code != status || body["status"] != "failed" || body["message"] != "internal server error" {
			t.Fatalf("unsafe legacy error HTTP=%d body=%s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	failed(c, 422, "invalid field")
	if w.Code != 422 || !strings.Contains(w.Body.String(), "invalid field") {
		t.Fatal("validation contract changed")
	}
}
