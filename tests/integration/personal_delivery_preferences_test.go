package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

// TestDirectMessageDeliveryPreferences проверяет авторитетные ограничения
// очереди сообщений в отдельной случайной БД: mute сохраняет сообщение и unread,
// очистка/скрытие не возвращают старую историю, а настройки не затрагивают второго участника.
// @args: t — контекст проверки и очистки изолированной БД.
func TestDirectMessageDeliveryPreferences(t *testing.T) {
	db := stageOneDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	accounts := pg.NewUserRepository(db)
	owner := users.User{ID: uuid.NewString(), Email: "delivery-owner@example.test", PasswordHash: "fixture-not-a-password"}
	peer := users.User{ID: uuid.NewString(), Email: "delivery-peer@example.test", PasswordHash: "fixture-not-a-password"}
	for _, user := range []users.User{owner, peer} {
		if _, err := accounts.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repo := pg.NewPersonalRepository(db)
	messages := pg.NewDirectChatRepository(db)
	conversation, _, err := repo.GetOrCreate(ctx, owner.ID, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	send := func(text string) chat.Message {
		t.Helper()
		message, created, err := messages.Send(ctx, owner.ID, conversation.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text}, strings.Repeat("a", 64))
		if err != nil || !created {
			t.Fatal("send fixture", err, created)
		}
		return message
	}
	check := func(user, message string, wantAllowed, wantNotifications bool, wantCutoff int64) {
		t.Helper()
		called := false
		allowed, err := repo.WithConversationMessageDelivery(ctx, conversation.ID, user, message, func(enabled bool, cutoff int64) error {
			called = true
			if enabled != wantNotifications || cutoff != wantCutoff {
				t.Errorf("delivery state: got enabled=%v cutoff=%d, want %v/%d", enabled, cutoff, wantNotifications, wantCutoff)
			}
			return nil
		})
		if err != nil || allowed != wantAllowed || called != wantAllowed {
			t.Fatal("delivery guard", allowed, called, err)
		}
	}
	first := send("initial")
	check(peer.ID, first.ID, true, true, 0)
	if _, err := repo.SetDirectPreferences(ctx, peer.ID, conversation.ID, false); err != nil {
		t.Fatal(err)
	}
	check(peer.ID, first.ID, true, false, 0)
	state, err := messages.ReadState(ctx, peer.ID, conversation.ID)
	if err != nil || state.UnreadCount != 1 {
		t.Fatal("mute changed unread", state, err)
	}
	if _, err := repo.ClearDirectHistory(ctx, peer.ID, conversation.ID); err != nil {
		t.Fatal(err)
	}
	check(peer.ID, first.ID, false, false, 0)
	check(owner.ID, first.ID, true, true, 0)
	second := send("after clear")
	check(peer.ID, second.ID, true, false, first.Sequence)
	check(peer.ID, uuid.NewString(), false, false, 0)
	if _, err := repo.HideDirectConversation(ctx, peer.ID, conversation.ID); err != nil {
		t.Fatal(err)
	}
	check(peer.ID, second.ID, false, false, 0)
	called := false
	allowed, err := repo.WithConversationDelivery(ctx, conversation.ID, peer.ID, func() error { called = true; return nil })
	if err != nil || !allowed || !called {
		t.Fatal("hidden lifecycle event cannot reach its owner", allowed, called, err)
	}
	called = false
	allowed, err = repo.WithDirectActionDelivery(ctx, conversation.ID, peer.ID, "conversation.hidden", func(item *personal.Conversation) error {
		called = true
		if item == nil || item.ID != conversation.ID || item.Type != "direct" || item.HistoryClearedThrough != second.Sequence || item.Peer != nil || item.Preview != "" || item.LastMessageID != nil {
			t.Errorf("hidden action lost its current cutoff or exposed metadata: %+v", item)
		}
		return nil
	})
	if err != nil || !allowed || !called {
		t.Fatal("fresh hidden action denied", allowed, called, err)
	}
	page, err := repo.List(ctx, peer.ID, "", 50)
	if err != nil || len(page.Items) != 0 || page.UnreadCount != 0 {
		t.Fatal("hidden conversation remained in list", page, err)
	}
	third := send("after hide")
	check(peer.ID, third.ID, true, false, second.Sequence)
	page, err = repo.List(ctx, peer.ID, "", 50)
	if err != nil || len(page.Items) != 1 || page.UnreadCount != 1 {
		t.Fatal("fresh message failed to restore chat/unread", page, err)
	}
	called = false
	allowed, err = repo.WithDirectActionDelivery(ctx, conversation.ID, peer.ID, "conversation.hidden", func(*personal.Conversation) error { called = true; return nil })
	if err != nil || allowed || called {
		t.Fatal("old hidden event hid a chat restored by a new message", allowed, called, err)
	}
	if _, err := repo.SetDirectPreferences(ctx, peer.ID, conversation.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"conversation.preferences.updated", "conversation.history.cleared"} {
		allowed, err = repo.WithDirectActionDelivery(ctx, conversation.ID, peer.ID, kind, func(item *personal.Conversation) error {
			if item == nil || !item.NotificationsEnabled || item.HistoryClearedThrough != second.Sequence || item.Preview != third.Text || item.LastMessageID == nil || *item.LastMessageID != third.ID {
				t.Errorf("out-of-order %s did not use latest state: %+v", kind, item)
			}
			return nil
		})
		if err != nil || !allowed {
			t.Fatal("latest personal action projection unavailable", allowed, err)
		}
	}

	// Проверяется реальное ожидание блокировки PostgreSQL, а не задержка по таймеру.
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	defer unlock()
	delivery := make(chan error, 1)
	go func() {
		_, err := repo.WithConversationMessageDelivery(ctx, conversation.ID, peer.ID, third.ID, func(bool, int64) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		delivery <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("delivery did not enter", ctx.Err())
	}
	cleared := make(chan error, 1)
	go func() { _, err := repo.ClearDirectHistory(ctx, peer.ID, conversation.ID); cleared <- err }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := false
	for !waiting {
		select {
		case err := <-cleared:
			t.Fatal("clear committed before bounded delivery ended", err)
		case <-ctx.Done():
			t.Fatal("clear lock wait not observed", ctx.Err())
		case <-ticker.C:
			var count int64
			if err := db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&count).Error; err != nil {
				t.Fatal(err)
			}
			waiting = count > 0
		}
	}
	unlock()
	if err := <-delivery; err != nil {
		t.Fatal(err)
	}
	if err := <-cleared; err != nil {
		t.Fatal(err)
	}
	check(peer.ID, third.ID, false, false, 0)
}
