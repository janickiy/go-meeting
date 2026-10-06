package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
)

func TestGroupRealtimeAcrossInstancesAndRemoval(t *testing.T) {
	if os.Getenv("RECORDER_DIRECT_E2E") != "true" {
		t.Skip("real Redis/PostgreSQL/MinIO required")
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
	third := users.User{ID: uuid.NewString(), Email: "third@group-realtime.example", PasswordHash: f.owner.PasswordHash, DisplayName: ptr("Charlie")}
	if _, err := pg.NewUserRepository(f.db).Create(ctx, third); err != nil {
		t.Fatal(err)
	}
	thirdToken, _ := f.tokens.Issue(third.ID)
	servers := []*httptest.Server{}
	handlers := []*wstransport.UserHandler{}
	var api stageOneAPI
	for i := 0; i < 2; i++ {
		r := gin.New()
		r.Use(gin.Recovery())
		httptransport.RegisterPersonalRoutes(r, &personalapp.Handler{Repo: repo, Events: events}, chatapp.NewHandler(service).ForConversations(), middleware.Authenticate(f.tokens), nil)
		cfg := f.config
		cfg.PingInterval = time.Second
		cfg.PongTimeout = 3 * time.Second
		cfg.SessionTTL = 5 * time.Second
		h := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":global"), nil, cfg, bus, redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL), repo)
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
		stream := make(chan realtime.Envelope, 128)
		stop := make(chan struct{})
		t.Cleanup(func() { close(stop); _ = conn.Close() })
		go func() {
			defer close(stream)
			for {
				var e realtime.Envelope
				if conn.ReadJSON(&e) != nil {
					return
				}
				select {
				case stream <- e:
				case <-stop:
					return
				}
			}
		}()
		return stream
	}
	wait := func(stream <-chan realtime.Envelope, kind, id string) realtime.Envelope {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case e, ok := <-stream:
				if !ok {
					t.Fatal("global WS closed")
				}
				if e.Type == kind && bytes.Contains(e.Data, []byte(id)) {
					return e
				}
			case <-timer.C:
				t.Fatal("missing event " + kind)
			}
		}
	}
	bob1, bob2, thirdWS := connect(0, f.memberToken), connect(1, f.memberToken), connect(1, thirdToken)
	var group struct{ Item personal.Conversation }
	api.expect(t, "POST", "/conversations/group", f.ownerToken, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Realtime group", MemberIDs: []string{f.member.ID}}, 201, &group)
	wait(bob1, "conversation.member.added", group.Item.ID)
	wait(bob2, "conversation.member.added", group.Item.ID)
	path := "/conversations/" + group.Item.ID
	api.expect(t, "POST", path+"/members", f.ownerToken, map[string]any{"userIds": []string{third.ID}}, 200, nil)
	wait(thirdWS, "conversation.member.added", third.ID)
	var sent struct{ Item chat.Message }
	api.expect(t, "POST", path+"/messages", f.ownerToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Committed across instances"}, 201, &sent)
	for _, stream := range []<-chan realtime.Envelope{bob1, bob2, thirdWS} {
		e := wait(stream, "message.created", sent.Item.ID)
		var data struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(e.Data, &data)
		if data.Type != "group" {
			t.Fatal("group event has direct type")
		}
	}
	history, err := pg.NewDirectChatRepository(f.db).List(ctx, third.ID, group.Item.ID, "", 50)
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != sent.Item.ID {
		t.Fatal("published before persistence", err)
	}
	api.expect(t, "DELETE", path+"/members/"+third.ID, f.ownerToken, nil, 200, nil)
	wait(thirdWS, "conversation.member.removed", third.ID)
	api.expect(t, "GET", path+"/messages", thirdToken, nil, 403, nil)
	// Publish a delayed pre-removal payload to that user's Redis channel. Delivery
	// must recheck PostgreSQL rather than trust the previously chosen recipient.
	if err := bus.Publish(ctx, third.ID, realtime.Event("message.created", "", map[string]any{"conversationId": group.Item.ID, "type": "group", "message": sent.Item})); err != nil {
		t.Fatal(err)
	}
	api.expect(t, "POST", path+"/messages", f.ownerToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "After removal"}, 201, &sent)
	wait(bob1, "message.created", sent.Item.ID)
	timer := time.NewTimer(350 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-thirdWS:
			if !ok {
				t.Fatal("revoke unnecessarily closed account WS")
			}
			if e.Type == "message.created" {
				t.Fatal("removed user received private message")
			}
		case <-timer.C:
			return
		}
	}
}
