package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	ws "github.com/gorilla/websocket"
	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const stageTwoSecret = "stage-two-isolated-integration-test-secret"

// stageTwoFixture хранит изолированное состояние тестового компонента «этап два тестовое окружение».
// @params:
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - redis: подключение Redis для распределённого состояния.
//   - store: значение store типа *redisinfra.RealtimeStore, используемое согласно назначению этой операции.
//   - hubs: набор значений hubs для последовательной или пакетной обработки.
//   - servers: набор значений servers для последовательной или пакетной обработки.
//   - tokens: сервис выпуска или проверки JWT авторизации.
//   - service: значение service типа *conferenceusecase.Service, используемое согласно назначению этой операции.
//   - config: настройки запуска и ограничений компонента.
//   - owner: значение owner типа users.User, используемое согласно назначению этой операции.
//   - member: значение member типа users.User, используемое согласно назначению этой операции.
//   - ownerToken: значение ownerToken типа string, используемое согласно назначению этой операции.
//   - memberToken: значение memberToken типа string, используемое согласно назначению этой операции.
//   - conference: конференция либо её идентификатор, ограничивающий область операции.
type stageTwoFixture struct {
	db                      *gorm.DB
	redis                   *goredis.Client
	store                   *redisinfra.RealtimeStore
	hubs                    []*realtimeusecase.Hub
	servers                 []*httptest.Server
	tokens                  *security.TokenService
	service                 *conferenceusecase.Service
	config                  config.RealtimeConfig
	owner, member           users.User
	ownerToken, memberToken string
	conference              conferences.View
}

// stageTwo подготавливает или проверяет часть тестового сценария «этап два».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//
// @return:
//   - результат 1 (*stageTwoFixture): значение, подготовленное операцией для вызывающей стороны.
func stageTwo(t *testing.T) *stageTwoFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	addr := os.Getenv("RECORDER_STAGE2_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RECORDER_STAGE2_TEST_REDIS_ADDR and RECORDER_STAGE1_TEST_POSTGRES_DSN for isolated realtime integration tests")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") && !strings.HasPrefix(addr, "localhost:") {
		t.Fatal("integration tests require local Redis")
	}
	f := &stageTwoFixture{db: stageOneDatabase(t)}
	f.redis = goredis.NewClient(&goredis.Options{Addr: addr})
	if err := f.redis.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	f.config = config.RealtimeConfig{PingInterval: 150 * time.Millisecond, PongTimeout: 150 * time.Millisecond, WriteTimeout: 300 * time.Millisecond, SessionTTL: time.Second, TicketTTL: 30 * time.Second, QueueSize: 64, MessageBytes: 65536, OutboundBytes: 262144, SDPBytes: 49152, ICEBytes: 4096, MessagesPerSecond: 100, Burst: 200, Namespace: "test:stage2:" + uuid.NewString(), ICE: domain.ICEConfig{ICEServers: []domain.ICEServer{}}}
	f.store = redisinfra.NewRealtimeStore(f.redis, f.config.Namespace)
	f.tokens, _ = security.NewTokenService(stageTwoSecret)
	ur := pg.NewUserRepository(f.db)
	hash, err := (security.PasswordHasher{}).Hash(stageOneTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	f.owner = users.User{ID: uuid.NewString(), Email: "owner@stage2.example", PasswordHash: hash, DisplayName: ptr("Alice")}
	f.member = users.User{ID: uuid.NewString(), Email: "member@stage2.example", PasswordHash: hash, DisplayName: ptr("Bob")}
	for _, user := range []users.User{f.owner, f.member} {
		if _, err := ur.Create(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	f.ownerToken, _ = f.tokens.Issue(f.owner.ID)
	f.memberToken, _ = f.tokens.Issue(f.member.ID)
	f.service = conferenceusecase.NewService(pg.NewConferenceRepository(f.db), ur, security.GenerateInviteCode)
	f.conference, err = f.service.Create(context.Background(), f.owner.ID, conferences.CreateRequest{Title: "Stage 2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Join(context.Background(), f.owner.ID, f.conference.ID, conferences.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Join(context.Background(), f.member.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Transition(context.Background(), f.owner.ID, f.conference.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for i := 0; i < 2; i++ {
		hub, err := realtimeusecase.NewHub(pg.NewSessionRepository(f.db), f.store, f.config.SessionTTL, logger)
		if err != nil {
			t.Fatal(err)
		}
		f.hubs = append(f.hubs, hub)
		svc := conferenceusecase.NewService(pg.NewConferenceRepository(f.db), ur, security.GenerateInviteCode)
		svc.SetObserver(hub)
		auth, _ := authusecase.NewService(ur, security.PasswordHasher{}, f.tokens)
		router := gin.New()
		router.Use(gin.Recovery())
		httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(auth), conferencesapp.NewHandler(svc), httpmiddleware.Authenticate(f.tokens))
		wstransport.NewHandler(hub, f.tokens, f.store, redisinfra.NewRateLimiter(f.redis), f.config).RegisterRoutes(router)
		f.servers = append(f.servers, httptest.NewServer(router))
	}
	f.service.SetObserver(f.hubs[0])
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			for _, hub := range f.hubs {
				hub.Shutdown()
			}
			for _, server := range f.servers {
				server.Close()
			}
			// Delete only this test's random namespace; never FLUSHDB or app keys.
			cleanup := goredis.NewClient(&goredis.Options{Addr: addr})
			defer cleanup.Close()
			keys, _ := cleanup.Keys(context.Background(), f.config.Namespace+":*").Result()
			if len(keys) > 0 {
				_ = cleanup.Del(context.Background(), keys...).Err()
			}
			_ = f.redis.Close()
		})
	return f
}

