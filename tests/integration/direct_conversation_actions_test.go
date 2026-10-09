package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/folders"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
)

// TestDirectConversationActionsPersonalVisibility проверяет историю и вложения в изолированной схеме.
// @args: t — контекст теста; fixture пропускается без явно заданных локальных PostgreSQL и Redis.
func TestDirectConversationActionsPersonalVisibility(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	item, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := groupSend(t, messages, f.member.ID, item.ID, "Прежнее сообщение с файлом")
	attachmentID, requestID := uuid.NewString(), uuid.NewString()
	if err = f.db.Exec(`INSERT INTO conversation_attachments(id,conversation_id,owner_user_id,client_request_id,filename,mime_type,size,status,message_id,expires_at)
	 VALUES(?,?,?,?,'document.txt','text/plain',3,'attached',?,?)`, attachmentID, item.ID, f.member.ID, requestID, first.ID, time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	before, err := repo.Get(ctx, f.owner.ID, item.ID)
	if err != nil || before.UnreadCount != 1 || before.Preview == "" {
		t.Fatal("нет исходной проекции", before, err)
	}
	cleared, err := repo.ClearDirectHistory(ctx, f.owner.ID, item.ID)
	if err != nil || cleared.HistoryClearedThrough != first.Sequence || cleared.UnreadCount != 0 || cleared.Preview != "" || cleared.LastMessageID != nil || cleared.LastMessageAt != nil || cleared.LastSender != nil {
		t.Fatal("очистка раскрыла старое превью", cleared, err)
	}
	page, err := messages.List(ctx, f.owner.ID, item.ID, "", 50)
	if err != nil || len(page.Items) != 0 || page.NextCursor != "" || page.UnreadCount != 0 || page.LastReadMessageID != nil {
		t.Fatal("старые сообщения доступны", page, err)
	}
	peerPage, err := messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || len(peerPage.Items) != 1 || len(peerPage.Items[0].Attachments) != 1 {
		t.Fatal("очистка изменила данные собеседника", peerPage, err)
	}
	if _, err = messages.DownloadAttachment(ctx, f.owner.ID, item.ID, attachmentID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("старое вложение доступно", err)
	}
	if _, err = messages.DownloadAttachment(ctx, f.member.ID, item.ID, attachmentID); err != nil {
		t.Fatal("собеседник потерял вложение", err)
	}
	if _, err = messages.Edit(ctx, f.owner.ID, item.ID, first.ID, "изменение"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("очищенное сообщение доступно через edit", err)
	}
	if _, err = messages.Delete(ctx, f.owner.ID, item.ID, first.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("очищенное сообщение доступно через delete", err)
	}
	if _, err = messages.MarkRead(ctx, f.owner.ID, item.ID, first.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("очищенное сообщение доступно через read", err)
	}
	replyRequest, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Ответ", ReplyTo: first.ID})
	if _, _, err = messages.Send(ctx, f.owner.ID, item.ID, replyRequest, fp); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("ответ на очищенную историю доступен", err)
	}
	peerReply, _, err := messages.Send(ctx, f.member.ID, item.ID, replyRequest, fp)
	if err != nil || peerReply.ReplyPreview == nil || peerReply.ReplyPreview.Sequence != first.Sequence {
		t.Fatal("собеседник не может ответить на свою историю", peerReply, err)
	}
	page, err = messages.List(ctx, f.owner.ID, item.ID, "", 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != peerReply.ID || page.Items[0].ReplyPreview != nil || page.Items[0].ReplyTo != nil {
		t.Fatal("исходное сообщение раскрыто через ответ", page, err)
	}
	peerPage, err = messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || len(peerPage.Items) != 2 || peerPage.Items[1].ReplyPreview == nil {
		t.Fatal("превью собеседника исчезло", peerPage, err)
	}
	list, err := repo.List(ctx, f.owner.ID, "", 50)
	if err != nil || len(list.Items) != 1 || list.UnreadCount != 1 {
		t.Fatal("новое сообщение не появилось", list, err)
	}
	// Повтор сохранённого запроса не создаёт сообщение заново и не возвращает очищенный текст.
	oldRequest, oldFP, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Сохранённый запрос"})
	old, _, err := messages.Send(ctx, f.owner.ID, item.ID, oldRequest, oldFP)
	if err != nil {
		t.Fatal(err)
	}
	ownedAttachmentID, ownedRequestID := uuid.NewString(), uuid.NewString()
	if err = f.db.Exec(`INSERT INTO conversation_attachments(id,conversation_id,owner_user_id,client_request_id,filename,mime_type,size,status,message_id,expires_at)
	 VALUES(?,?,?,?,'own.txt','text/plain',3,'attached',?,?)`, ownedAttachmentID, item.ID, f.owner.ID, ownedRequestID, old.ID, time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ClearDirectHistory(ctx, f.owner.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = messages.InitAttachment(ctx, f.owner.ID, item.ID, chat.InitRequest{ClientRequestID: ownedRequestID, Filename: "own.txt", MimeType: "text/plain", Size: 3}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("повтор загрузки раскрыл очищенное вложение", err)
	}
	if _, created, err := messages.Send(ctx, f.owner.ID, item.ID, oldRequest, oldFP); !errors.Is(err, apperrors.ErrNotFound) || created {
		t.Fatal("повтор запроса раскрыл очищенное сообщение", created, err)
	}
	var count int64
	if err = f.db.Table("conversation_messages").Where("id=?", old.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("общие данные удалены", count, err)
	}
}

// TestDirectConversationHideAndPreferences проверяет персональность скрытия и уведомлений, включая папки.
// @args: t — контекст проверки с изолированной схемой.
func TestDirectConversationHideAndPreferences(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	item, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	first := groupSend(t, messages, f.member.ID, item.ID, "До удаления чата")
	ownerView, err := repo.SetDirectPreferences(ctx, f.owner.ID, item.ID, false)
	if err != nil || ownerView.NotificationsEnabled {
		t.Fatal("настройка не сохранена", ownerView, err)
	}
	peerView, err := repo.Get(ctx, f.member.ID, item.ID)
	if err != nil || !peerView.NotificationsEnabled {
		t.Fatal("настройка изменилась у собеседника", peerView, err)
	}
	folderRepo := pg.NewFolderRepository(f.db)
	folder, err := folderRepo.Create(ctx, f.owner.ID, "Персональная папка")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = folderRepo.SetItem(ctx, f.owner.ID, folder.ID, "conversation", item.ID, true); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		cutoff, err := repo.HideDirectConversation(ctx, f.owner.ID, item.ID)
		if err != nil || cutoff != first.Sequence {
			t.Fatal("повтор скрытия или граница", cutoff, err)
		}
	}
	list, err := repo.List(ctx, f.owner.ID, "", 50)
	if err != nil || len(list.Items) != 0 || list.UnreadCount != 0 {
		t.Fatal("скрытый чат есть в списке", list, err)
	}
	if _, err = repo.Get(ctx, f.owner.ID, item.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("скрытый чат доступен напрямую", err)
	}
	if _, err = messages.List(ctx, f.owner.ID, item.ID, "", 50); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("скрытая история доступна", err)
	}
	contents, err := folderRepo.Items(ctx, f.owner.ID, folder.ID, "", 50, folders.Filter{})
	if err != nil || len(contents.Items) != 0 {
		t.Fatal("скрытый чат есть в папке", contents, err)
	}
	folder, err = folderRepo.Get(ctx, f.owner.ID, folder.ID)
	if err != nil || folder.ItemCount != 0 {
		t.Fatal("скрытый чат есть в счётчиках", folder, err)
	}
	peerPage, err := messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || len(peerPage.Items) != 1 {
		t.Fatal("удаление чата повлияло на собеседника", peerPage, err)
	}
	newMessage := groupSend(t, messages, f.member.ID, item.ID, "Новая переписка")
	ownerView, err = repo.Get(ctx, f.owner.ID, item.ID)
	if err != nil || ownerView.HistoryClearedThrough != first.Sequence || ownerView.NotificationsEnabled || ownerView.LastMessageID == nil || *ownerView.LastMessageID != newMessage.ID {
		t.Fatal("новое сообщение не восстановило персональный чат", ownerView, err)
	}
	page, err := messages.List(ctx, f.owner.ID, item.ID, "", 50)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != newMessage.ID {
		t.Fatal("восстановление раскрыло старую историю", page, err)
	}
	contents, err = folderRepo.Items(ctx, f.owner.ID, folder.ID, "", 50, folders.Filter{})
	if err != nil || len(contents.Items) != 1 {
		t.Fatal("восстановленный чат не появился в папке", contents, err)
	}
	if _, err = repo.HideDirectConversation(ctx, f.owner.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	reopened, created, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil || created || reopened.ID != item.ID || reopened.HistoryClearedThrough != newMessage.Sequence || reopened.Preview != "" {
		t.Fatal("явное открытие восстановило старую историю", reopened, created, err)
	}
}

// TestDirectConversationActionsRoutesAndAuthorization проверяет авторизацию и неизменность группового API.
// @args: t — контекст теста API с изолированным хранилищем.
func TestDirectConversationActionsRoutesAndAuthorization(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	item, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages := pg.NewDirectChatRepository(f.db)
	first := groupSend(t, messages, f.member.ID, item.ID, "История перед очисткой")
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Группа", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	outsider := groupAccount(t, f, "Посторонний")
	outsideToken, _ := f.tokens.Issue(outsider.ID)
	router := gin.New()
	httptransport.RegisterPersonalRoutes(router, &personalapp.Handler{Repo: repo}, chatapp.NewHandler(nil).ForConversations(), middleware.Authenticate(f.tokens), nil)
	api := stageOneAPI{router: router}
	path := "/conversations/" + item.ID
	for _, action := range []struct {
		method, suffix string
		body           any
	}{
		{"PATCH", "/preferences", map[string]any{"notificationsEnabled": false}},
		{"POST", "/clear-history", nil}, {"POST", "/hide", nil},
	} {
		api.expect(t, action.method, path+action.suffix, "", action.body, 401, nil)
		api.expect(t, action.method, path+action.suffix, outsideToken, action.body, 403, nil)
		api.expect(t, action.method, "/conversations/"+group.ID+action.suffix, f.ownerToken, action.body, 403, nil)
	}
	api.expect(t, "PATCH", path+"/preferences", f.ownerToken, map[string]any{}, 422, nil)
	api.expect(t, "PATCH", path+"/preferences", f.ownerToken, map[string]any{"notificationsEnabled": false, "userId": f.member.ID}, 400, nil)
	var response struct{ Item personal.Conversation }
	api.expect(t, "PATCH", path+"/preferences", f.ownerToken, map[string]any{"notificationsEnabled": false}, 200, &response)
	if response.Item.NotificationsEnabled {
		t.Fatal("API не вернул отключённые уведомления")
	}
	api.expect(t, "POST", path+"/clear-history", f.ownerToken, nil, 200, &response)
	if response.Item.HistoryClearedThrough != first.Sequence {
		t.Fatal("нет подтверждённой границы очистки", response.Item)
	}
	latest := groupSend(t, messages, f.member.ID, item.ID, "Сообщение между очисткой и скрытием")
	for attempt := 0; attempt < 2; attempt++ {
		var receipt struct {
			Hidden                bool
			HistoryClearedThrough int64
		}
		result := api.expect(t, "POST", path+"/hide", f.ownerToken, nil, 200, &receipt)
		if !receipt.Hidden || receipt.HistoryClearedThrough != latest.Sequence || receipt.HistoryClearedThrough < response.Item.HistoryClearedThrough {
			t.Fatal("повтор скрытия не вернул точную персональную границу", receipt, latest)
		}
		var fields map[string]any
		if err := json.Unmarshal(result.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 {
			t.Fatal("скрытие раскрыло лишнюю информацию", fields)
		}
	}
	api.expect(t, "DELETE", path, f.ownerToken, nil, 403, nil)
	if _, err = repo.Get(ctx, f.member.ID, item.ID); err != nil {
		t.Fatal("peer потерял доступ", err)
	}
	peerPage, err := messages.List(ctx, f.member.ID, item.ID, "", 50)
	if err != nil || len(peerPage.Items) != 2 {
		t.Fatal("ответ скрытия изменил историю собеседника", peerPage, err)
	}
	api.expect(t, "DELETE", "/conversations/"+group.ID, f.memberToken, nil, 403, nil)
	api.expect(t, "DELETE", "/conversations/"+group.ID, f.ownerToken, nil, 200, nil)
}

// TestDirectConversationClearSendRace проверяет атомарную границу истории при конкурентной отправке.
// @args: t — контекст конкурентной проверки PostgreSQL.
func TestDirectConversationClearSendRace(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	item, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 12; attempt++ {
		var sendErr, clearErr error
		var sent chat.Message
		runConcurrent(2, func(i int) {
			if i == 0 {
				_, clearErr = repo.ClearDirectHistory(ctx, f.owner.ID, item.ID)
				return
			}
			req, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Конкурентное сообщение"})
			sent, _, sendErr = messages.Send(ctx, f.member.ID, item.ID, req, fp)
		})
		if sendErr != nil || clearErr != nil {
			t.Fatal("конкурентные действия", sendErr, clearErr)
		}
		view, err := repo.Get(ctx, f.owner.ID, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		page, err := messages.List(ctx, f.owner.ID, item.ID, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range page.Items {
			if message.Sequence <= view.HistoryClearedThrough {
				t.Fatal("граница истории нарушена", message, view)
			}
		}
		if sent.Sequence > view.HistoryClearedThrough && (len(page.Items) != 1 || page.Items[0].ID != sent.ID) {
			t.Fatal("новое сообщение потеряно", sent, page, view)
		}
	}
}

// TestDirectConversationPreferencesRollbackGuard проверяет безопасный откат только в случайной тестовой базе.
// @args: t — контекст теста; без явно заданной локальной PostgreSQL проверка пропускается.
func TestDirectConversationPreferencesRollbackGuard(t *testing.T) {
	db := stageOneDatabase(t)
	ctx := context.Background()
	userRepo := pg.NewUserRepository(db)
	owner := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@rollback.test", PasswordHash: "unused", DisplayName: ptr("Владелец")}
	peer := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@rollback.test", PasswordHash: "unused", DisplayName: ptr("Собеседник")}
	for _, user := range []users.User{owner, peer} {
		if _, err := userRepo.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	repo := pg.NewPersonalRepository(db)
	conversation, _, err := repo.GetOrCreate(ctx, owner.ID, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	message := groupSend(t, pg.NewDirectChatRepository(db), peer.ID, conversation.ID, "Данные должны сохраниться")
	dir := filepath.Join("..", "..", "database", "migrations")
	down, err := os.ReadFile(filepath.Join(dir, "000033_direct_conversation_preferences.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, mutation string }{
		{"mute", "notifications_enabled=false"},
		{"cutoff", "history_cleared_through=1"},
		{"hidden", "hidden_at=clock_timestamp()"},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Только фикстура: сброс между независимыми вариантами не является процедурой отката продукта.
			if err := db.Exec("UPDATE conversation_members SET notifications_enabled=true,history_cleared_through=0,hidden_at=NULL WHERE conversation_id=?", conversation.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("UPDATE conversation_members SET "+test.mutation+" WHERE conversation_id=? AND user_id=?", conversation.ID, owner.ID).Error; err != nil {
				t.Fatal(err)
			}
			var before string
			if err := db.Raw("SELECT row_to_json(m)::text FROM conversation_members m WHERE conversation_id=? AND user_id=?", conversation.ID, owner.ID).Scan(&before).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(string(down)).Error; err == nil {
				t.Fatal("откат удалил нестандартные персональные настройки")
			}
			var columns int64
			if err := db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='conversation_members' AND column_name IN('notifications_enabled','history_cleared_through','hidden_at')").Scan(&columns).Error; err != nil || columns != 3 {
				t.Fatal("защита отката потеряла столбцы", columns, err)
			}
			var after string
			if err := db.Raw("SELECT row_to_json(m)::text FROM conversation_members m WHERE conversation_id=? AND user_id=?", conversation.ID, owner.ID).Scan(&after).Error; err != nil || after != before {
				t.Fatal("неуспешный откат изменил персональные данные", before, after, err)
			}
			var count int64
			if err := db.Table("conversation_messages").Where("id=?", message.ID).Count(&count).Error; err != nil || count != 1 {
				t.Fatal("защита отката изменила сообщение", count, err)
			}
		})
	}
	// Только после восстановления значений фикстуры по умолчанию откат становится допустимым.
	if err = db.Exec("UPDATE conversation_members SET notifications_enabled=true,history_cleared_through=0,hidden_at=NULL WHERE conversation_id=?", conversation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(down)).Error; err != nil {
		t.Fatal("пустые персональные настройки нельзя откатить", err)
	}
	var columns int64
	if err = db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='conversation_members' AND column_name IN('notifications_enabled','history_cleared_through','hidden_at')").Scan(&columns).Error; err != nil || columns != 0 {
		t.Fatal("откат не удалил новые столбцы", columns, err)
	}
	if err = db.Exec("DELETE FROM release_schema_migrations WHERE name='000033_direct_conversation_preferences.up.sql'").Error; err != nil {
		t.Fatal(err)
	}
	if err = pg.RunMigrations(db, dir); err != nil {
		t.Fatal("повторная миграция", err)
	}
	view, err := repo.Get(ctx, owner.ID, conversation.ID)
	if err != nil || !view.NotificationsEnabled || view.HistoryClearedThrough != 0 || view.LastMessageID == nil || *view.LastMessageID != message.ID {
		t.Fatal("откат и восстановление изменили общие данные", view, err)
	}
}
