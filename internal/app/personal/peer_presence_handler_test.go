package personalapp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// peerPresenceRepository проверяет порядок допуска и чтения без настоящей базы данных.
// @params
//   - Repository: остальные методы контракта не используются этим GET-запросом.
//   - item, getError, accountError: заданная проекция чата и безопасно обрабатываемые ошибки.
//   - accountCalls, getCalls, actor, conversation: параметры и число обращений обработчика.
//   - order: общий журнал операций для проверки ACL перед хранилищем присутствия.
//   - cancelOnGet: отменяет родительский запрос во время проверки членства.
type peerPresenceRepository struct {
	personalapp.Repository
	item                   personal.Conversation
	getError, accountError error
	accountCalls, getCalls int
	actor, conversation    string
	order                  *[]string
	cancelOnGet            context.CancelFunc
}

// Account имитирует запрет гостевых или недействительных аккаунтов.
// @args user — идентификатор, ранее проверенный middleware.
// @return заданная ошибка допуска либо nil.
func (r *peerPresenceRepository) Account(_ context.Context, user string) error {
	r.accountCalls++
	r.actor = user
	*r.order = append(*r.order, "account")
	return r.accountError
}

// Get сохраняет проверенную идентичность и возвращает проекцию только указанного чата.
// @args actor — вызывающий аккаунт; id — нормализованный UUID переписки.
// @return проекция и заданная ошибка проверки доступа.
func (r *peerPresenceRepository) Get(_ context.Context, actor, id string) (personal.Conversation, error) {
	r.getCalls++
	r.actor, r.conversation = actor, id
	*r.order = append(*r.order, "acl")
	if r.cancelOnGet != nil {
		r.cancelOnGet()
	}
	return r.item, r.getError
}

// peerPresenceProvider подменяет присутствие и фиксирует предел времени чтения.
// @params
//   - statuses, failure: ответ или ошибка изолированного хранилища.
//   - calls, ids, lookupAt, deadline, hasDeadline: параметры и время вызова Online.
//   - order: общий журнал, подтверждающий проверку ACL до Online.
//   - cancelOnLookup: отменяет запрос после получения результата присутствия.
type peerPresenceProvider struct {
	statuses       map[string]bool
	failure        error
	calls          int
	ids            []string
	lookupAt       time.Time
	deadline       time.Time
	hasDeadline    bool
	order          *[]string
	cancelOnLookup context.CancelFunc
}

// Online возвращает только настроенную проекцию, не создавая и не продлевая сессии.
// @args ctx — ограниченный контекст чтения; ids — серверные идентификаторы собеседников.
// @return заранее заданные статусы и ошибка без сетевых обращений.
func (p *peerPresenceProvider) Online(ctx context.Context, ids []string) (map[string]bool, error) {
	p.calls++
	p.lookupAt = time.Now()
	p.ids = append([]string(nil), ids...)
	p.deadline, p.hasDeadline = ctx.Deadline()
	*p.order = append(*p.order, "presence")
	if p.cancelOnLookup != nil {
		p.cancelOnLookup()
	}
	return p.statuses, p.failure
}

// peerPresenceVerifier связывает тестовый Bearer-токен с одним аккаунтом.
// @params tokens — фиксированные идентичности без настоящих JWT или секретов.
type peerPresenceVerifier struct{ tokens map[string]string }

// Verify отвергает неизвестные токены до вызова репозитория и хранилища.
// @args token — токен из заголовка изолированного HTTP-запроса.
// @return идентификатор аккаунта либо ошибка отсутствующей авторизации.
func (v peerPresenceVerifier) Verify(token string) (string, error) {
	if user := v.tokens[token]; user != "" {
		return user, nil
	}
	return "", apperrors.ErrUnauthorized
}

// peerPresenceLimiter записывает правило частоты без Redis и пользовательских данных.
// @params allow, calls, key, limit, window — решение и параметры проверки ограничения.
type peerPresenceLimiter struct {
	allow  bool
	calls  int
	key    string
	limit  int
	window time.Duration
}