// ptr подготавливает или проверяет часть тестового сценария «ptr».
//
// @args
//   - s (string): значение s типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (*string): значение, подготовленное операцией для вызывающей стороны.
func ptr(s string) *string { return &s }

// testSocket хранит изолированное состояние тестового компонента «проверка Socket».
// @params:
//   - conn: действующее сетевое соединение операции.
//   - events: получатель или издатель событий прикладного сценария.
//   - done: канал уведомления о завершении ресурса.
//   - state: значение state типа domain.State, используемое согласно назначению этой операции.
type testSocket struct {
	conn   *ws.Conn
	events chan domain.Envelope
	done   chan struct{}
	state  domain.State
}

// connect подготавливает или проверяет часть тестового сценария «connect».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - instance (int): значение instance типа int, используемое согласно назначению этой операции.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (*testSocket): значение, подготовленное операцией для вызывающей стороны.
func (f *stageTwoFixture) connect(t *testing.T, instance int, token, conferenceID string) *testSocket {
	t.Helper()
	conn, response, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.servers[instance].URL, "http")+"/api/v1/conferences/"+conferenceID+"/ws", http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("connect: status %d, %v", status, err)
	}
	s := readSocket(conn)
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() { _ = conn.Close(); <-s.done })
	state := s.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == "conference.state" })
	if err = json.Unmarshal(state.Data, &s.state); err != nil {
		t.Fatal(err)
	}
	return s
}

// readSocket подготавливает или проверяет часть тестового сценария «чтение Socket».
//
// @args
//   - conn (*ws.Conn): действующее сетевое соединение операции.
//
// @return:
//   - результат 1 (*testSocket): значение, подготовленное операцией для вызывающей стороны.
func readSocket(conn *ws.Conn) *testSocket {
	s := &testSocket{conn: conn, events: make(chan domain.Envelope, 4096), done: make(chan struct{})}
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer close(s.done)
		for {
			var event domain.Envelope
			if err := conn.ReadJSON(&event); err != nil {
				return
			}
			select {
			case s.events <- event:
			default:
				return
			}
		}
	}()
	return s
}

// wait подготавливает или проверяет часть тестового сценария «ожидание».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - predicate (func(domain.Envelope) bool): условие выбора ожидаемого события в проверке.
//
// @return:
//   - результат 1 (domain.Envelope): значение, подготовленное операцией для вызывающей стороны.
func (s *testSocket) wait(t *testing.T, predicate func(domain.Envelope) bool) domain.Envelope {
	t.Helper()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case e := <-s.events:
			if predicate(e) {
				return e
			}
		case <-s.done:
			t.Fatal("socket closed before expected event")
		case <-timeout.C:
			t.Fatal("expected event timed out")
		}
	}
}

