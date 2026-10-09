package personalapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
)

// directActionsStub подменяет только персональные действия для проверки HTTP-контракта.
type directActionsStub struct {
	Repository
	actor, conversation, action string
	enabled                     bool
	failure                     error
}

// SetDirectPreferences записывает идентичность запроса и возвращает тестовую настройку.
// @args: actor — текущий аккаунт; id — переписка; enabled — новое разрешение уведомлений.
// @return: минимальная проекция и заранее заданная ошибка.
func (r *directActionsStub) SetDirectPreferences(_ context.Context, actor, id string, enabled bool) (personal.Conversation, error) {
	r.actor, r.conversation, r.action, r.enabled = actor, id, "preferences", enabled
	return personal.Conversation{ID: id, Type: "direct", NotificationsEnabled: enabled}, r.failure
}

// ClearDirectHistory имитирует персональную очистку с фиксированной границей истории.
// @args: actor — текущий аккаунт; id — переписка.
// @return: проекция с границей 42 и заранее заданная ошибка.
func (r *directActionsStub) ClearDirectHistory(_ context.Context, actor, id string) (personal.Conversation, error) {
	r.actor, r.conversation, r.action = actor, id, "clear"
	return personal.Conversation{ID: id, Type: "direct", NotificationsEnabled: true, HistoryClearedThrough: 42}, r.failure
}

// HideDirectConversation сохраняет параметры вызова без изменения настоящего хранилища.
// @args: actor — текущий аккаунт; id — переписка.
// @return: подтверждённая граница 73 и заранее заданная ошибка операции.
func (r *directActionsStub) HideDirectConversation(_ context.Context, actor, id string) (int64, error) {
	r.actor, r.conversation, r.action = actor, id, "hide"
	return 73, r.failure
}

// directActionBus записывает события в память, не используя сеть или пользовательские данные.
type directActionBus struct {
	recipients []string
	events     []realtime.Envelope
}

// Publish сохраняет адресата и событие для последующей проверки отсутствия рассылки собеседнику.
// @args: user — адресат; event — персональное событие.
// @return: nil после сохранения в память теста.
func (b *directActionBus) Publish(_ context.Context, user string, event realtime.Envelope) error {
	b.recipients = append(b.recipients, user)
	b.events = append(b.events, event)
	return nil
}

// TestDirectActionHandlersOwnerScopedEvents проверяет, что результат получает только вызвавший действие аккаунт.
// @args: t — контекст теста HTTP и локальной шины.
func TestDirectActionHandlersOwnerScopedEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	actor, id := uuid.NewString(), uuid.NewString()
	for _, test := range []struct{ method, suffix, body, kind, action string }{
		{"PATCH", "preferences", `{"notificationsEnabled":false}`, "conversation.preferences.updated", "preferences"},
		{"POST", "clear-history", `{}`, "conversation.history.cleared", "clear"},
		{"POST", "hide", `{}`, "conversation.hidden", "hide"},
	} {
		t.Run(test.action, func(t *testing.T) {
			repo, bus := &directActionsStub{}, &directActionBus{}
			h := &Handler{Repo: repo, Events: &personalusecase.Events{Bus: bus}}
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set("authenticated_user_id", actor); c.Next() })
			router.PATCH("/:id/preferences", h.SetDirectPreferences)
			router.POST("/:id/clear-history", h.ClearDirectHistory)
			router.POST("/:id/hide", h.HideDirectConversation)
			request := httptest.NewRequest(test.method, "/"+id+"/"+test.suffix, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || repo.actor != actor || repo.conversation != id || repo.action != test.action {
				t.Fatal("неверный контракт", response.Code, response.Body.String(), repo)
			}
			if len(bus.events) != 1 || len(bus.recipients) != 1 || bus.recipients[0] != actor || bus.events[0].Type != test.kind {
				t.Fatal("событие направлено не владельцу", bus)
			}
			var payload struct {
				ConversationID, Type, UserID string
				Item                         *personal.Conversation
				HistoryClearedThrough        int64
				Hidden                       bool
			}
			if err := json.Unmarshal(bus.events[0].Data, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.ConversationID != id || payload.UserID != actor || payload.Type != "direct" {
				t.Fatal("неверная нагрузка события", payload)
			}
			if test.action == "clear" && (payload.Item == nil || payload.HistoryClearedThrough != 42) {
				t.Fatal("нет границы очищенной истории", payload)
			}
			if test.action == "hide" && payload.Item != nil {
				t.Fatal("минимальное событие скрытия содержит лишние данные", payload)
			}
			if test.action == "hide" {
				var receipt struct {
					Hidden                bool
					HistoryClearedThrough int64
				}
				if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
					t.Fatal(err)
				}
				if !receipt.Hidden || receipt.HistoryClearedThrough != 73 || !payload.Hidden || payload.HistoryClearedThrough != 73 {
					t.Fatal("ответ скрытия не фиксирует точную границу", receipt, payload)
				}
			}
		})
	}
}

// TestDirectPreferenceHandlerValidation проверяет строгий JSON и отсутствие изменения при ошибке.
// @args: t — контекст проверки входных данных.
func TestDirectPreferenceHandlerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	id := uuid.NewString()
	for _, test := range []struct {
		path, body string
		status     int
	}{
		{id, `{}`, 422}, {id, `{"notificationsEnabled":null}`, 422},
		{id, `{"notificationsEnabled":"false"}`, 400}, {id, `{"notificationsEnabled":false,"userId":"forged"}`, 400},
		{"invalid", `{"notificationsEnabled":false}`, 422},
	} {
		repo := &directActionsStub{}
		router := gin.New()
		router.PATCH("/:id/preferences", (&Handler{Repo: repo}).SetDirectPreferences)
		response := httptest.NewRecorder()
		request := httptest.NewRequest("PATCH", "/"+test.path+"/preferences", strings.NewReader(test.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		if response.Code != test.status || repo.action != "" {
			t.Fatal("ошибочный запрос изменил настройки", test, response.Code, response.Body.String(), repo)
		}
	}
	// Неуспешное сохранение не должно публиковать событие успеха.
	bus := &directActionBus{}
	h := &Handler{Repo: &directActionsStub{failure: apperrors.ErrForbidden}, Events: &personalusecase.Events{Bus: bus}}
	router := gin.New()
	router.POST("/:id/hide", h.HideDirectConversation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/"+id+"/hide", nil))
	if response.Code != 403 || len(bus.events) != 0 {
		t.Fatal("ошибка сохранения опубликована как успех", response.Code, bus)
	}
}
