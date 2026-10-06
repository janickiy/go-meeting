package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	personalapp "github.com/janickiy/go-recorder/internal/app/personal"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"gorm.io/gorm"
)

func groupAccount(t *testing.T, f *stageTwoFixture, name string) users.User {
	t.Helper()
	user := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@group.test", PasswordHash: "unused", DisplayName: ptr(name)}
	if _, err := pg.NewUserRepository(f.db).Create(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}
func groupSend(t *testing.T, repo *pg.ChatRepository, actor, id, text string) chat.Message {
	t.Helper()
	req, fp, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text})
	if err != nil {
		t.Fatal(err)
	}
	m, created, err := repo.Send(context.Background(), actor, id, req, fp)
	if err != nil || !created {
		t.Fatalf("send: %+v %v", m, err)
	}
	return m
}

func TestGroupConversationsLifecycleAndAuthorization(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	cara := groupAccount(t, f, "Cara")
	dana := groupAccount(t, f, "Dana")
	req := personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "  Команда  ", Description: "Общее обсуждение", MemberIDs: []string{f.owner.ID, f.member.ID, f.member.ID}}
	item, created, err := repo.CreateGroup(ctx, f.owner.ID, req)
	if err != nil || !created || item.Type != "group" || item.Peer != nil || item.Name != "Команда" || item.MemberCount != 2 || item.MyRole != personal.Owner || item.CreatedBy == nil || *item.CreatedBy != f.owner.ID {
		t.Fatalf("create: %+v %v", item, err)
	}
	var failures [12]error
	var results [12]personal.Conversation
	runConcurrent(12, func(i int) { results[i], _, failures[i] = repo.CreateGroup(ctx, f.owner.ID, req) })
	for i, err := range failures {
		if err != nil || results[i].ID != item.ID {
			t.Fatalf("create race: %v %+v", err, results[i])
		}
	}
	changed := req
	changed.Name = "Different"
	if _, _, err = repo.CreateGroup(ctx, f.owner.ID, changed); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("changed repeated create accepted", err)
	}
	if _, err = repo.Get(ctx, dana.ID, item.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("outsider detail allowed", err)
	}
	newName := "Редактирование"
	if _, err = repo.UpdateGroup(ctx, f.member.ID, item.ID, personal.UpdateGroupRequest{Name: &newName}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("ordinary member edited metadata", err)
	}
	if _, _, err = repo.AddGroupMembers(ctx, f.member.ID, item.ID, []string{cara.ID}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("ordinary member added member", err)
	}
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, item.ID, []string{cara.ID, cara.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ChangeGroupRole(ctx, f.owner.ID, item.ID, cara.ID, personal.Admin); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ChangeGroupRole(ctx, cara.ID, item.ID, f.member.ID, personal.Admin); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("admin promoted member", err)
	}
	if _, _, err = repo.RemoveGroupMember(ctx, cara.ID, item.ID, f.owner.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("admin removed owner", err)
	}
	if _, err = repo.ChangeGroupRole(ctx, f.owner.ID, item.ID, f.member.ID, personal.Admin); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.RemoveGroupMember(ctx, cara.ID, item.ID, f.member.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("admin removed admin", err)
	}
	if _, err = repo.ChangeGroupRole(ctx, f.owner.ID, item.ID, f.member.ID, personal.MemberRole); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.AddGroupMembers(ctx, cara.ID, item.ID, []string{dana.ID}); err != nil {
		t.Fatal("admin could not add ordinary member", err)
	}
	if _, _, err = repo.RemoveGroupMember(ctx, cara.ID, item.ID, dana.ID); err != nil {
		t.Fatal("admin could not remove ordinary member", err)
	}
	first := groupSend(t, messages, f.owner.ID, item.ID, "Первое сообщение")
	if first.SenderName != "Alice" || first.ConversationID != item.ID || first.ConferenceID != "" {
		t.Fatal("shared message projection", first)
	}
	reply, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Ответ", ReplyTo: first.ID})
	response, _, err := messages.Send(ctx, f.member.ID, item.ID, reply, fp)
	if err != nil || response.ReplyPreview == nil || response.ReplyPreview.ID != first.ID {
		t.Fatal("group reply failed", err, response)
	}
	if _, err = messages.Edit(ctx, cara.ID, item.ID, first.ID, "forged"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("admin edited another author's message", err)
	}
	if _, err = messages.Delete(ctx, cara.ID, item.ID, first.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("admin deleted another author's message", err)
	}
	state, err := messages.MarkRead(ctx, f.member.ID, item.ID, response.ID)
	if err != nil {
		t.Fatal(err)
	}
	frontier := state.LastReadSequence
	second := groupSend(t, messages, f.owner.ID, item.ID, "После прочтения")
	newMember := groupAccount(t, f, "New member")
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, item.ID, []string{newMember.ID}); err != nil {
		t.Fatal(err)
	}
	newState, err := messages.ReadState(ctx, newMember.ID, item.ID)
	if err != nil || newState.UnreadCount != 0 || newState.LastReadMessageID == nil || *newState.LastReadMessageID != second.ID {
		t.Fatal("newjoin frontier", err, newState)
	}
	page, err := messages.List(ctx, newMember.ID, item.ID, "", 50)
	if err != nil || len(page.Items) != 3 {
		t.Fatal("joined member lacks full history", err, page)
	}
	if _, _, err = repo.RemoveGroupMember(ctx, f.owner.ID, item.ID, f.member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = messages.List(ctx, f.member.ID, item.ID, "", 50); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed history accessible", err)
	}
	if _, _, err = messages.Send(ctx, f.member.ID, item.ID, reply, fp); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed repeat send accessible", err)
	}
	if _, err = messages.MarkRead(ctx, f.member.ID, item.ID, first.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed read accessible", err)
	}
	if _, err = messages.Edit(ctx, f.member.ID, item.ID, response.ID, "forged"); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed edit accessible", err)
	}
	if _, err = messages.Delete(ctx, f.member.ID, item.ID, response.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed delete accessible", err)
	}
	if _, _, err = messages.InitAttachment(ctx, f.member.ID, item.ID, chat.InitRequest{ClientRequestID: uuid.NewString(), Filename: "a.txt", MimeType: "text/plain", Size: 1}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("removed upload accessible", err)
	}
	active, err := repo.HasActiveMembership(ctx, item.ID, f.member.ID)
	if err != nil || active {
		t.Fatal("removed global delivery guard", err)
	}
	list, err := repo.List(ctx, f.member.ID, "", 50)
	if err != nil || len(list.Items) != 0 || list.UnreadCount != 0 {
		t.Fatal("removed list/unread leak", err, list)
	}
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, item.ID, []string{f.member.ID}); err != nil {
		t.Fatal(err)
	}
	state, err = messages.ReadState(ctx, f.member.ID, item.ID)
	if err != nil || state.LastReadSequence != frontier || state.UnreadCount != 1 {
		t.Fatal("returning read cursor lost", err, state)
	}
	if err = repo.LeaveGroup(ctx, f.owner.ID, item.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("owner left without transfer", err)
	}
	item, err = repo.TransferGroupOwnership(ctx, f.owner.ID, item.ID, cara.ID)
	if err != nil || item.MyRole != personal.Admin {
		t.Fatal("transfer failed", err, item)
	}
	if _, err = repo.DeleteGroup(ctx, f.owner.ID, item.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("old owner deleted group", err)
	}
	former, err := repo.DeleteGroup(ctx, cara.ID, item.ID)
	if err != nil || len(former) != 4 {
		t.Fatal("delete failed", err, former)
	}
	for _, user := range former {
		if _, err = repo.Get(ctx, user, item.ID); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("deleted group accessible", user, err)
		}
		if active, err = repo.HasActiveMembership(ctx, item.ID, user); err != nil || active {
			t.Fatal("deleted group active", err)
		}
	}
	var count int64
	if err = f.db.Table("conversation_messages").Where("conversation_id=?", item.ID).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("group deletion destroyed history", err, count)
	}
}