// signal подготавливает или проверяет часть тестового сценария «signal».
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - target (string): целевой объект, участник или состояние операции.
//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (s *testSocket) signal(t *testing.T, kind, target string, data any) string {
	t.Helper()
	raw, _ := json.Marshal(data)
	var signal map[string]any
	_ = json.Unmarshal(raw, &signal)
	signal["targetConnectionId"] = target
	event := domain.Event(kind, s.state.Participants[0].ConferenceID, signal)
	if err := s.conn.WriteJSON(event); err != nil {
		t.Fatal(err)
	}
	return event.ID
}

// presenceCount подготавливает или проверяет часть тестового сценария «присутствие количество».
//
// @args
//   - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - count (int): значение count типа int, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func presenceCount(e domain.Envelope, userID string, count int) bool {
	if e.Type != "participant.connected" && e.Type != "participant.disconnected" && e.Type != "conference.state" {
		return false
	}
	var state domain.State
	if json.Unmarshal(e.Data, &state) != nil {
		return false
	}
	for _, p := range state.Participants {
		if p.UserID != nil && *p.UserID == userID {
			return p.Connections == count && p.Online == (count > 0)
		}
	}
	return false
}

// TestStageTwoPresenceSignalingMultiInstance проверяет сценарий «этап два присутствие сигнализация Multi Instance», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoPresenceSignalingMultiInstance(t *testing.T) {
	f := stageTwo(t)
	a := f.connect(t, 0, f.ownerToken, f.conference.ID)
	b := f.connect(t, 1, f.memberToken, f.conference.ID)
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 1) })
	for _, step := range []struct {
		kind             string
		sender, receiver *testSocket
		payload          map[string]any
	}{
		{"webrtc.offer", a, b, map[string]any{"sdp": "opaque offer; not parsed by backend"}},
		{"webrtc.answer", b, a, map[string]any{"sdp": "opaque answer"}},
		{"webrtc.ice", a, b, map[string]any{"candidate": map[string]any{"candidate": "candidate:one", "sdpMid": "0"}}},
		{"webrtc.ice", b, a, map[string]any{"candidate": map[string]any{"candidate": "candidate:two", "sdpMLineIndex": 0}}},
	} {
		id := step.sender.signal(t, step.kind, step.receiver.state.ConnectionID, step.payload)
		e := step.receiver.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

			@return:
			  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == step.kind })
		var data domain.Signal
		_ = json.Unmarshal(e.Data, &data)
		if e.ReplyTo != id || data.SenderConnectionID != step.sender.state.ConnectionID || data.SenderParticipantID != step.sender.state.ParticipantID {
			t.Fatal("sender attribution/replyTo incorrect")
		}
		step.sender.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

			@return:
			  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == "ack" && e.ReplyTo == id })
		select {
		case duplicate := <-step.receiver.events:
			if duplicate.Type == step.kind && duplicate.ReplyTo == id {
				t.Fatal("duplicate signaling delivery")
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	b2 := f.connect(t, 0, f.memberToken, f.conference.ID)
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 2) })
	_ = b.conn.Close()
	<-b.done
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 1) })
	_ = b2.conn.Close()
	<-b2.done
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 0) })
	b3 := f.connect(t, 1, f.memberToken, f.conference.ID)
	if b3.state.ConnectionID == b.state.ConnectionID || b3.state.ParticipantID != b.state.ParticipantID {
		t.Fatal("reconnect did not replace only the connection")
	}
	other, err := f.service.Create(context.Background(), f.member.ID, conferences.CreateRequest{Title: "Other room"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Join(context.Background(), f.member.ID, other.ID, conferences.JoinRequest{})
	if err != nil {
		t.Fatal(err)
	}
	foreign := f.connect(t, 1, f.memberToken, other.ID)
	id := a.signal(t, "webrtc.offer", foreign.state.ConnectionID, map[string]any{"sdp": "cross conference"})
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == "error" && e.ReplyTo == id })
	select {
	case e := <-foreign.events:
		if e.Type == "webrtc.offer" {
			t.Fatal("cross-conference signal delivered")
		}
	case <-time.After(30 * time.Millisecond):
	}
	_, err = f.service.Leave(context.Background(), f.member.ID, f.conference.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-b3.done:
	case <-time.After(3 * time.Second):
		t.Fatal("leave did not close remote socket")
	}
	_, err = f.service.Transition(context.Background(), f.owner.ID, f.conference.ID, conferences.Finished)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.done:
	case <-time.After(3 * time.Second):
		t.Fatal("finish did not close socket")
	}
}

