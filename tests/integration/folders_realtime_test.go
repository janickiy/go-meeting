package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	ws "github.com/gorilla/websocket"
	foldersapp "github.com/janickiy/go-recorder/internal/app/folders"
	folderdomain "github.com/janickiy/go-recorder/internal/domain/folders"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	folderusecase "github.com/janickiy/go-recorder/internal/usecase/folders"
)

func TestFoldersRealtimeIsPrivateAcrossInstances(t *testing.T) {
	if os.Getenv("RECORDER_DIRECT_E2E") != "true" {
		t.Skip("real PostgreSQL and Redis required")
	}
	f := stageTwo(t)
	ctx := context.Background()
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	folderHandler := foldersapp.NewHandler(pg.NewFolderRepository(f.db), &folderusecase.Events{Bus: bus})
	servers := []*httptest.Server{}
	handlers := []*wstransport.UserHandler{}
	var api stageOneAPI
	for i := 0; i < 2; i++ {
		r := gin.New()
		r.Use(gin.Recovery())
		httptransport.RegisterFolderRoutes(r, folderHandler, middleware.Authenticate(f.tokens), nil)
		cfg := f.config
		cfg.PingInterval, cfg.PongTimeout, cfg.SessionTTL = time.Second, 3*time.Second, 5*time.Second
		h := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":folders-global"), nil, cfg, bus, redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL), pg.NewPersonalRepository(f.db))
		h.RegisterRoutes(r)
		handlers = append(handlers, h)
		servers = append(servers, httptest.NewServer(r))
		if i == 0 {
			api = stageOneAPI{router: r}
		}
	}
	t.Cleanup(func() {
		for _, h := range handlers {
			h.Shutdown()
		}
		for _, s := range servers {
			s.Close()
		}
	})
	connect := func(index int, token string) <-chan realtime.Envelope {
		request, _ := http.NewRequest("POST", servers[index].URL+"/api/v1/ws-ticket", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var ticket struct{ Ticket string }
		err = json.NewDecoder(response.Body).Decode(&ticket)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != 201 {
			t.Fatal("ticket", response.StatusCode, err)
		}
		conn, _, err := ws.DefaultDialer.Dial(strings.Replace(servers[index].URL, "http:", "ws:", 1)+"/api/v1/ws?ticket="+ticket.Ticket, nil)
		if err != nil {
			t.Fatal(err)
		}
		stream, stop := make(chan realtime.Envelope, 128), make(chan struct{})
		var readers sync.WaitGroup
		readers.Add(1)
		t.Cleanup(func() { close(stop); _ = conn.Close(); readers.Wait() })
		go func() {
			defer readers.Done()
			defer close(stream)
			for {
				var event realtime.Envelope
				if conn.ReadJSON(&event) != nil {
					return
				}
				select {
				case stream <- event:
				case <-stop:
					return
				}
			}
		}()
		return stream
	}
	owner1, owner2, outsider := connect(0, f.ownerToken), connect(1, f.ownerToken), connect(1, f.memberToken)
	wait := func(stream <-chan realtime.Envelope, kind, id string) {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event, ok := <-stream:
				if !ok {
					t.Fatal("owner socket closed")
				}
				if event.Type != kind {
					continue
				}
				var data map[string]string
				if err := json.Unmarshal(event.Data, &data); err != nil {
					t.Fatal(err)
				}
				want := 1
				if id == "" {
					want = 0
				}
				if len(data) != want || (id != "" && data["folderId"] != id) {
					t.Fatalf("folder event leaked metadata or item identifiers: %#v", data)
				}
				return
			case <-timer.C:
				t.Fatal("missing " + kind)
			}
		}
	}
	var created struct{ Item folderdomain.Folder }
	api.expect(t, "POST", "/folders", f.ownerToken, map[string]string{"name": "Приватная организация"}, 201, &created)
	path := "/folders/" + created.Item.ID
	for _, stream := range []<-chan realtime.Envelope{owner1, owner2} {
		wait(stream, "folder.created", created.Item.ID)
	}
	// Events follow persistence: the second API instance can read the folder immediately.
	if _, err := pg.NewFolderRepository(f.db).Get(ctx, f.owner.ID, created.Item.ID); err != nil {
		t.Fatal("event before persistence", err)
	}
	api.expect(t, "PATCH", path, f.ownerToken, map[string]string{"name": "Переименована"}, 200, nil)
	for _, stream := range []<-chan realtime.Envelope{owner1, owner2} {
		wait(stream, "folder.updated", created.Item.ID)
	}
	api.expect(t, "PUT", "/folders/order", f.ownerToken, map[string]any{"ids": []string{created.Item.ID}}, 200, nil)
	for _, stream := range []<-chan realtime.Envelope{owner1, owner2} {
		wait(stream, "folder.reordered", "")
	}
	direct, _, err := pg.NewPersonalRepository(f.db).GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	api.expect(t, "PUT", path+"/items/conversation/"+direct.ID, f.ownerToken, nil, 200, nil)
	for _, stream := range []<-chan realtime.Envelope{owner1, owner2} {
		wait(stream, "folder.items.updated", created.Item.ID)
	}
	api.expect(t, "DELETE", path, f.ownerToken, nil, 200, nil)
	for _, stream := range []<-chan realtime.Envelope{owner1, owner2} {
		wait(stream, "folder.deleted", created.Item.ID)
	}
	timer := time.NewTimer(350 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event, ok := <-outsider:
			if !ok {
				t.Fatal("outsider socket closed")
			}
			if strings.HasPrefix(event.Type, "folder.") {
				t.Fatalf("personal organization sent to another user: %s", event.Type)
			}
		case <-timer.C:
			return
		}
	}
}