func TestGroupConstraintsFiltersAndMembershipBounds(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	direct, _, err := repo.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Group search", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Another group"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE conversation_members SET role='member' WHERE conversation_id=? AND user_id=?", group.ID, f.owner.ID).Error; err == nil {
		t.Fatal("DB accepted ownerless live group")
	}
	if err = f.db.Exec("UPDATE conversation_members SET role='owner' WHERE conversation_id=? AND user_id=?", group.ID, f.member.ID).Error; err == nil {
		t.Fatal("DB accepted two owners")
	}
	if err = f.db.Exec("UPDATE conversation_members SET left_at=clock_timestamp() WHERE conversation_id=? AND user_id=?", direct.ID, f.member.ID).Error; err == nil {
		t.Fatal("direct membership lifecycle altered")
	}
	scope := f.conference.ID
	guest := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@guest.group", PasswordHash: "unused", GuestConferenceID: &scope}
	if _, err = pg.NewUserRepository(f.db).Create(ctx, guest); err != nil {
		t.Fatal(err)
	}
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, group.ID, []string{guest.ID}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("guest added", err)
	}
	if err = f.db.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?)", group.ID, guest.ID).Error; err == nil {
		t.Fatal("DB accepted guest membership")
	}
	first := groupSend(t, messages, f.owner.ID, group.ID, "group content")
	r, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "cross", ReplyTo: first.ID})
	if _, _, err = messages.Send(ctx, f.owner.ID, other.ID, r, fp); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("cross group reply accepted", err)
	}
	list, err := repo.ListFiltered(ctx, f.member.ID, "", 1, personal.ListFilter{Type: "group", UnreadOnly: true, Search: "search"})
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != group.ID || list.Items[0].LastSender == nil || list.Items[0].LastSender.ID != f.owner.ID || list.UnreadCount != 1 {
		t.Fatal("group projection/filter", err, list)
	}
	directList, err := repo.ListFiltered(ctx, f.owner.ID, "", 50, personal.ListFilter{Type: "direct"})
	if err != nil || len(directList.Items) != 1 || directList.Items[0].Peer == nil || directList.Items[0].Peer.ID != f.member.ID {
		t.Fatal("direct compatibility", err, directList)
	}
	page, err := repo.ListFiltered(ctx, f.owner.ID, "", 1, personal.ListFilter{Type: "group"})
	if err != nil || page.NextCursor == "" {
		t.Fatal("pagination absent", err, page)
	}
	for _, filter := range []personal.ListFilter{{Type: "direct"}, {Type: "group", UnreadOnly: true}, {Type: "group", Search: "different"}} {
		if _, err = repo.ListFiltered(ctx, f.owner.ID, page.NextCursor, 1, filter); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatal("cursor escaped filter", filter, err)
		}
	}
	if _, err = repo.ListFiltered(ctx, f.member.ID, page.NextCursor, 1, personal.ListFilter{Type: "group"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("cursor escaped account", err)
	}
	ids := []string{}
	for i := 0; i < 98; i++ {
		ids = append(ids, groupAccount(t, f, "Bounded member").ID)
	}
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, group.ID, ids); err != nil {
		t.Fatal("100 boundary denied", err)
	}
	extra := groupAccount(t, f, "Overflow")
	if _, _, err = repo.AddGroupMembers(ctx, f.owner.ID, group.ID, []string{extra.ID}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("101st member accepted", err)
	}
	if err = f.db.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?)", group.ID, extra.ID).Error; err == nil {
		t.Fatal("DB accepted 101st member")
	}
	if err = repo.LeaveGroup(ctx, f.owner.ID, other.ID); err != nil {
		t.Fatal("last owner cannot leave", err)
	}
	if _, err = repo.Get(ctx, f.owner.ID, other.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("last-owner group left accessible")
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "database", "migrations", "000030_group_conversations.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(down)).Error; err == nil {
		t.Fatal("down migration discarded existing groups")
	}
}

