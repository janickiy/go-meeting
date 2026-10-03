package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	engagementapp "github.com/janickiy/go-recorder/internal/app/engagement"
)

// TestEngagementRoutesOnlyExposeReactions проверяет удаление старых команд из HTTP API.
// @args t — контекст теста; хранилища и внешние сервисы не используются.
func TestEngagementRoutesOnlyExposeReactions(t *testing.T) {
	router := gin.New()
	RegisterEngagementRoutes(router, engagementapp.NewHandler(nil, nil, "test"), func(c *gin.Context) { c.Next() })
	routes := router.Routes()
	if len(routes) != 1 || routes[0].Method != http.MethodPost || routes[0].Path != APIV1Prefix+"/conferences/:id/reactions" {
		t.Fatalf("unexpected engagement routes: %+v", routes)
	}
	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/conferences/00000000-0000-4000-8000-000000000001/hands"},
		{http.MethodPut, "/conferences/00000000-0000-4000-8000-000000000001/participants/00000000-0000-4000-8000-000000000002/hand"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(target.method, APIV1Prefix+target.path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("removed route %s %s returned %d", target.method, target.path, response.Code)
		}
	}
}
