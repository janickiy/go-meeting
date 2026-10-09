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
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	goredis "github.com/redis/go-redis/v9"
)

// readPeerPresenceIntegration проверяет реальный защищённый GET без доступа к другим данным чата.
// @args t — тест; api — локальный router; path — переписка; token — токен тестового аккаунта;
// id и peer — ожидаемые серверные идентификаторы.
// @return подтверждённый bool присутствия из строго ограниченного JSON-ответа.
func readPeerPresenceIntegration(t *testing.T, api stageOneAPI, path, token, id, peer string) bool {
	t.Helper()
	var result struct {
		Status string                `json:"status"`
		Item   personal.PeerPresence `json:"item"`
	}
	response := api.expect(t, http.MethodGet, path, token, nil, http.StatusOK, &result)
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("статус присутствия не защищён от кэширования", response.Header())
	}
	var fields struct {
		Item map[string]json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Item.ConversationID != id || result.Item.PeerID != peer || len(fields.Item) != 3 || fields.Item["online"] == nil {
		t.Fatal("ответ раскрыл лишние поля или потерял привязку к собеседнику", response.Body.String())
	}
	return result.Item.Online
}

// connectPeerPresenceIntegration открывает настоящий учётный WebSocket изолированной API-реплики.
// Чтение сообщений обрабатывает ping/pong, а очистка закрывает только это соединение.
// @args t — тест; server — локальная API-реплика; token — токен тестового собеседника.
// @return установленное физическое соединение, автоматически закрываемое после теста.
func connectPeerPresenceIntegration(t *testing.T, server *httptest.Server, token string) *ws.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/v1/ws-ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	err = json.NewDecoder(response.Body).Decode(&ticket)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusCreated || ticket.Ticket == "" {
		t.Fatalf("учётный ticket не получен: HTTP %d, %v", response.StatusCode, err)
	}
	connection, _, err := ws.DefaultDialer.DialContext(ctx,
		strings.Replace(server.URL, "http:", "ws:", 1)+"/api/v1/ws?ticket="+ticket.Ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	// Читает события и автоматически отвечает на ping до закрытия тестовой вкладки.
	go func() {
		defer close(finished)
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}()
	// Освобождает только подключение этого теста, без воздействия на стенд пользователя.
	t.Cleanup(func() {
		_ = connection.Close()
		select {
		case <-finished:
		case <-time.After(3 * time.Second):
			t.Error("чтение закрытой тестовой вкладки не завершилось")
		}
	})
	return connection
}

// waitPeerPresenceLeaseCount ожидает изменения реальных физических сессий, не продлевая их.
// @args t — тест; presence — Redis-адаптер; user — собеседник; expected — число действующих вкладок.
func waitPeerPresenceLeaseCount(t *testing.T, presence *redisinfra.UserPresence, user string, expected int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		count, err := presence.Count(context.Background(), user)
		if err != nil {
			t.Fatal(err)
		}
		if count == expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("число действующих тестовых вкладок не стало равным %d", expected)
}