// Allow имитирует разрешение либо отказ того же лимита, который используется в маршруте.
// @args key — область аккаунта и маршрута; limit — предел; window — окно запросов.
// @return фиксированное решение, безопасное для теста без внешнего хранилища.
func (l *peerPresenceLimiter) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error) {
	l.calls++
	l.key, l.limit, l.window = key, limit, window
	return ratelimit.Result{Allowed: l.allow, Limit: limit, Remaining: limit - 1, RetryAfter: time.Minute}, nil
}

// peerPresenceRouter подключает настоящий маршрут и всю цепочку защиты к локальным подменам.
// @args h — обработчик; actor — аккаунт Bearer-токена; limiter — изолированный ограничитель.
// @return Gin router без настоящей базы, Redis и HTTP-сервера.
func peerPresenceRouter(h *personalapp.Handler, actor string, limiter middleware.Limiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	verifier := peerPresenceVerifier{tokens: map[string]string{"actor-token": actor}}
	httptransport.RegisterPersonalRoutes(router, h, chatapp.NewHandler(nil).ForConversations(), middleware.Authenticate(verifier), limiter)
	return router
}

// peerPresenceRequest отправляет запрос прямо в локальный router и проверяет приватные заголовки.
// @args t — контекст теста; router — маршруты; path — адрес; token — Bearer; ctx — контекст запроса.
// @return записанный HTTP-ответ без создания внешних соединений.
func peerPresenceRequest(t *testing.T, router *gin.Engine, path, token string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("статус присутствия может кэшироваться или интерпретироваться как другой тип", response.Header())
	}
	return response
}

