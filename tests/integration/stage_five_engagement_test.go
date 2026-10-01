package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	engagementapp "github.com/janickiy/go-recorder/internal/app/engagement"
	notificationsapp "github.com/janickiy/go-recorder/internal/app/notifications"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	notifications "github.com/janickiy/go-recorder/internal/domain/notifications"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	notificationcase "github.com/janickiy/go-recorder/internal/usecase/notifications"
	realtimecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
)

// TestStageFiveHandsReactionsAndReconnect проверяет сценарий «этап пять Hands Reactions и переподключение», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveHandsReactionsAndReconnect(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	hands := redisinfra.NewHands(f.redis, f.config.Namespace)
	for _, hub := range f.hubs {
		hub.SetHands(hands)
	}
	service := realtimecase.NewEngagement(pg.NewSessionRepository(f.db), hands, f.hubs[0])
	owner := f.connect(t, 0, f.ownerToken, f.conference.ID)
	member := f.connect(t, 1, f.memberToken, f.conference.ID)
	raised, err := service.Hand(ctx, f.conference.ID, f.member.ID, member.state.ParticipantID, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Hand(ctx, f.conference.ID, f.member.ID, member.state.ParticipantID, true)
	if err != nil || !again.RaisedAt.Equal(raised.RaisedAt) {
		t.Fatal("duplicate raise changed order", err)
	}
	owner.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@parameters:
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.Type == "hand.raised" })
	another := f.connect(t, 0, f.memberToken, f.conference.ID)
	if len(another.state.Hands) != 1 || another.state.Hands[0].ParticipantID != member.state.ParticipantID {
		t.Fatal("reconnect lost hands")
	}
	if _, err := service.Hand(ctx, f.conference.ID, f.member.ID, owner.state.ParticipantID, false); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("member lowered owner", err)
	}
	if _, err := service.Hand(ctx, f.conference.ID, f.owner.ID, member.state.ParticipantID, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Reaction(ctx, f.conference.ID, f.member.ID, "💥"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("emoji not checked")
	}
	router := gin.New()
	httptransport.RegisterEngagementRoutes(router, engagementapp.NewHandler(service, redisinfra.NewRateLimiter(f.redis), f.config.Namespace), httpmiddleware.Authenticate(f.tokens))
	api := stageOneAPI{router: router}
	for i := 0; i < 5; i++ {
		api.expect(t, "POST", "/conferences/"+f.conference.ID+"/reactions", f.memberToken, map[string]string{"emoji": "👍"}, 200, nil)
	}
	api.expect(t, "POST", "/conferences/"+f.conference.ID+"/reactions", f.memberToken, map[string]string{"emoji": "👍"}, 429, nil)
	signalID := owner.signal(t, "webrtc.offer", member.state.ConnectionID, map[string]any{"sdp": "v=0\r\n"})
	member.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@parameters:
		  - e (domain.Envelope): значение e типа domain.Envelope, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(e domain.Envelope) bool { return e.ID == signalID || e.Type == "webrtc.offer" })
	if _, err := f.service.Transition(ctx, f.owner.ID, f.conference.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	if err := service.Reaction(ctx, f.conference.ID, f.member.ID, "👍"); err == nil {
		t.Fatal("reaction after finish accepted")
	}
}

// TestStageFiveNotificationDedupAuthorizationAndSSE проверяет сценарий «этап пять уведомление дедупликация авторизация и SSE», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveNotificationDedupAuthorizationAndSSE(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	soon := time.Now().Add(10 * time.Minute)
	conf, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Notification smoke", ScheduledAt: &soon})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Join(ctx, f.member.ID, conf.ID, conferences.JoinRequest{InviteCode: conf.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE conference_participants SET admission_decided_at=now(),admission_version=2 WHERE conference_id=? AND user_id=?", conf.ID, f.member.ID).Error; err != nil {
		t.Fatal(err)
	}
	recordID := uuid.NewString()
	if err = f.db.Exec("INSERT INTO record(uuid,conference_id,platform_conference_id,mode,source_type,transport_type,status,quality_mode) VALUES(?,?,?,'composite','conference','sfu','ready','auto')", recordID, f.conference.ID, f.conference.ID).Error; err != nil {
		t.Fatal(err)
	}
	repo := pg.NewNotificationRepository(f.db)
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	service := notificationcase.NewService(repo, bus)
	router := gin.New()
	httptransport.RegisterNotificationRoutes(router, notificationsapp.NewHandler(service, bus, f.tokens, redisinfra.NewRateLimiter(f.redis), f.config.Namespace), httpmiddleware.Authenticate(f.tokens))
	server := httptest.NewServer(router)
	defer server.Close()
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(requestCtx, "GET", server.URL+"/api/v1/notifications/events", nil)
	req.Header.Set("Authorization", "Bearer "+f.memberToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("SSE status %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	if !scanner.Scan() {
		t.Fatal("SSE initial comment missing")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			if err := service.Tick(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	page, err := repo.List(ctx, f.member.ID, "", 10)
	if err != nil || len(page.Items) != 3 || page.UnreadCount != 3 {
		t.Fatalf("notifications dedup/list: %+v %v", page, err)
	}
	found := false
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			var event domain.Envelope
			if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &event) == nil && event.Type == "notification.created" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("SSE notification missing", scanner.Err())
	}
	item := page.Items[0]
	if item.Payload.ConferenceID == "" || item.Version != 1 {
		t.Fatal("invalid payload serialization")
	}
	if _, err := repo.Read(ctx, f.owner.ID, item.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("notification IDOR", err)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			n, err := service.Read(ctx, f.member.ID, item.ID)
			if err != nil || n.ReadAt == nil || n.Payload.ConferenceID == "" {
				t.Error("read failed", err)
			}
		}()
	}
	wg.Wait()
	first, err := repo.List(ctx, f.member.ID, "", 1)
	if err != nil || first.NextCursor == nil || first.UnreadCount != 2 {
		t.Fatal("pagination/unread", err)
	}
	second, err := repo.List(ctx, f.member.ID, *first.NextCursor, 10)
	if err != nil || len(second.Items) != 2 {
		t.Fatal("keyset pagination", err)
	}
	if _, err := repo.List(ctx, f.owner.ID, *first.NextCursor, 10); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("foreign cursor accepted")
	}
	api := stageOneAPI{router: router}
	api.expect(t, "GET", "/notifications", "", nil, 401, nil)
	api.expect(t, "GET", "/notifications/events?token=not-allowed", f.memberToken, nil, 401, nil)
	var count int64
	f.db.Model(&notifications.Notification{}).Where("published_at IS NULL").Count(&count)
	if count != 0 {
		t.Fatal("outbox not published")
	}
}

