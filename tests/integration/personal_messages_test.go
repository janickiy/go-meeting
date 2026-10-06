package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
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
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPersonalPairAndConstraints(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	var ids [24]string
	var failures [24]error
	runConcurrent(24, func(i int) {
		a, b := f.owner.ID, f.member.ID
		if i%2 == 1 {
			a, b = b, a
		}
		item, _, err := repo.GetOrCreate(ctx, a, b)
		ids[i] = item.ID
		failures[i] = err
	})
	for i := range ids {
		if failures[i] != nil {
			t.Fatal(failures[i])
		}
		if ids[i] != ids[0] {
			t.Fatal("pair raced into duplicate conversations")
		}
	}
	if _, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.owner.ID); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("self allowed: %v", err)
	}
	outsider := users.User{ID: uuid.NewString(), Email: "outside@example.test", PasswordHash: "unused", DisplayName: ptr("Outside")}
	if _, err := pg.NewUserRepository(f.db).Create(ctx, outsider); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, outsider.ID, ids[0]); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("outsider read conversation")
	}
	if err := f.db.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?)", ids[0], outsider.ID).Error; err == nil {
		t.Fatal("DB accepted third member")
	}
	if err := f.db.Exec("DELETE FROM conversation_members WHERE conversation_id=? AND user_id=?", ids[0], f.member.ID).Error; err == nil {
		t.Fatal("DB accepted incomplete pair")
	}
	scope := f.conference.ID
	guest := users.User{ID: uuid.NewString(), Email: "guest@example.test", PasswordHash: "unused", GuestConferenceID: &scope, DisplayName: ptr("Guest")}
	if _, err := pg.NewUserRepository(f.db).Create(ctx, guest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GetOrCreate(ctx, f.owner.ID, guest.ID); err == nil {
		t.Fatal("guest conversation allowed")
	}
	peers, err := repo.Search(ctx, f.owner.ID, "Bo")
	if err != nil || len(peers) != 1 || peers[0].ID != f.member.ID {
		t.Fatalf("search: %+v %v", peers, err)
	}
	data, _ := json.Marshal(peers)
	if bytes.Contains(data, []byte("email")) || bytes.Contains(data, []byte("password")) {
		t.Fatal("private search data leaked")
	}
	if _, err := repo.Search(ctx, f.owner.ID, "%"); err == nil {
		t.Fatal("short enumeration accepted")
	}
	peers, err = repo.Search(ctx, f.owner.ID, "%%")
	if err != nil || len(peers) != 0 {
		t.Fatal("wildcards enumerated directory")
	}
}