// TestStageTwoBrokerFailureClosesSockets проверяет сценарий «этап два Broker сбой Closes Sockets», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoBrokerFailureClosesSockets(t *testing.T) {
	f := stageTwo(t)
	a := f.connect(t, 0, f.ownerToken, f.conference.ID)
	b := f.connect(t, 1, f.memberToken, f.conference.ID)
	// Closing only this test's Redis client simulates a lost broker connection;
	// the shared Redis service and user's application are left untouched.
	_ = f.redis.Close()
	for _, s := range []*testSocket{a, b} {
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			t.Fatal("lost broker did not close socket")
		}
	}
	for _, hub := range f.hubs {
		hub.Shutdown()
		if hub.LocalCount() != 0 {
			t.Fatal("broker failure leaked registry")
		}
	}
	var count int64
	if err := f.db.Model(&domain.Session{}).Where("status='connected'").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("broker failure left connected history")
	}
}

// TestStageTwoJWTExpiryClosesLiveSession проверяет сценарий «этап два JWT истечение срока Closes Live сессия», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoJWTExpiryClosesLiveSession(t *testing.T) {
	f := stageTwo(t)
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "go-recorder", Subject: f.owner.ID, Audience: jwt.ClaimStrings{"go-recorder-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Second))}).SignedString([]byte(stageTwoSecret))
	if err != nil {
		t.Fatal(err)
	}
	s := f.connect(t, 0, token, f.conference.ID)
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
		t.Fatal("expired JWT left a live socket")
	}
	f.hubs[0].Shutdown()
	var session domain.Session
	if err = f.db.Where("connection_id=?", s.state.ConnectionID).Take(&session).Error; err != nil || session.Status != "disconnected" {
		t.Fatal("JWT expiry did not close session history")
	}
}

// TestStageTwoAuthTicketsAndRestrictions проверяет сценарий «этап два авторизация билеты и Restrictions», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoAuthTicketsAndRestrictions(t *testing.T) {
	f := stageTwo(t)
	path := "/api/v1/conferences/" + f.conference.ID + "/ws"
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "go-recorder", Subject: f.owner.ID, Audience: jwt.ClaimStrings{"go-recorder-api"}, IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)), ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute))}).SignedString([]byte(stageTwoSecret))
	outsider, _ := f.tokens.Issue(uuid.NewString())
	for _, step := range []struct {
		token  string
		status int
	}{{"", 401}, {"invalid", 401}, {expired, 401}, {outsider, 403}} {
		req, _ := http.NewRequest("GET", f.servers[0].URL+path, nil)
		if step.token != "" {
			req.Header.Set("Authorization", "Bearer "+step.token)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != step.status {
			t.Fatalf("expected %d, got %d", step.status, response.StatusCode)
		}
	}
	req, _ := http.NewRequest("POST", f.servers[0].URL+path+"-ticket", nil)
	req.Header.Set("Authorization", "Bearer "+f.ownerToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var value struct{ Ticket string }
	if err = json.NewDecoder(response.Body).Decode(&value); err != nil || response.StatusCode != 201 || len(value.Ticket) != 43 || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("ticket issuance failed")
	}
	address := "ws" + strings.TrimPrefix(f.servers[1].URL, "http") + path + "?ticket=" + value.Ticket
	conn, _, err := ws.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if conn, resp, err := ws.DefaultDialer.Dial(address, nil); err == nil || resp.StatusCode != 401 {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("ticket replay accepted")
	}
	identity := domain.Identity{UserID: f.owner.ID, ExpiresAt: time.Now().Add(time.Hour)}
	ticket := strings.Repeat("a", 43)
	_ = f.store.SaveTicket(context.Background(), ticket, f.conference.ID, identity, time.Millisecond)
	time.Sleep(3 * time.Millisecond)
	if _, err := f.store.ConsumeTicket(context.Background(), ticket, f.conference.ID); err == nil {
		t.Fatal("expired ticket accepted")
	}
	if conn, resp, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.servers[0].URL, "http")+path, http.Header{"Authorization": []string{"Bearer " + f.ownerToken}, "Origin": []string{"https://evil.example"}}); err == nil || resp.StatusCode != 403 {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("foreign origin accepted")
	}
	for _, status := range []conferences.Status{conferences.Finished, conferences.Cancelled} {
		view, err := f.service.Create(context.Background(), f.owner.ID, conferences.CreateRequest{Title: "Closed"})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.service.Join(context.Background(), f.owner.ID, view.ID, conferences.JoinRequest{})
		if status == conferences.Finished {
			_, _ = f.service.Transition(context.Background(), f.owner.ID, view.ID, conferences.Active)
		}
		_, _ = f.service.Transition(context.Background(), f.owner.ID, view.ID, status)
		if conn, response, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.servers[0].URL, "http")+"/api/v1/conferences/"+view.ID+"/ws", http.Header{"Authorization": []string{"Bearer " + f.ownerToken}}); err == nil || response.StatusCode != 409 {
			if conn != nil {
				_ = conn.Close()
			}
			t.Fatal("closed conference accepted websocket")
		}
	}
	left, err := f.service.Create(context.Background(), f.owner.ID, conferences.CreateRequest{Title: "Not joined"})
	if err != nil {
		t.Fatal(err)
	}
	if conn, response, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.servers[0].URL, "http")+"/api/v1/conferences/"+left.ID+"/ws", http.Header{"Authorization": []string{"Bearer " + f.ownerToken}}); err == nil || response.StatusCode != 403 {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("left membership accepted socket")
	}
	ticket = strings.Repeat("b", 43)
	_ = f.store.SaveTicket(context.Background(), ticket, f.conference.ID, identity, time.Second)
	if _, err := f.store.ConsumeTicket(context.Background(), ticket, left.ID); err == nil {
		t.Fatal("ticket accepted for another conference")
	}
}

