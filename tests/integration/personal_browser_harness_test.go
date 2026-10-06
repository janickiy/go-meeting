package integration_test

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Explicitly opted-in local browser harness. It never connects to production or changes Compose services.
// The control file contains temporary tokens, has mode 0600 and is removed with the fixture database.
func TestPersonalBrowserHarness(t *testing.T) {
	path := os.Getenv("RECORDER_PERSONAL_BROWSER_FIXTURE")
	if path == "" {
		t.Skip("opt-in local browser harness")
	}
	if !strings.HasPrefix(path, os.TempDir()) && !strings.Contains(path, "/tmp/direct-messages-") {
		t.Fatal("fixture must be in a temporary directory")
	}
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	events := &personalusecase.Events{Members: repo, Bus: bus}
	storage, err := s3storage.NewClient(ctx, os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY"), "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := chatusecase.NewService(ctx, pg.NewDirectChatRepository(f.db), storage, events)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := authusecase.NewService(pg.NewUserRepository(f.db), security.PasswordHasher{}, f.tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(pg.NewAuthSessionRepository(f.db))
	router := gin.New()
	router.Use(gin.Recovery())
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(auth).WithSessionCookies("http://127.0.0.1:5174", false), conferencesapp.NewHandler(f.service), middleware.Authenticate(auth))
	httptransport.RegisterPersonalRoutes(router, &personalapp.Handler{Repo: repo, Events: events}, chatapp.NewHandler(service).ForConversations(), middleware.Authenticate(auth), nil)
	cfg := f.config
	cfg.PingInterval = time.Second
	cfg.PongTimeout = 3 * time.Second
	cfg.SessionTTL = 5 * time.Second
	cfg.AllowedOrigins = []string{"http://127.0.0.1:5174"}
	global := wstransport.NewUserHandler(auth, redisinfra.NewRealtimeStore(f.redis, f.config.Namespace+":user-ws"), nil, cfg, bus, redisinfra.NewUserPresence(f.redis, f.config.Namespace, cfg.SessionTTL), repo)
	global.RegisterRoutes(router)
	defer global.Shutdown()
	server := httptest.NewServer(router)
	defer server.Close()
	data, _ := json.Marshal(map[string]any{"url": server.URL, "alice": map[string]string{"id": f.owner.ID, "token": f.ownerToken, "name": "Alice"}, "bob": map[string]string{"id": f.member.ID, "token": f.memberToken, "name": "Bob"}})
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	t.Log("isolated personal browser API ready")
	timer := time.NewTimer(4 * time.Minute)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("browser harness expired")
		case <-tick.C:
			if _, err = os.Stat(path + ".stop"); err == nil {
				_ = os.Remove(path + ".stop")
				return
			}
		}
	}
}
