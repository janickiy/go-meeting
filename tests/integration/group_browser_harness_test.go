package integration_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	foldersapp "github.com/janickiy/go-recorder/internal/app/folders"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
	foldersusecase "github.com/janickiy/go-recorder/internal/usecase/folders"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
)

// Opt-in fixture: random local database, isolated Redis namespace, temporary tokens.
// It deliberately uses the production REST/WS/assets wiring, never the live host.
func TestGroupBrowserHarness(t *testing.T) {
	runPersonalBrowserHarness(t, "RECORDER_GROUP_BROWSER_FIXTURE", "meetrix-group-chats-", false)
}

func runPersonalBrowserHarness(t *testing.T, variable, prefix string, withFolders bool) {
	path := os.Getenv(variable)
	if path == "" {
		t.Skip("opt-in local personal browser harness")
	}
	if filepath.Dir(path) != os.TempDir() && !(filepath.Dir(path) == "/tmp" && strings.HasPrefix(filepath.Base(path), prefix)) {
		t.Fatal("browser fixture must be in OS temporary directory")
	}
	f := stageTwo(t)
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	repo := pg.NewPersonalRepository(f.db)
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	events := &personalusecase.Events{Members: repo, Bus: bus}
	storage, err := s3storage.NewClient(ctx, os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY"), "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		var ids []string
		if err := f.db.WithContext(cleanup).Table("conversations").Pluck("id", &ids).Error; err != nil {
			t.Error(err)
			return
		}
		for _, id := range ids {
			for _, prefix := range []string{"avatars/conversations/" + id + "/", "attachments/direct/" + id + "/"} {
				if err := storage.RemovePrefix(cleanup, prefix); err != nil {
					t.Error("fixture object cleanup", err)
				}
			}
		}
	})
	assets, err := personalusecase.NewAssetService(ctx, pg.NewPersonalAssetRepository(f.db), storage, repo, events)
	if err != nil {
		t.Fatal(err)
	}
	service, err := chatusecase.NewService(ctx, pg.NewDirectChatRepository(f.db), storage, events)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range []func(context.Context){assets.Run, service.Run} {
		workers.Add(1)
		go func(run func(context.Context)) { defer workers.Done(); run(ctx) }(run)
	}
	ur := pg.NewUserRepository(f.db)
	actors := map[string]map[string]string{
		"alice": {"id": f.owner.ID, "token": f.ownerToken, "name": "Alice"},
		"bob":   {"id": f.member.ID, "token": f.memberToken, "name": "Bob"},
	}
	for _, name := range []string{"Charlie", "Dana"} {
		user := users.User{ID: uuid.NewString(), Email: strings.ToLower(name) + "@group-browser.example", PasswordHash: f.owner.PasswordHash, DisplayName: ptr(name)}
		if _, err := ur.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
		token, err := f.tokens.Issue(user.ID)
		if err != nil {
			t.Fatal(err)
		}
		key := "charlie"
		if name == "Dana" {
			key = "dana"
		}
		actors[key] = map[string]string{"id": user.ID, "token": token, "name": name}
	}
	auth, err := authusecase.NewService(ur, security.PasswordHasher{}, f.tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(pg.NewAuthSessionRepository(f.db))
	router := gin.New()
	router.Use(gin.Recovery())
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(auth).WithSessionCookies("http://127.0.0.1:5174", false), conferencesapp.NewHandler(f.service), middleware.Authenticate(auth))
	cfg := f.config
	cfg.PingInterval = time.Second
	cfg.PongTimeout = 3 * time.Second
	cfg.SessionTTL = 5 * time.Second
	cfg.AllowedOrigins = []string{"http://127.0.0.1:5174"}
	presence := redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL)
	h := &personalapp.Handler{Repo: repo, Events: events, Presence: presence}
	httptransport.RegisterPersonalRoutes(router, h, chatapp.NewHandler(service).ForConversations().WithGroupDownloads(assets), middleware.Authenticate(auth), nil)
	httptransport.RegisterPersonalAssetRoutes(router, personalapp.NewAssetHandler(assets), middleware.Authenticate(auth), h.AccountOnly, nil)
	if withFolders {
		httptransport.RegisterFolderRoutes(router, foldersapp.NewHandler(pg.NewFolderRepository(f.db), &foldersusecase.Events{Bus: bus}), middleware.Authenticate(auth), nil)
		httptransport.RegisterChatActionRoutes(router, &chatapp.ActionHandler{Service: chatusecase.NewActionService(pg.NewChatRepository(f.db), nil).WithPresence(presence, nil)}, middleware.Authenticate(auth), nil)
	}
	global := wstransport.NewUserHandler(auth, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":user-ws"), nil, cfg, bus, presence, repo)
	global.RegisterRoutes(router)
	defer global.Shutdown()
	server := httptest.NewServer(router)
	defer server.Close()
	data := map[string]any{"url": server.URL}
	if withFolders {
		direct, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
		if err != nil {
			t.Fatal(err)
		}
		group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Команда папок", MemberIDs: []string{f.member.ID}})
		if err != nil {
			t.Fatal(err)
		}
		data["conversationId"], data["groupId"], data["meetingId"] = direct.ID, group.ID, f.conference.ID
	}
	for key, actor := range actors {
		data[key] = actor
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	t.Log("isolated personal browser API ready")
	timer := time.NewTimer(12 * time.Minute)
	defer timer.Stop()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("personal browser harness expired")
		case <-tick.C:
			if _, err := os.Stat(path + ".stop"); err == nil {
				_ = os.Remove(path + ".stop")
				return
			}
		}
	}
}