// TestStageTwoPayloadLimitsAndHistoryConstraints проверяет сценарий «этап два Payload ограничения и история Constraints», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoPayloadLimitsAndHistoryConstraints(t *testing.T) {
	f := stageTwo(t)
	a := f.connect(t, 0, f.ownerToken, f.conference.ID)
	b := f.connect(t, 1, f.memberToken, f.conference.ID)
	for _, step := range []struct {
		kind    string
		payload map[string]any
		code    string
	}{
		{"webrtc.offer", map[string]any{"sdp": strings.Repeat("x", f.config.SDPBytes+1)}, "invalid_sdp"},
		{"webrtc.ice", map[string]any{"candidate": map[string]any{"candidate": strings.Repeat("x", f.config.ICEBytes)}}, "invalid_ice"},
		{"webrtc.offer", map[string]any{"sdp": "x", "senderConnectionId": uuid.NewString()}, "invalid_signal"},
	} {
		id := a.signal(t, step.kind, b.state.ConnectionID, step.payload)
		e := a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@args
			  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

			@return:
			  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == "error" && e.ReplyTo == id })
		var data struct{ Code string }
		_ = json.Unmarshal(e.Data, &data)
		if data.Code != step.code {
			t.Fatalf("expected %s, got %s", step.code, data.Code)
		}
	}
	// The database cannot accept a participant belonging to another identity.
	now := time.Now().UTC()
	invalid := domain.Session{ID: uuid.NewString(), ConnectionID: uuid.NewString(), ConferenceID: f.conference.ID, ParticipantID: a.state.ParticipantID, UserID: f.member.ID, Status: "connected", ConnectedAt: now, LastSeenAt: now}
	if err := f.db.Create(&invalid).Error; err == nil {
		t.Fatal("cross-user session FK accepted")
	}
	invalid.UserID = f.owner.ID
	invalid.Status = "disconnected"
	if err := f.db.Create(&invalid).Error; err == nil {
		t.Fatal("disconnected session without timestamp accepted")
	}
	// Simulate SQL commit immediately before a process dies, without Redis registration.
	orphan := invalid
	orphan.Status = "connected"
	orphan.ID = uuid.NewString()
	orphan.ConnectionID = uuid.NewString()
	orphan.ConnectedAt = now.Add(-3 * f.config.SessionTTL)
	orphan.LastSeenAt = orphan.ConnectedAt
	if err := pg.NewSessionRepository(f.db).Open(context.Background(), orphan); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var row domain.Session
		_ = f.db.Where("id=?", orphan.ID).Take(&row).Error
		if row.Status == "disconnected" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SQL-only crash session was not repaired")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := a.conn.WriteMessage(ws.TextMessage, []byte(strings.Repeat("x", int(f.config.MessageBytes)+1))); err != nil {
		t.Fatal(err)
	}
	select {
	case <-a.done:
	case <-time.After(3 * time.Second):
		t.Fatal("oversized frame did not close connection")
	}
}