// TestStageFiveRealtimeBurstPreservesSignaling проверяет сценарий «этап пять события реального времени Burst Preserves сигнализация», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveRealtimeBurstPreservesSignaling(t *testing.T) {
	f := stageTwo(t)
	owner := f.connect(t, 0, f.ownerToken, f.conference.ID)
	member := f.connect(t, 1, f.memberToken, f.conference.ID)
	finished := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer close(finished)
		for i := 0; i < 1500; i++ {
			kind := "reaction.created"
			if i%2 == 0 {
				kind = "chat.message.created"
			}
			_ = f.hubs[0].Broadcast(context.Background(), domain.Event(kind, f.conference.ID, map[string]string{"participantId": owner.state.ParticipantID}))
		}
	}()
	latencies := make([]time.Duration, 0, 20)
	for i := 0; i < 20; i++ {
		start := time.Now()
		owner.signal(t, "webrtc.offer", member.state.ConnectionID, map[string]any{"sdp": "v=0\r\n"})
		member.wait(t, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@parameters:
			  - event (domain.Envelope): конверт входящего или публикуемого события.

			@return:
			  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(event domain.Envelope) bool { return event.Type == "webrtc.offer" })
		latencies = append(latencies, time.Since(start))
	}
	<-finished
	sort.Slice(latencies, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		@parameters:
		  - i (int): значение i типа int, используемое согласно назначению этой операции.
		  - j (int): значение j типа int, используемое согласно назначению этой операции.

		@return:
		  - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния. */func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("1500 collaboration events + 20 signaling exchanges: p50=%s p95=%s max=%s", latencies[10], latencies[18], latencies[19])
	if latencies[19] > 2*time.Second {
		t.Fatal("critical signaling starved by collaboration")
	}
	for i := 0; i < 5; i++ {
		f.connect(t, i%2, f.memberToken, f.conference.ID)
	}
}

// TestStageFiveNotificationStreamExpiresWithJWT проверяет сценарий «этап пять уведомление Stream Expires с JWT», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveNotificationStreamExpiresWithJWT(t *testing.T) {
	f := stageTwo(t)
	bus := redisinfra.NewNotificationBus(f.redis, f.config.Namespace)
	service := notificationcase.NewService(pg.NewNotificationRepository(f.db), bus)
	router := gin.New()
	httptransport.RegisterNotificationRoutes(router, notificationsapp.NewHandler(service, bus, f.tokens, nil, f.config.Namespace), httpmiddleware.Authenticate(f.tokens))
	server := httptest.NewServer(router)
	defer server.Close()
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "go-recorder", Subject: f.owner.ID, Audience: jwt.ClaimStrings{"go-recorder-api"}, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Second))}).SignedString([]byte(stageTwoSecret))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", server.URL+"/api/v1/notifications/events", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("valid stream rejected", response.StatusCode)
	}
	_, err = io.ReadAll(response.Body)
	if err != nil {
		t.Fatal("stream did not end at token expiry", err)
	}
	if time.Since(now) > 3*time.Second {
		t.Fatal("expired stream remained open")
	}
}

// TestStageFiveEngagementAdmissionPrecedesSharedLimit проверяет сценарий «этап пять Engagement допуск Precedes Shared лимит», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFiveEngagementAdmissionPrecedesSharedLimit(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	if _, err := f.service.Leave(ctx, f.member.ID, f.conference.ID); err != nil {
		t.Fatal(err)
	}
	limiter := redisinfra.NewRateLimiter(f.redis)
	service := realtimecase.NewEngagement(pg.NewSessionRepository(f.db), redisinfra.NewHands(f.redis, f.config.Namespace), f.hubs[0])
	router := gin.New()
	httptransport.RegisterEngagementRoutes(router, engagementapp.NewHandler(service, limiter, f.config.Namespace), httpmiddleware.Authenticate(f.tokens))
	api := stageOneAPI{router: router}
	api.expect(t, "POST", "/conferences/"+f.conference.ID+"/reactions", f.memberToken, map[string]string{"emoji": "👍"}, 403, nil)
	count, err := f.redis.ZCard(ctx, f.config.Namespace+":engagement:reactions:conference:"+f.conference.ID).Result()
	if err != nil || count != 0 {
		t.Fatal("nonmember spent conference rate budget", err, count)
	}
	for i := 0; i < 120; i++ {
		if _, err := limiter.Allow(ctx, f.config.Namespace+":engagement:hands-read:user:"+f.owner.ID, 120, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	response := api.expect(t, "GET", "/conferences/"+f.conference.ID+"/hands", f.ownerToken, nil, 429, nil)
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("missing private cache policy")
	}
}
