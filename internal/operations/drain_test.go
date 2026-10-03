package operations

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/config"
)

// TestDrainAuthorizationAndReadiness проверяет закрытый, повторяемый контракт завершения.
// @args t — контекст проверки HTTP и запрета новых прикладных запросов.
func TestDrainAuthorizationAndReadiness(t *testing.T) {
	r := New("test", "test", config.OperationsConfig{MetricsSecret: strings.Repeat("m", 32)}, nil)
	r.ready.Store(true)
	began := false
	r.ConfigureDrain(func() { began = true }, func() int { return 2 })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(r.Middleware())
	r.RegisterGin(router)
	router.GET("/work", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := func(method, path, secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if request("POST", "/operations/drain", "wrong").Code != 404 || began {
		t.Fatal("unprotected drain")
	}
	w := request("POST", "/operations/drain", r.Config.MetricsSecret)
	if w.Code != 200 || !began || r.Ready() || !strings.Contains(w.Body.String(), `"active":2`) {
		t.Fatal(w.Body.String())
	}
	if request("GET", "/work", "").Code != 503 {
		t.Fatal("new work admitted")
	}
	if request("GET", "/version", "").Code != 200 || request("GET", "/operations/drain", r.Config.MetricsSecret).Code != 200 {
		t.Fatal("operations unavailable after drain")
	}
}
