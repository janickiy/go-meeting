package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
)

// Existing chat-action tests isolate PostgreSQL; they do not assert live presence.
type conferenceChatOfflinePresence struct{}

func (conferenceChatOfflinePresence) Online(_ context.Context, ids []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (conferenceChatOfflinePresence) GetActiveSessions(context.Context, string) ([]realtime.Session, error) {
	return nil, nil
}

func TestConferenceChatMembersLivePresenceAndAccess(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewChatRepository(f.db)
	presence := redisinfra.NewUserPresence(f.redis, f.config.Namespace, f.config.SessionTTL)
	actions := chatusecase.NewActionService(repo, nil).WithPresence(presence, f.hubs[0])
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	global := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, f.config.Namespace+":user-ws"), nil, f.config, bus, presence, pg.NewPersonalRepository(f.db))
	router := gin.New()
	httptransport.RegisterChatActionRoutes(router, &chatapp.ActionHandler{Service: actions}, middleware.Authenticate(f.tokens), nil)
	global.RegisterRoutes(router)
	server := httptest.NewServer(router)
	t.Cleanup(func() { global.Shutdown(); server.Close() })
	// A separate handler and presence adapter model another API replica.
	replica := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, f.config.Namespace+":user-ws"), nil, f.config, bus,
		redisinfra.NewUserPresence(f.redis, f.config.Namespace, f.config.SessionTTL), pg.NewPersonalRepository(f.db))
	replicaRouter := gin.New()
	replica.RegisterRoutes(replicaRouter)
	replicaServer := httptest.NewServer(replicaRouter)
	t.Cleanup(func() { replica.Shutdown(); replicaServer.Close() })
	api := stageOneAPI{router: router}
	path := "/conferences/" + f.conference.ID + "/chat/members"
	outsider := users.User{ID: uuid.NewString(), Email: "presence-outside@example.test", PasswordHash: "test"}
	guest := users.User{ID: uuid.NewString(), Email: "presence-guest@guest.invalid", PasswordHash: "test", GuestConferenceID: &f.conference.ID}
	for _, user := range []users.User{outsider, guest} {
		if _, err := pg.NewUserRepository(f.db).Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.Exec(`INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,joined_at,admission_state)
		VALUES(gen_random_uuid(),?::uuid,?::uuid,'Guest','participant','joined',now(),'admitted')`, f.conference.ID, guest.ID).Error; err != nil {
		t.Fatal(err)
	}
	outsideToken, _ := f.tokens.Issue(outsider.ID)
	guestToken, _ := f.tokens.IssueGuest(guest.ID, f.conference.ID)
	api.expect(t, "GET", path, "", nil, 401, nil)
	api.expect(t, "GET", path, outsideToken, nil, 403, nil)
	api.expect(t, "GET", path+"?limit=101", f.ownerToken, nil, 422, nil)
	api.expect(t, "POST", "/ws-ticket", guestToken, nil, 403, nil)

	members := func() map[string]chat.MemberView {
		t.Helper()
		var response struct{ Items []chat.MemberView }
		result := api.expect(t, "GET", path+"?limit=100", f.ownerToken, nil, 200, &response)
		if result.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("presence response is cacheable")
		}
		items := map[string]chat.MemberView{}
		for _, item := range response.Items {
			if item.UserID != nil {
				items[*item.UserID] = item
			}
		}
		return items
	}
	assertEventually := func(user string, online bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if item, ok := members()[user]; ok && item.Online != nil && *item.Online == online {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("user %s did not reach online=%v", user, online)
	}
	initial := members()
	if len(initial) != 3 || initial[f.owner.ID].Online == nil || *initial[f.owner.ID].Online || initial[f.member.ID].Online == nil || *initial[f.member.ID].Online || !initial[guest.ID].IsGuest || initial[guest.ID].Online == nil || *initial[guest.ID].Online || initial[f.member.ID].IsGuest {
		t.Fatalf("initial membership is not live presence: %+v", initial)
	}
	connectGlobal := func(target *httptest.Server) *ws.Conn {
		t.Helper()
		request, _ := http.NewRequest("POST", target.URL+"/api/v1/ws-ticket", nil)
		request.Header.Set("Authorization", "Bearer "+f.memberToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var ticket struct{ Ticket string }
		err = json.NewDecoder(response.Body).Decode(&ticket)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != 201 {
			t.Fatalf("global ticket: %d %v", response.StatusCode, err)
		}
		connection, _, err := ws.DefaultDialer.Dial(strings.Replace(target.URL, "http:", "ws:", 1)+"/api/v1/ws?ticket="+ticket.Ticket, nil)
		if err != nil {
			t.Fatal(err)
		}
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := connection.ReadMessage(); err != nil {
					return
				}
			}
		}()
		t.Cleanup(func() { _ = connection.Close(); <-closed })
		return connection
	}
	first, second := connectGlobal(server), connectGlobal(replicaServer)
	assertEventually(f.member.ID, true)
	// Leaving a conference is independent of having a live account socket.
	if _, err := f.service.Leave(ctx, f.member.ID, f.conference.ID); err != nil {
		t.Fatal(err)
	}
	if item := members()[f.member.ID]; item.Status != conferences.Left || item.Online == nil || !*item.Online {
		t.Fatalf("conference leave changed global presence: %+v", item)
	}
	_ = first.Close()
	if count, err := presence.Count(ctx, f.member.ID); err != nil || count < 1 {
		t.Fatalf("closing one account tab removed another: %d %v", count, err)
	}
	assertEventually(f.member.ID, true)
	_ = second.Close()
	assertEventually(f.member.ID, false)
	// A crashed API leaves a lease behind; the TTL remains authoritative.
	abandoned := redisinfra.NewUserPresence(f.redis, f.config.Namespace, 120*time.Millisecond)
	if _, err := abandoned.Touch(ctx, f.member.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	assertEventually(f.member.ID, true)
	time.Sleep(160 * time.Millisecond)
	assertEventually(f.member.ID, false)

	// Guests have conference sockets, not global account sockets.
	guestFirst := f.connect(t, 0, guestToken, f.conference.ID)
	guestSecond := f.connect(t, 1, guestToken, f.conference.ID)
	assertEventually(guest.ID, true)
	_ = guestFirst.conn.Close()
	<-guestFirst.done
	assertEventually(guest.ID, true)
	_ = guestSecond.conn.Close()
	<-guestSecond.done
	assertEventually(guest.ID, false)
	if err := f.db.Exec("UPDATE conference_participants SET admission_state='kicked',admission_decided_at=now(),admission_version=admission_version+1 WHERE conference_id=? AND user_id=?", f.conference.ID, f.member.ID).Error; err != nil {
		t.Fatal(err)
	}
	api.expect(t, "GET", path, f.memberToken, nil, 403, nil)
	if _, exists := members()[f.member.ID]; exists {
		t.Fatal("removed participant remains in chat member projection")
	}
	// Redis failure cannot be mistaken for every participant being offline.
	if err := f.redis.Close(); err != nil {
		t.Fatal(err)
	}
	for _, item := range members() {
		if item.Online != nil {
			t.Fatal("Redis outage returned a false offline status")
		}
	}
	api.expect(t, "GET", path, outsideToken, nil, 403, nil)
}