func TestGroupRevocationWaiterRechecksMembership(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewPersonalRepository(f.db)
	messages := pg.NewDirectChatRepository(f.db)
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Race", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	tx := f.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err = tx.Exec("SELECT id FROM conversations WHERE id=? FOR UPDATE", group.ID).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	req, fp, _ := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Must not persist after revoke"})
	go func() { _, _, err := messages.Send(ctx, f.member.ID, group.ID, req, fp); done <- err }()
	wait := time.Now().Add(3 * time.Second)
	for {
		var n int64
		if err = f.db.Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query ILIKE '%conversations%'`).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			break
		}
		if time.Now().After(wait) {
			t.Fatal("send did not wait for conversation lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = tx.Exec("UPDATE conversation_members SET left_at=clock_timestamp() WHERE conversation_id=? AND user_id=?", group.ID, f.member.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec("UPDATE conversations SET metadata_version=metadata_version+1,updated_at=clock_timestamp() WHERE id=?", group.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("waiting send retained stale access", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("send failed to unblock")
	}
	var n int64
	if err = f.db.Table("conversation_messages").Where("conversation_id=?", group.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("unauthorized message persisted", err, n)
	}
	user := groupAccount(t, f, "Concurrent add")
	var wg sync.WaitGroup
	var added [10]int
	var failures [10]error
	for i := range added {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, rows, err := repo.AddGroupMembers(ctx, f.owner.ID, group.ID, []string{user.ID})
			added[i] = len(rows)
			failures[i] = err
		}(i)
	}
	wg.Wait()
	total := 0
	for i, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
		total += added[i]
	}
	if total != 1 {
		t.Fatal("duplicate add raced", total)
	}
}

func TestGroupWriteResponseSurvivesConcurrentRevocation(t *testing.T) {
	f := stageTwo(t)
	// This SQL-hook test does not use conference realtime. Join the fixture's
	// janitors before mutating GORM's shared callback registry.
	for _, hub := range f.hubs {
		hub.Shutdown()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo := pg.NewPersonalRepository(f.db)
	group, _, err := repo.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Before", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ChangeGroupRole(ctx, f.owner.ID, group.ID, f.member.ID, personal.Admin); err != nil {
		t.Fatal(err)
	}
	// Pause the actual DTO read. Removal must wait for this committed response
	// snapshot, rather than make a successful write return forbidden afterwards.
	projection, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unlock := func() { once.Do(func() { close(release) }) }
	defer unlock()
	var reached sync.Once
	const callback = "group_write_response_race"
	if err = f.db.Callback().Row().Before("gorm:row").Register(callback, func(db *gorm.DB) {
		if strings.HasPrefix(db.Statement.SQL.String(), "SELECT c.id,c.type") {
			reached.Do(func() {
				close(projection)
				select {
				case <-release:
				case <-ctx.Done():
					db.AddError(ctx.Err())
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer f.db.Callback().Row().Remove(callback)
	type writeResult struct {
		item personal.Conversation
		err  error
	}
	result := make(chan writeResult, 1)
	name := "Committed 👩‍💻"
	go func() {
		item, err := repo.UpdateGroup(ctx, f.member.ID, group.ID, personal.UpdateGroupRequest{Name: &name})
		result <- writeResult{item, err}
	}()
	select {
	case <-projection:
	case <-ctx.Done():
		t.Fatal("write response projection was not reached", ctx.Err())
	}
	removal := make(chan error, 1)
	go func() { _, _, err := repo.RemoveGroupMember(ctx, f.owner.ID, group.ID, f.member.ID); removal <- err }()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := false
	for !waiting {
		select {
		case err := <-removal:
			t.Fatal("revocation overtook committed write response", err)
		case <-ctx.Done():
			t.Fatal("revocation did not wait on response lock", ctx.Err())
		case <-ticker.C:
			var count int64
			if err = f.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&count).Error; err != nil {
				t.Fatal(err)
			}
			waiting = count > 0
		}
	}
	unlock()
	written := <-result
	if written.err != nil || written.item.Name != name || written.item.MyRole != personal.Admin {
		t.Fatal("committed write response was lost", written.item, written.err)
	}
	if err = <-removal; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, f.member.ID, group.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("revoked actor can still fetch group", err)
	}
	ownerView, err := repo.Get(ctx, f.owner.ID, group.ID)
	if err != nil || ownerView.Name != name {
		t.Fatal("successful write did not persist", ownerView, err)
	}
}

type groupUnavailablePresence struct{}

func (groupUnavailablePresence) Online(context.Context, []string) (map[string]bool, error) {
	return nil, errors.New("isolated presence unavailable")
}

func TestGroupHTTPContractsAndAuthoritativeRoles(t *testing.T) {
	f := stageTwo(t)
	repo := pg.NewPersonalRepository(f.db)
	router := gin.New()
	handler := &personalapp.Handler{Repo: repo, Presence: groupUnavailablePresence{}}
	httptransport.RegisterPersonalRoutes(router, handler, chatapp.NewHandler(nil).ForConversations(), middleware.Authenticate(f.tokens), nil)
	api := stageOneAPI{router: router}
	req := personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "HTTP group", MemberIDs: []string{f.member.ID}}
	api.expect(t, "POST", "/conversations/group", "", req, 401, nil)
	var result struct{ Item personal.Conversation }
	w := api.expect(t, "POST", "/conversations/group", f.ownerToken, req, 201, &result)
	if w.Header().Get("Cache-Control") != "private, no-store" || result.Item.MemberCount != 2 || strings.Contains(w.Body.String(), "avatar_key") || strings.Contains(w.Body.String(), "password") {
		t.Fatal("unsafe group response", w.Body.String())
	}
	id := result.Item.ID
	path := "/conversations/" + id
	api.expect(t, "POST", "/conversations/group", f.ownerToken, req, 200, nil)
	api.expect(t, "PATCH", path, f.memberToken, map[string]any{"name": "forged"}, 403, nil)
	api.expect(t, "PATCH", path, f.ownerToken, map[string]any{"avatar_key": "forged"}, 400, nil)
	api.expect(t, "PATCH", path+"/members/"+f.member.ID, f.memberToken, map[string]any{"role": "admin"}, 403, nil)
	api.expect(t, "PATCH", path+"/members/"+f.member.ID, f.ownerToken, map[string]any{"role": "owner"}, 422, nil)
	var members struct{ Items []personal.Member }
	api.expect(t, "GET", path+"/members", f.memberToken, nil, 200, &members)
	if len(members.Items) != 2 {
		t.Fatal("members lost on Redis error")
	}
	for _, m := range members.Items {
		if m.Online != nil {
			t.Fatal("presence outage became false offline")
		}
	}
	api.expect(t, "GET", "/conversations?unreadOnly=garbage", f.ownerToken, nil, 422, nil)
	api.expect(t, "POST", path+"/leave", f.ownerToken, map[string]any{}, 409, nil)
	api.expect(t, "POST", path+"/ownership", f.memberToken, map[string]any{"userId": f.owner.ID}, 403, nil)
	api.expect(t, "DELETE", path+"/members/"+f.member.ID, f.ownerToken, nil, 200, nil)
	api.expect(t, "GET", path+"/members", f.memberToken, nil, 403, nil)
	api.expect(t, "GET", path, f.memberToken, nil, 403, nil)
	api.expect(t, "DELETE", path, f.memberToken, nil, 403, nil)
	api.expect(t, "DELETE", path, f.ownerToken, nil, 200, nil)
	api.expect(t, "GET", path, f.ownerToken, nil, 403, nil)
}