// TestStageTwoDeadLeaseAndShutdown проверяет сценарий «этап два недействующий аренда и завершение», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoDeadLeaseAndShutdown(t *testing.T) {
	f := stageTwo(t)
	a := f.connect(t, 0, f.ownerToken, f.conference.ID)
	// No read loop means no native pong; server must expire it without client close.
	dead, _, err := ws.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.servers[1].URL, "http")+"/api/v1/conferences/"+f.conference.ID+"/ws", http.Header{"Authorization": []string{"Bearer " + f.memberToken}})
	if err != nil {
		t.Fatal(err)
	}
	defer dead.Close()
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 1) })
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.member.ID, 0) })
	// Emulate process crash: lease expires but no local socket calls Unregister.
	now := time.Now().UTC()
	ghost := domain.Session{ID: uuid.NewString(), ConferenceID: f.conference.ID, ParticipantID: a.state.ParticipantID, UserID: f.owner.ID, ConnectionID: uuid.NewString(), Status: "connected", ConnectedAt: now, LastSeenAt: now}
	repo := pg.NewSessionRepository(f.db)
	if err = repo.Open(context.Background(), ghost); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Register(context.Background(), ghost, 50*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.owner.ID, 2) })
	a.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return presenceCount(e, f.owner.ID, 1) })
	for _, hub := range f.hubs {
		hub.Shutdown()
		if hub.LocalCount() != 0 {
			t.Fatal("local registry leaked")
		}
	}
	var count int64
	if err = f.db.Model(&domain.Session{}).Where("status='connected'").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("connected history leaked: %d, %v", count, err)
	}
	active, err := f.store.Active(context.Background(), f.conference.ID)
	if err != nil || len(active) != 0 {
		t.Fatal("Redis active sessions leaked")
	}
}

// TestStageTwoLoad100Connections проверяет сценарий «этап два Load100Connections», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoLoad100Connections(t *testing.T) {
	f := stageTwo(t)
	start := time.Now()
	sockets := make([]*testSocket, 100)
	// Connect sequentially but keep every socket open and draining; simultaneous
	// means 100 live sockets, not 100 upgrade requests or media participants.
	for i := range sockets {
		sockets[i] = f.connect(t, i%2, f.ownerToken, f.conference.ID)
	}
	active, err := f.store.Active(context.Background(), f.conference.ID)
	if err != nil || len(active) != 100 {
		t.Fatalf("expected 100 simultaneous connections, got %d: %v", len(active), err)
	}
	for _, socket := range sockets {
		select {
		case <-socket.done:
			t.Fatal("load socket closed prematurely")
		default:
		}
	}
	t.Logf("load smoke: %d live sockets, 2 API instances, connect time %s", len(active), time.Since(start).Round(time.Millisecond))
	var wg sync.WaitGroup
	for _, s := range sockets {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@args
		  - s (*testSocket): значение s типа *testSocket, используемое согласно назначению этой операции.
		*/func(s *testSocket) { defer wg.Done(); _ = s.conn.Close(); <-s.done }(s)
	}
	wg.Wait()
	for _, hub := range f.hubs {
		hub.Shutdown()
	}
	active, err = f.store.Active(context.Background(), f.conference.ID)
	if err != nil || len(active) != 0 {
		t.Fatal("load cleanup leaked")
	}
}

// TestStageTwoBrowserProof проверяет сценарий «этап два браузер Proof», фиксируя ошибки поведения как регрессию.
// Внешняя команда или запрос использует контекст операции.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageTwoBrowserProof(t *testing.T) {
	if os.Getenv("RECORDER_FRONTEND_E2E") != "true" {
		t.Skip("set RECORDER_FRONTEND_E2E=true for the two-browser DataChannel proof")
	}
	f := stageTwo(t)
	// Browser lifecycle/ICE negotiation needs production-style heartbeat timing.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "npm", "run", "test:e2e", "--", "e2e/realtime.spec.ts", "--workers=1")
	command.Dir = "../../frontend"
	command.Env = append(os.Environ(), "API_PROXY_TARGET="+f.servers[0].URL, "MEET_LIVE_TEST_URL=http://127.0.0.1:5175", "MEET_REALTIME_CONFERENCE="+f.conference.ID, "MEET_REALTIME_ALICE="+f.owner.ID, "MEET_REALTIME_BOB="+f.member.ID)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("two-browser proof: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}