func TestPersonalMessagesRealtimeAndFiles(t *testing.T) {
	if os.Getenv("RECORDER_DIRECT_E2E") != "true" {
		t.Skip("RECORDER_DIRECT_E2E=true enables real MinIO + Redis + PostgreSQL")
	}
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	events := &personalusecase.Events{Members: repo, Bus: bus}
	endpoint, access, secret := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	if access == "" {
		access = "go_recorder"
	}
	if secret == "" {
		secret = "go_recorder_pass"
	}
	storage, err := s3storage.NewClient(ctx, endpoint, access, secret, "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetPublicEndpoint(endpoint)
	service, err := chatusecase.NewService(ctx, messages, storage, events)
	if err != nil {
		t.Fatal(err)
	}
	item, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.RemovePrefix(ctx, "attachments/direct/"+item.ID+"/") })
	presence := redisinfra.NewUserPresence(f.redis, f.config.Namespace, time.Second)
	var api stageOneAPI
	var servers []*httptest.Server
	var handlers []*wstransport.UserHandler
	for i := 0; i < 2; i++ {
		router := gin.New()
		router.Use(gin.Recovery())
		httptransport.RegisterPersonalRoutes(router, &personalapp.Handler{Repo: repo, Events: events}, chatapp.NewHandler(service).ForConversations(), middleware.Authenticate(f.tokens), redisinfra.NewRateLimiter(f.redis))
		h := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, f.config.Namespace+":user-ws"), redisinfra.NewRateLimiter(f.redis), f.config, bus, presence, repo)
		h.RegisterRoutes(router)
		handlers = append(handlers, h)
		servers = append(servers, httptest.NewServer(router))
		if i == 0 {
			api = stageOneAPI{router: router}
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
	path := "/conversations/" + item.ID
	api.expect(t, "GET", path, "", nil, 401, nil)
	api.expect(t, "POST", "/conversations/direct", f.ownerToken, map[string]any{"userId": f.member.ID}, 200, nil)
	outsider := users.User{ID: uuid.NewString(), Email: "other@example.test", PasswordHash: "unused", DisplayName: ptr("Other")}
	if _, err = pg.NewUserRepository(f.db).Create(ctx, outsider); err != nil {
		t.Fatal(err)
	}
	outsideToken, _ := f.tokens.Issue(outsider.ID)
	api.expect(t, "GET", path, outsideToken, nil, 403, nil)
	api.expect(t, "POST", path+"/messages", outsideToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "forged"}, 403, nil)
	guest := users.User{ID: uuid.NewString(), Email: "direct-guest@example.test", PasswordHash: "unused", GuestConferenceID: &f.conference.ID}
	if _, err = pg.NewUserRepository(f.db).Create(ctx, guest); err != nil {
		t.Fatal(err)
	}
	guestToken, err := f.tokens.IssueGuest(guest.ID, f.conference.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/conversations", path, path + "/messages", "/users?search=Bo"} {
		api.expect(t, "GET", endpoint, guestToken, nil, 403, nil)
	}
	api.expect(t, "POST", "/conversations/direct", guestToken, map[string]any{"userId": f.owner.ID}, 403, nil)
	api.expect(t, "POST", path+"/attachments/init", guestToken, nil, 403, nil)
	api.expect(t, "POST", "/ws-ticket", guestToken, nil, 403, nil)
	// The receiver's sockets live on different API instances; the sender does not join a conference WS.
	connect := func(index int, token string) (*ws.Conn, <-chan realtime.Envelope, string) {
		request, _ := http.NewRequest("POST", servers[index].URL+"/api/v1/ws-ticket", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var ticket struct{ Ticket string }
		if err = json.NewDecoder(response.Body).Decode(&ticket); err != nil || response.StatusCode != 201 {
			t.Fatalf("ticket %d %v", response.StatusCode, err)
		}
		address := strings.Replace(servers[index].URL, "http:", "ws:", 1) + "/api/v1/ws?ticket=" + ticket.Ticket
		conn, _, err := ws.DefaultDialer.Dial(address, nil)
		if err != nil {
			t.Fatal(err)
		}
		stream := make(chan realtime.Envelope, 128)
		go func() {
			defer close(stream)
			for {
				var event realtime.Envelope
				if conn.ReadJSON(&event) != nil {
					return
				}
				stream <- event
			}
		}()
		t.Cleanup(func() { _ = conn.Close() })
		return conn, stream, address
	}
	wait := func(stream <-chan realtime.Envelope, kind, id string) {
		deadline := time.NewTimer(4 * time.Second)
		defer deadline.Stop()
		for {
			select {
			case event, ok := <-stream:
				if !ok {
					t.Fatal("global socket closed")
				}
				if event.Type == kind && (id == "" || bytes.Contains(event.Data, []byte(id))) {
					return
				}
			case <-deadline.C:
				t.Fatal("missing global event " + kind)
			}
		}
	}
	firstSocket, stream, address := connect(0, f.memberToken)
	secondSocket, _, _ := connect(1, f.memberToken)
	if c, resp, err := ws.DefaultDialer.Dial(address, nil); err == nil {
		_ = c.Close()
		t.Fatal("ticket reused")
	} else if resp == nil || resp.StatusCode != 401 {
		t.Fatal("wrong ticket reuse rejection")
	}
	badOrigin := http.Header{"Origin": []string{"https://evil.example"}}
	if c, resp, err := ws.DefaultDialer.Dial(address, badOrigin); err == nil {
		_ = c.Close()
		t.Fatal("origin allowed")
	} else if resp == nil || resp.StatusCode != 403 {
		t.Fatal("wrong origin rejection")
	}
	var result struct{ Item chat.Message }
	request := chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Hello offline/online"}
	api.expect(t, "POST", path+"/messages", f.ownerToken, request, 201, &result)
	first := result.Item
	wait(stream, "message.created", first.ID)
	// Receiving the event proves that an independent PostgreSQL read sees committed content.
	history, err := messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != first.ID {
		t.Fatal("event before commit/history mismatch")
	}
	var retryIDs [12]string
	var failures [12]error
	runConcurrent(12, func(i int) {
		m, _, err := service.Send(ctx, f.owner.ID, item.ID, request)
		retryIDs[i] = m.ID
		failures[i] = err
	})
	for i := range retryIDs {
		if failures[i] != nil || retryIDs[i] != first.ID {
			t.Fatalf("duplicate retry %v", failures[i])
		}
	}
	request.Text = "collision"
	api.expect(t, "POST", path+"/messages", f.ownerToken, request, 409, nil)
	api.expect(t, "POST", path+"/messages", f.memberToken, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "reply", ReplyTo: first.ID}, 201, &result)
	reply := result.Item
	if reply.ReplyPreview == nil || reply.ReplyPreview.ID != first.ID {
		t.Fatal("reply preview")
	}
	other, _, err := repo.GetOrCreate(ctx, f.owner.ID, outsider.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Send(ctx, f.owner.ID, other.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "cross", ReplyTo: first.ID}); err == nil {
		t.Fatal("cross conversation reply")
	}
	api.expect(t, "PATCH", path+"/messages/"+first.ID, f.memberToken, chat.EditRequest{Text: "forged"}, 403, nil)
	api.expect(t, "DELETE", path+"/messages/"+first.ID, f.memberToken, nil, 403, nil)
	api.expect(t, "PATCH", path+"/messages/"+first.ID, f.ownerToken, chat.EditRequest{Text: "edited"}, 200, &result)
	wait(stream, "message.updated", first.ID)
	var readFailures [20]error
	runConcurrent(20, func(i int) {
		id := first.ID
		if i%2 == 0 {
			id = reply.ID
		}
		_, err := messages.MarkRead(ctx, f.member.ID, item.ID, id)
		if err != nil {
			readFailures[i] = err
		}
	})
	for _, err := range readFailures {
		if err != nil {
			t.Fatal(err)
		}
	}
	state, err := messages.ReadState(ctx, f.member.ID, item.ID)
	if err != nil || state.LastReadMessageID == nil || *state.LastReadMessageID != reply.ID || state.UnreadCount != 0 {
		t.Fatalf("nonmonotonic read: %+v %v", state, err)
	}
	page, err := repo.List(ctx, f.member.ID, "", 1)
	if err != nil || len(page.Items) != 1 || page.UnreadCount != 0 {
		t.Fatalf("conversation projection %+v %v", page, err)
	}
	// Real private object upload, repeat finalize, scoped download and attachment IDOR.
	a, _, err := service.InitAttachment(ctx, f.owner.ID, item.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "note.txt", MimeType: "text/plain", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	a, err = service.Upload(ctx, f.owner.ID, item.ID, a.ID, strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		a, err = service.FinalizeAttachment(ctx, f.owner.ID, item.ID, a.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err = service.Send(ctx, f.owner.ID, other.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "IDOR", AttachmentIDs: []string{a.ID}}); err == nil {
		t.Fatal("cross attachment attached")
	}
	if _, _, err = service.Download(ctx, outsider.ID, item.ID, a.ID); err == nil {
		t.Fatal("outsider download")
	}
	fileMessage, _, err := service.Send(ctx, f.owner.ID, item.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), AttachmentIDs: []string{a.ID}})
	if err != nil || len(fileMessage.Attachments) != 1 {
		t.Fatalf("file send: %+v %v", fileMessage, err)
	}
	url, _, err := service.Download(ctx, f.member.ID, item.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	download, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	_ = download.Body.Close()
	if download.StatusCode != 200 {
		t.Fatal("signed download")
	}
	// Concurrent delete/edit is serialized by conversation lock; deletion cannot be undone.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = service.Edit(ctx, f.owner.ID, item.ID, first.ID, chat.EditRequest{Text: "race"})
	}()
	go func() { defer wg.Done(); _, _ = service.Delete(ctx, f.owner.ID, item.ID, first.ID) }()
	wg.Wait()
	history, err = messages.List(ctx, f.member.ID, item.ID, "", 1)
	if err != nil || history.NextCursor == "" {
		t.Fatal("history pagination")
	}
	older, err := messages.List(ctx, f.member.ID, item.ID, history.NextCursor, 100)
	if err != nil || len(older.Items) != 2 || older.Items[0].DeletedAt == nil {
		t.Fatal("edit resurrected deleted message")
	}
	_ = firstSocket.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := presence.Count(ctx, f.member.ID)
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n, _ := presence.Count(ctx, f.member.ID); n != 1 {
		t.Fatalf("second tab lost online: %d", n)
	}
	_ = secondSocket.Close()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := presence.Count(ctx, f.member.ID)
		if n == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if n, _ := presence.Count(ctx, f.member.ID); n != 0 {
		t.Fatal("last tab left presence")
	}
	offline, _, err := service.Send(ctx, f.owner.ID, item.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "offline"})
	if err != nil {
		t.Fatal(err)
	}
	history, err = messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || history.Items[len(history.Items)-1].ID != offline.ID {
		t.Fatal("offline message lost")
	}
	// Cross-instance user-only routing: a third account receives no private messages.
	third, err := bus.Subscribe(ctx, outsider.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if _, _, err = service.Send(ctx, f.owner.ID, item.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "private"}); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err = third.ReceiveMessage(short); err == nil {
		t.Fatal("global broadcast leaked DM")
	}
	var summary personal.Page
	api.expect(t, "GET", "/conversations", f.memberToken, nil, 200, &summary)
	if summary.UnreadCount != 3 {
		t.Fatalf("unread expected 3 got %d", summary.UnreadCount)
	}
	// New directory/ticket endpoints use shared Redis rate limits across both APIs.
	for i := 0; i < 30; i++ {
		api.expect(t, "GET", "/users?search=Bo", f.ownerToken, nil, 200, nil)
		api.expect(t, "POST", "/ws-ticket", outsideToken, nil, 201, nil)
	}
	api.expect(t, "GET", "/users?search=Bo", f.ownerToken, nil, 429, nil)
	ticketRequest, _ := http.NewRequest("POST", servers[1].URL+"/api/v1/ws-ticket", nil)
	ticketRequest.Header.Set("Authorization", "Bearer "+outsideToken)
	response, err := http.DefaultClient.Do(ticketRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 429 {
		t.Fatal("cross-instance ticket rate limit bypassed")
	}
}

