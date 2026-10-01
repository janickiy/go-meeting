package websocket

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
)

// TestICEAuthenticationTTLAndConnectionCap проверяет защищённую выдачу TURN,
// no-store/TTL/secret redaction и освобождение reservation после отказа handshake.
// t использует фиктивные secrets и не подключается к Redis, SFU или Coturn.
func TestICEAuthenticationTTLAndConnectionCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tokens, err := security.NewTokenService(strings.Repeat("j", 40))
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("t", 40)
	h := NewHandler(nil, tokens, nil, nil, config.RealtimeConfig{MaxConnections: 1, TURN: config.TURNConfig{URLs: []string{"turn:relay.example:3478?transport=tcp"}, Secret: secret, TTL: 5 * time.Minute, ForceRelay: true}})
	router := gin.New()
	h.RegisterRoutes(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/webrtc/config", nil))
	if response.Code != 401 {
		t.Fatal("unauthenticated TURN access")
	}
	token, err := tokens.Issue(uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	var previous string
	for range 2 {
		req := httptest.NewRequest("GET", "/api/v1/webrtc/config", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var ice domain.ICEConfig
		if err := json.Unmarshal(response.Body.Bytes(), &ice); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), secret) || ice.ICETransportPolicy != "relay" || len(ice.ICEServers) != 1 || ice.ExpiresAt <= time.Now().Unix() || ice.ExpiresAt > time.Now().Add(6*time.Minute).Unix() {
			t.Fatal("invalid temporary config")
		}
		if ice.ICEServers[0].Username == previous {
			t.Fatal("reused TURN username")
		}
		previous = ice.ICEServers[0].Username
	}
	path := "/api/v1/conferences/" + uuid.NewString() + "/ws"
	h.connections.Store(1)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	if response.Code != 503 || h.connections.Load() != 1 {
		t.Fatal("physical cap/reservation")
	}
	h.connections.Store(0)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
	if response.Code != 401 || h.connections.Load() != 0 {
		t.Fatal("failed handshake leaked reservation")
	}
}