// TestPeerPresenceHTTPWithPostgresRedis проверяет ACL и присутствие через настоящие PG/Redis и две API-реплики.
// Использует отдельную UUID-базу stageOneDatabase и случайное Redis-пространство stageTwo;
// пользовательские контейнеры, базы, ключи и миграции стенда не используются.
// @args t — контекст изолированной интеграционной проверки личного статуса.
func TestPeerPresenceHTTPWithPostgresRedis(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	conversation, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.config
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = time.Second
	cfg.WriteTimeout = time.Second
	cfg.SessionTTL = config.PresenceTimeout
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	presence := redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL)
	bus := redisinfra.NewNotificationBus(f.redis, cfg.Namespace)
	handler := &personalapp.Handler{Repo: repo, Presence: presence}
	router := gin.New()
	httptransport.RegisterPersonalRoutes(router, handler, chatapp.NewHandler(nil).ForConversations(), middleware.Authenticate(f.tokens), nil)
	global := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":peer-status-ws"), nil, cfg, bus, presence, repo)
	global.RegisterRoutes(router)
	server := httptest.NewServer(router)
	// Завершает только API-обработчик и HTTP-сервер изолированного теста.
	t.Cleanup(func() { global.Shutdown(); server.Close() })
	replica := wstransport.NewUserHandler(f.tokens, redisinfra.NewRealtimeStore(f.redis, cfg.Namespace+":peer-status-ws"), nil, cfg, bus,
		redisinfra.NewUserPresence(f.redis, cfg.Namespace, cfg.SessionTTL), repo)
	replicaRouter := gin.New()
	replica.RegisterRoutes(replicaRouter)
	replicaServer := httptest.NewServer(replicaRouter)
	// Завершает вторую тестовую API-реплику после закрытия её физических соединений.
	t.Cleanup(func() { replica.Shutdown(); replicaServer.Close() })
	api := stageOneAPI{router: router}
	path := "/conversations/" + conversation.ID + "/peer-presence"

	outsider := users.User{ID: uuid.NewString(), Email: "peer-presence-outside@example.test", PasswordHash: "test"}
	guest := users.User{ID: uuid.NewString(), Email: "peer-presence-guest@guest.invalid", PasswordHash: "test", GuestConferenceID: &f.conference.ID}
	for _, user := range []users.User{outsider, guest} {
		if _, err := pg.NewUserRepository(f.db).Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	outsideToken, err := f.tokens.Issue(outsider.ID)
	if err != nil {
		t.Fatal(err)
	}
	guestToken, err := f.tokens.IssueGuest(guest.ID, f.conference.ID)
	if err != nil {
		t.Fatal(err)
	}
	api.expect(t, http.MethodGet, path, "", nil, 401, nil)
	api.expect(t, http.MethodGet, path, outsideToken, nil, 403, nil)
	api.expect(t, http.MethodGet, path, guestToken, nil, 403, nil)
	api.expect(t, http.MethodGet, "/conversations/"+uuid.NewString()+"/peer-presence", f.ownerToken, nil, 403, nil)
	api.expect(t, http.MethodGet, "/conversations/invalid/peer-presence", f.ownerToken, nil, 400, nil)
	if readPeerPresenceIntegration(t, api, path, f.ownerToken, conversation.ID, f.member.ID) {
		t.Fatal("одного членства в PostgreSQL недостаточно для статуса онлайн")
	}

	first := connectPeerPresenceIntegration(t, server, f.memberToken)
	second := connectPeerPresenceIntegration(t, replicaServer, f.memberToken)
	waitPeerPresenceLeaseCount(t, presence, f.member.ID, 2)
	if !readPeerPresenceIntegration(t, api, path+"?userId="+outsider.ID+"&targetUser="+outsider.ID,
		f.ownerToken, conversation.ID, f.member.ID) {
		t.Fatal("две действующие вкладки не подтверждены HTTP-статусом собеседника")
	}
	_ = first.Close()
	waitPeerPresenceLeaseCount(t, presence, f.member.ID, 1)
	if !readPeerPresenceIntegration(t, api, path, f.ownerToken, conversation.ID, f.member.ID) {
		t.Fatal("закрытие первой вкладки выключило присутствие второй")
	}
	_ = second.Close()
	waitPeerPresenceLeaseCount(t, presence, f.member.ID, 0)
	if readPeerPresenceIntegration(t, api, path, f.ownerToken, conversation.ID, f.member.ID) {
		t.Fatal("последняя закрытая вкладка осталась онлайн")
	}

	// Имитирует аварийный процесс: lease остался, но ни один pong больше не продлевает его.
	abandoned := uuid.NewString()
	if _, err := presence.Touch(ctx, f.member.ID, abandoned); err != nil {
		t.Fatal(err)
	}
	key := cfg.Namespace + ":user-presence:" + f.member.ID
	score, err := f.redis.ZScore(ctx, key, abandoned).Result()
	if err != nil {
		t.Fatal(err)
	}
	if !readPeerPresenceIntegration(t, api, path, f.ownerToken, conversation.ID, f.member.ID) {
		t.Fatal("свежая аварийная lease не отобразилась как действующая")
	}
	deadline := time.Now().Add(config.PresenceTimeout + time.Second)
	expired := false
	for time.Now().Before(deadline) {
		online := readPeerPresenceIntegration(t, api, path, f.ownerToken, conversation.ID, f.member.ID)
		current, scoreErr := f.redis.ZScore(ctx, key, abandoned).Result()
		if scoreErr != nil && scoreErr != goredis.Nil {
			t.Fatal(scoreErr)
		}
		if scoreErr == nil && current != score {
			t.Fatal("чтение HTTP-статуса продлило lease", score, current)
		}
		if !online {
			expired = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !expired {
		t.Fatal("аварийная вкладка пережила пятисекундный TTL из-за HTTP-чтения")
	}
	if exists, err := f.redis.Exists(ctx, key).Result(); err != nil || exists != 0 {
		t.Fatal("HTTP-чтение восстановило истёкший Redis-ключ", exists, err)
	}

	// Ошибка настоящего Redis не должна превращаться в подтверждённый офлайн.
	if err := f.redis.Set(ctx, key, "wrong type in isolated namespace", time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	response := api.expect(t, http.MethodGet, path, f.ownerToken, nil, 503, nil)
	if strings.Contains(response.Body.String(), "online") || strings.Contains(response.Body.String(), "wrong type") {
		t.Fatal("ошибка Redis стала статусом или раскрыла внутренние сведения", response.Body.String())
	}
	if err := f.redis.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	handler.Presence = nil
	response = api.expect(t, http.MethodGet, path, f.ownerToken, nil, 503, nil)
	if strings.Contains(response.Body.String(), "online") {
		t.Fatal("отсутствующий провайдер стал ложным офлайном", response.Body.String())
	}
	handler.Presence = presence

	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Peer presence group", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	api.expect(t, http.MethodGet, "/conversations/"+group.ID+"/peer-presence", f.ownerToken, nil, 403, nil)
	if _, err := repo.HideDirectConversation(ctx, f.owner.ID, conversation.ID); err != nil {
		t.Fatal(err)
	}
	api.expect(t, http.MethodGet, path, f.ownerToken, nil, 403, nil)
}