func TestPersonalMigrationPreservesConferenceChat(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewChatRepository(f.db)
	request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Existing conference message"})
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := repo.Send(ctx, f.owner.ID, f.conference.ID, request, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.MarkRead(ctx, f.member.ID, f.conference.ID, message.ID); err != nil {
		t.Fatal(err)
	}
	a, _, err := repo.InitAttachment(ctx, f.owner.ID, f.conference.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "legacy.txt", MimeType: "text/plain", Size: 5})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var result string
		if err := f.db.Raw(`SELECT json_build_object('messages',(SELECT json_agg(x) FROM chat_messages x),'attachments',(SELECT json_agg(x) FROM chat_attachments x),'read',(SELECT json_agg(x) FROM chat_read_states x))::text`).Scan(&result).Error; err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	down, err := os.ReadFile(filepath.Join("..", "..", "database", "migrations", "000028_personal_conversations.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(down)).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("DELETE FROM release_schema_migrations WHERE name='000028_personal_conversations.up.sql'").Error; err != nil {
		t.Fatal(err)
	}
	if err = pg.RunMigrations(f.db, filepath.Join("..", "..", "database", "migrations")); err != nil {
		t.Fatal(err)
	}
	if snapshot() != before {
		t.Fatal("migration altered existing conference data")
	}
	if a.Prefix() != "attachments/"+f.conference.ID+"/"+a.ID+"/" {
		t.Fatal("legacy object prefix changed")
	}
}

func TestPersonalGlobalSessionLifecycle(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	auth, err := authusecase.NewService(pg.NewUserRepository(f.db), security.PasswordHasher{}, f.tokens)
	if err != nil {
		t.Fatal(err)
	}
	auth.WithSessions(pg.NewAuthSessionRepository(f.db))
	login, err := auth.Login(ctx, users.LoginRequest{Email: f.owner.Email, Password: stageOneTestPassword})
	if err != nil {
		t.Fatal(err)
	}
	_, _, sid, _, err := f.tokens.VerifyAuthorization(login.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(2 * time.Second)
	short, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": f.owner.ID, "iss": "go-recorder", "aud": "go-recorder-api", "sid": sid, "iat": time.Now().Unix(), "exp": expires.Unix()}).SignedString([]byte(stageTwoSecret))
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.config
	cfg.PingInterval = time.Second
	cfg.PongTimeout = 3 * time.Second
	cfg.SessionTTL = 5 * time.Second
	bus := redisinfra.NewNotificationBus(f.redis, cfg.Namespace)
	presence := redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL)
	h := wstransport.NewUserHandler(auth, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":user-ws"), nil, cfg, bus, presence, repo)
	defer h.Shutdown()
	router := gin.New()
	h.RegisterRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()
	req, _ := http.NewRequest("POST", server.URL+"/api/v1/ws-ticket", nil)
	req.Header.Set("Authorization", "Bearer "+short)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var ticket struct{ Ticket string }
	_ = json.NewDecoder(response.Body).Decode(&ticket)
	_ = response.Body.Close()
	if response.StatusCode != 201 {
		t.Fatalf("ticket %d", response.StatusCode)
	}
	conn, _, err := ws.DefaultDialer.Dial(strings.Replace(server.URL, "http:", "ws:", 1)+"/api/v1/ws?ticket="+ticket.Ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	events := make(chan realtime.Envelope, 8)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			var event realtime.Envelope
			if conn.ReadJSON(&event) != nil {
				return
			}
			events <- event
		}
	}()
	// Cross the JWT expiry and a server authorization heartbeat. The durable SID keeps the socket live.
	time.Sleep(6 * time.Second)
	if err = bus.Publish(ctx, f.owner.ID, realtime.Event("notification.created", "", map[string]string{"id": "after-expiry"})); err != nil {
		t.Fatal(err)
	}
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	received := false
	for !received {
		select {
		case event := <-events:
			received = event.Type == "notification.created"
		case <-closed:
			t.Fatal("JWT expiry disconnected persistent session")
		case <-timeout.C:
			t.Fatal("notification not delivered on global stream")
		}
	}
	if err = auth.Logout(ctx, login.SessionToken, login.AccessToken); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	case <-time.After(6 * time.Second):
		t.Fatal("logout did not revoke global socket")
	}
	h.Shutdown()
	if h.LocalCount() != 0 {
		t.Fatal("socket reservation leaked")
	}
	if n, _ := presence.Count(ctx, f.owner.ID); n != 0 {
		t.Fatal("presence leaked after logout")
	}
	// An abandoned physical lease expires without disconnect or application memory.
	abandoned := uuid.NewString()
	lease := redisinfra.NewUserPresence(f.redis, cfg.Namespace, 100*time.Millisecond)
	if _, err = lease.Touch(ctx, f.owner.ID, abandoned); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if n, _ := lease.Count(ctx, f.owner.ID); n != 0 {
		t.Fatal("dead lease remains online")
	}
}