// TestPeerPresenceAuthorizedProjection проверяет ACL, истинный и ложный статусы и отсутствие лишних данных.
// @args t — контекст теста защищённого HTTP-ответа.
func TestPeerPresenceAuthorizedProjection(t *testing.T) {
	for _, online := range []bool{true, false} {
		// Проверяет каждую подтверждённую проекцию с отдельной идентичностью и подменой.
		t.Run(map[bool]string{true: "online", false: "offline"}[online], func(t *testing.T) {
			actor, peer, id, forged := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
			order := []string{}
			repo := &peerPresenceRepository{order: &order, item: personal.Conversation{
				ID: id, Type: "direct", Peer: &personal.Peer{ID: peer, DisplayName: "private@example.test"}, Preview: "private-message",
			}}
			presence := &peerPresenceProvider{order: &order, statuses: map[string]bool{peer: online, forged: !online}}
			router := peerPresenceRouter(&personalapp.Handler{Repo: repo, Presence: presence}, actor, nil)
			response := peerPresenceRequest(t, router, "/api/v1/conversations/"+strings.ToUpper(id)+"/peer-presence?userId="+forged+"&targetUser="+forged,
				"actor-token", context.Background())
			if response.Code != http.StatusOK || repo.actor != actor || repo.conversation != id || repo.getCalls != 1 || presence.calls != 1 {
				t.Fatal("неверная идентичность или число обращений", response.Code, response.Body.String(), repo, presence)
			}
			if !reflect.DeepEqual(order, []string{"account", "acl", "presence"}) || !reflect.DeepEqual(presence.ids, []string{peer}) {
				t.Fatal("хранилище прочитано до ACL или по идентификатору из query", order, presence.ids)
			}
			if !presence.hasDeadline || presence.deadline.After(presence.lookupAt.Add(2*time.Second)) || !presence.deadline.After(presence.lookupAt) {
				t.Fatal("контекст присутствия не ограничен двумя секундами", presence.deadline)
			}
			var payload struct {
				Status string         `json:"status"`
				Item   map[string]any `json:"item"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Status != "success" || len(payload.Item) != 3 || payload.Item["conversationId"] != id || payload.Item["peerId"] != peer || payload.Item["online"] != online {
				t.Fatal("ответ не содержит ровно разрешённые идентификаторы и явный bool", payload)
			}
			if strings.Contains(response.Body.String(), "private") || strings.Contains(response.Body.String(), "connections") {
				t.Fatal("ответ раскрыл имя, сообщение или физические соединения", response.Body.String())
			}
		})
	}
}

// TestPeerPresenceRejectsBeforeLookup проверяет, что ошибка доступа или проекции не раскрывает присутствие.
// @args t — контекст негативных проверок цепочки авторизации.
func TestPeerPresenceRejectsBeforeLookup(t *testing.T) {
	actor, peer, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, test := range []struct {
		name, path, token      string
		item                   personal.Conversation
		accountError, getError error
		status, getCalls       int
	}{
		{name: "unauthenticated", path: id, status: 401},
		{name: "invalid-token", path: id, token: "invalid", status: 401},
		{name: "guest", path: id, token: "actor-token", accountError: apperrors.ErrForbidden, status: 403},
		{name: "malformed-id", path: "invalid", token: "actor-token", status: 400},
		{name: "zero-id", path: uuid.Nil.String(), token: "actor-token", status: 400},
		{name: "foreign", path: id, token: "actor-token", getError: apperrors.ErrForbidden, status: 403, getCalls: 1},
		{name: "hidden", path: id, token: "actor-token", getError: apperrors.ErrForbidden, status: 403, getCalls: 1},
		{name: "group", path: id, token: "actor-token", item: personal.Conversation{ID: id, Type: "group"}, status: 403, getCalls: 1},
		{name: "missing-peer", path: id, token: "actor-token", item: personal.Conversation{ID: id, Type: "direct"}, status: 403, getCalls: 1},
		{name: "malformed-peer", path: id, token: "actor-token", item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: "invalid"}}, status: 403, getCalls: 1},
		{name: "zero-peer", path: id, token: "actor-token", item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: uuid.Nil.String()}}, status: 403, getCalls: 1},
		{name: "self-peer", path: id, token: "actor-token", item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: actor}}, status: 403, getCalls: 1},
		{name: "wrong-conversation", path: id, token: "actor-token", item: personal.Conversation{ID: uuid.NewString(), Type: "direct", Peer: &personal.Peer{ID: peer}}, status: 403, getCalls: 1},
		{name: "repository-failure", path: id, token: "actor-token", getError: errors.New("private database error"), status: 500, getCalls: 1},
	} {
		// Проверяет конкретный отказ без обращения к данным собеседника.
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			repo := &peerPresenceRepository{order: &order, item: test.item, accountError: test.accountError, getError: test.getError}
			presence := &peerPresenceProvider{order: &order, statuses: map[string]bool{peer: true}}
			router := peerPresenceRouter(&personalapp.Handler{Repo: repo, Presence: presence}, actor, nil)
			response := peerPresenceRequest(t, router, "/api/v1/conversations/"+test.path+"/peer-presence", test.token, context.Background())
			if response.Code != test.status || repo.getCalls != test.getCalls || presence.calls != 0 {
				t.Fatal("отказ не остановил чтение присутствия", response.Code, response.Body.String(), repo.getCalls, presence.calls)
			}
			if strings.Contains(response.Body.String(), "online") || strings.Contains(response.Body.String(), "private database") {
				t.Fatal("ошибка раскрыла статус или внутренние данные", response.Body.String())
			}
		})
	}
}

// TestPeerPresenceUnavailableDoesNotBecomeOffline проверяет неполные данные и ошибки без ложного false.
// @args t — контекст проверки недоступного хранилища присутствия.
func TestPeerPresenceUnavailableDoesNotBecomeOffline(t *testing.T) {
	actor, peer, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, test := range []struct {
		name       string
		nilService bool
		statuses   map[string]bool
		failure    error
	}{
		{name: "not-configured", nilService: true},
		{name: "nil-map"},
		{name: "missing-peer", statuses: map[string]bool{uuid.NewString(): true}},
		{name: "store-failure", statuses: map[string]bool{peer: false}, failure: errors.New("private redis address")},
		{name: "timeout", failure: context.DeadlineExceeded},
		{name: "cancelled-store", failure: context.Canceled},
	} {
		// Проверяет недоступность независимо от ошибочно присланных частичных статусов.
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			repo := &peerPresenceRepository{order: &order, item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: peer}}}
			presence := &peerPresenceProvider{order: &order, statuses: test.statuses, failure: test.failure}
			h := &personalapp.Handler{Repo: repo}
			if !test.nilService {
				h.Presence = presence
			}
			response := peerPresenceRequest(t, peerPresenceRouter(h, actor, nil), "/api/v1/conversations/"+id+"/peer-presence", "actor-token", context.Background())
			if response.Code != http.StatusServiceUnavailable || repo.getCalls != 1 || strings.Contains(response.Body.String(), "online") || strings.Contains(response.Body.String(), "private redis") {
				t.Fatal("неизвестный статус стал офлайн или раскрыл внутреннюю ошибку", response.Code, response.Body.String(), repo.getCalls)
			}
		})
	}
}

// TestPeerPresenceParentCancellation проверяет отмену до ACL, после ACL и после чтения Redis.
// @args t — контекст проверки, запрещающей успешный ответ отменённому запросу.
func TestPeerPresenceParentCancellation(t *testing.T) {
	for _, phase := range []string{"before-acl", "after-acl", "after-presence"} {
		// Отменяет только память локального запроса, не используя настоящие соединения.
		t.Run(phase, func(t *testing.T) {
			actor, peer, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			order := []string{}
			repo := &peerPresenceRepository{order: &order, item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: peer}}}
			presence := &peerPresenceProvider{order: &order, statuses: map[string]bool{peer: true}}
			if phase == "before-acl" {
				cancel()
			} else if phase == "after-acl" {
				repo.cancelOnGet = cancel
			} else {
				presence.cancelOnLookup = cancel
			}
			response := peerPresenceRequest(t, peerPresenceRouter(&personalapp.Handler{Repo: repo, Presence: presence}, actor, nil),
				"/api/v1/conversations/"+id+"/peer-presence", "actor-token", ctx)
			if response.Code != 503 || strings.Contains(response.Body.String(), "online") {
				t.Fatal("отменённый запрос получил подтверждённый статус", phase, response.Code, response.Body.String())
			}
			if phase != "after-presence" && presence.calls != 0 {
				t.Fatal("отмена не остановила обращение к Redis", phase, presence.calls)
			}
		})
	}
}

// TestPeerPresenceRateRule проверяет 120 запросов в минуту и отказ до чтения чата/присутствия.
// @args t — контекст проверки отдельной области ограничителя для аккаунта.
func TestPeerPresenceRateRule(t *testing.T) {
	actor, peer, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
	order := []string{}
	repo := &peerPresenceRepository{order: &order, item: personal.Conversation{ID: id, Type: "direct", Peer: &personal.Peer{ID: peer}}}
	presence := &peerPresenceProvider{order: &order, statuses: map[string]bool{peer: true}}
	limiter := &peerPresenceLimiter{allow: false}
	router := peerPresenceRouter(&personalapp.Handler{Repo: repo, Presence: presence}, actor, limiter)
	response := peerPresenceRequest(t, router, "/api/v1/conversations/"+id+"/peer-presence", "actor-token", context.Background())
	if response.Code != 429 || limiter.calls != 1 || limiter.limit != 120 || limiter.window != time.Minute ||
		!strings.Contains(limiter.key, "personal_peer_presence") || !strings.Contains(limiter.key, actor) ||
		repo.getCalls != 0 || presence.calls != 0 || response.Header().Get("Retry-After") != "60" {
		t.Fatal("неверное правило либо запрос обошёл ограничение", response.Code, limiter, repo.getCalls, presence.calls)
	}
	limiter.allow = true
	response = peerPresenceRequest(t, router, "/api/v1/conversations/"+id+"/peer-presence", "actor-token", context.Background())
	if response.Code != 200 || limiter.calls != 2 || repo.getCalls != 1 || presence.calls != 1 {
		t.Fatal("разрешённый лимитом запрос не использовал защищённый маршрут", response.Code, limiter.calls, repo.getCalls, presence.calls)
	}
}