func TestPersonalDirectoryAndProjection(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	for i := 0; i < 25; i++ {
		u := users.User{ID: uuid.NewString(), Email: fmt.Sprintf("peer%d@directory.example", i), PasswordHash: "unused", DisplayName: ptr(fmt.Sprintf("Directory Peer %02d", i))}
		if _, err := pg.NewUserRepository(f.db).Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		item, _, err := repo.GetOrCreate(ctx, f.owner.ID, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.db.Exec(`INSERT INTO conversation_messages(id,conversation_id,sender_user_id,client_request_id,request_fingerprint,text) SELECT gen_random_uuid(),?,?,gen_random_uuid(),repeat('0',64),'sample message' FROM generate_series(1,100)`, item.ID, u.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err = f.db.Exec(`UPDATE conversations SET last_message_id=(SELECT id FROM conversation_messages WHERE conversation_id=? ORDER BY sequence DESC LIMIT 1),last_message_at=clock_timestamp() WHERE id=?`, item.ID, item.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	peers, err := repo.Search(ctx, f.owner.ID, "Directory")
	if err != nil || len(peers) != 20 {
		t.Fatalf("directory limit %+v %v", peers, err)
	}
	if peers, err = repo.Search(ctx, f.owner.ID, "@directory.example"); err != nil || len(peers) != 0 {
		t.Fatal("email search exposed accounts")
	}
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := repo.List(ctx, f.owner.ID, cursor, 7)
		if err != nil {
			t.Fatal(err)
		}
		if page.UnreadCount != 2500 {
			t.Fatal("aggregate unread mismatch")
		}
		for _, c := range page.Items {
			if seen[c.ID] || c.UnreadCount != 100 || c.Preview != "sample message" {
				t.Fatalf("projection mismatch %+v", c)
			}
			seen[c.ID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	if len(seen) != 25 {
		t.Fatal("conversation cursor skipped rows")
	}
	if err = f.db.Exec("ANALYZE conversation_messages; ANALYZE conversation_members; ANALYZE conversations").Error; err != nil {
		t.Fatal(err)
	}
	var plan []struct {
		QueryPlan string `gorm:"column:QUERY PLAN"`
	}
	sql := `EXPLAIN (ANALYZE,BUFFERS) SELECT c.id,u.display_name,(SELECT count(*) FROM conversation_messages x WHERE x.conversation_id=c.id AND x.sequence>m.last_read_sequence AND x.sender_user_id<>m.user_id AND x.deleted_at IS NULL) AS unread FROM conversation_members m JOIN conversations c ON c.id=m.conversation_id JOIN users u ON u.id=CASE WHEN c.user_low_id=m.user_id THEN c.user_high_id ELSE c.user_low_id END LEFT JOIN conversation_messages lm ON lm.id=c.last_message_id WHERE m.user_id=? ORDER BY COALESCE(c.last_message_at,c.created_at) DESC,c.id DESC LIMIT 8`
	if err = f.db.Raw(sql, f.owner.ID).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range plan {
		t.Log(row.QueryPlan)
	}
}
