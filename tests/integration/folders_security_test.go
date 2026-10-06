package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	foldersapp "github.com/janickiy/go-recorder/internal/app/folders"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	middleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	"gorm.io/gorm"
)

type folderView struct {
	ID, Name                                      string
	Position                                      int
	ItemCount, ConversationCount, ConferenceCount int64
	Contains                                      *bool
}
type folderItemView struct {
	Type     string
	Item     json.RawMessage
	InFolder *bool
}
type folderItemsPage struct {
	Items      []folderItemView
	NextCursor string
}
type folderFixture struct {
	db                                                *gorm.DB
	repo                                              *pg.FolderRepository
	personal                                          *pg.PersonalRepository
	conferences                                       *pg.ConferenceRepository
	conferenceService                                 *conferenceusecase.Service
	owner, member, outsider, guest                    users.User
	ownerToken, memberToken, outsideToken, guestToken string
	meeting                                           conferences.View
	direct, group                                     personal.Conversation
	api                                               stageOneAPI
}

func newFolderFixture(t *testing.T) *folderFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &folderFixture{db: stageOneDatabase(t)}
	ctx := context.Background()
	accounts := pg.NewUserRepository(f.db)
	f.owner = users.User{ID: uuid.NewString(), Email: "alice@folders.test", DisplayName: ptr("Alice"), PasswordHash: "unused"}
	f.member = users.User{ID: uuid.NewString(), Email: "bob@folders.test", DisplayName: ptr("Bob"), PasswordHash: "unused"}
	f.outsider = users.User{ID: uuid.NewString(), Email: "outsider@folders.test", DisplayName: ptr("Outsider"), PasswordHash: "unused"}
	for _, user := range []users.User{f.owner, f.member, f.outsider} {
		if _, err := accounts.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	tokens, err := security.NewTokenService(strings.Repeat("folders-integration-", 3))
	if err != nil {
		t.Fatal(err)
	}
	f.ownerToken, _ = tokens.Issue(f.owner.ID)
	f.memberToken, _ = tokens.Issue(f.member.ID)
	f.outsideToken, _ = tokens.Issue(f.outsider.ID)
	f.conferences = pg.NewConferenceRepository(f.db)
	f.conferenceService = conferenceusecase.NewService(f.conferences, accounts, security.GenerateInviteCode)
	f.meeting, err = f.conferenceService.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Folder meeting"})
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{f.owner.ID, f.member.ID} {
		if _, err = f.conferenceService.Join(ctx, actor, f.meeting.ID, conferences.JoinRequest{InviteCode: f.meeting.InviteCode}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.conferenceService.Transition(ctx, f.owner.ID, f.meeting.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	f.guest = users.User{ID: uuid.NewString(), Email: "guest@folders.invalid", PasswordHash: "unused", GuestConferenceID: &f.meeting.ID}
	if _, err = accounts.Create(ctx, f.guest); err != nil {
		t.Fatal(err)
	}
	f.guestToken, _ = tokens.IssueGuest(f.guest.ID, f.meeting.ID)
	f.personal = pg.NewPersonalRepository(f.db)
	f.direct, _, err = f.personal.GetOrCreate(ctx, f.owner.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.group, _, err = f.personal.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Folder group", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	f.repo = pg.NewFolderRepository(f.db)
	router := gin.New()
	httptransport.RegisterFolderRoutes(router, foldersapp.NewHandler(f.repo, nil), middleware.Authenticate(tokens), nil)
	f.api = stageOneAPI{router: router}
	return f
}

func (f *folderFixture) create(t *testing.T, token, name string) folderView {
	t.Helper()
	var result struct{ Item folderView }
	f.api.expect(t, "POST", "/folders", token, map[string]any{"name": name}, 201, &result)
	if result.Item.ID == "" || result.Item.Name == "" {
		t.Fatal("missing folder DTO", result)
	}
	return result.Item
}
func (f *folderFixture) add(t *testing.T, token, folderID, kind, id string) folderView {
	t.Helper()
	var result struct{ Item folderView }
	f.api.expect(t, "PUT", "/folders/"+folderID+"/items/"+kind+"/"+id, token, nil, 200, &result)
	return result.Item
}
func (f *folderFixture) get(t *testing.T, token, id string) folderView {
	t.Helper()
	var result struct{ Item folderView }
	f.api.expect(t, "GET", "/folders/"+id, token, nil, 200, &result)
	return result.Item
}
func (f *folderFixture) items(t *testing.T, token, path string) folderItemsPage {
	t.Helper()
	var result folderItemsPage
	f.api.expect(t, "GET", path, token, nil, 200, &result)
	return result
}
func folderItemID(t *testing.T, value folderItemView) string {
	t.Helper()
	var item struct{ ID string }
	if err := json.Unmarshal(value.Item, &item); err != nil || item.ID == "" {
		t.Fatal("invalid folder item", string(value.Item), err)
	}
	return value.Type + ":" + item.ID
}
func (f *folderFixture) raw(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	f.api.router.ServeHTTP(response, request)
	return response
}
func (f *folderFixture) denied(t *testing.T, method, path, token string, body any) {
	t.Helper()
	response := f.raw(t, method, path, token, body)
	if response.Code != 403 && response.Code != 404 {
		t.Fatalf("ownership denial %s %s: %d %s", method, path, response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "Private folder") || strings.Contains(response.Body.String(), "Folder group") {
		t.Fatal("denial leaked private metadata")
	}
}

func TestFoldersOwnershipMixedItemsAndOrdering(t *testing.T) {
	f := newFolderFixture(t)
	private := f.create(t, f.ownerToken, "Private folder")
	second := f.create(t, f.ownerToken, "Clients")
	memberFolder := f.create(t, f.memberToken, "Private folder")
	f.api.expect(t, "GET", "/folders", "", nil, 401, nil)
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/folders", nil}, {"POST", "/folders", map[string]any{"name": "Guest"}},
		{"GET", "/folder-items", nil}, {"PUT", "/folders/" + private.ID + "/items/conference/" + f.meeting.ID, nil},
	} {
		f.api.expect(t, request.method, request.path, f.guestToken, request.body, 403, nil)
	}
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/folders/" + private.ID, nil}, {"PATCH", "/folders/" + private.ID, map[string]any{"name": "forged"}},
		{"DELETE", "/folders/" + private.ID, nil}, {"GET", "/folders/" + private.ID + "/items", nil},
		{"PUT", "/folders/" + private.ID + "/items/conversation/" + f.direct.ID, nil},
		{"DELETE", "/folders/" + private.ID + "/items/conversation/" + f.direct.ID, nil},
		{"GET", "/folder-items?folderId=" + private.ID, nil},
	} {
		f.denied(t, request.method, request.path, f.memberToken, request.body)
	}
	f.denied(t, "PUT", "/folders/"+memberFolder.ID+"/items/conversation/"+f.group.ID, f.outsideToken, nil)
	f.denied(t, "PUT", "/folders/"+private.ID+"/items/conversation/"+uuid.NewString(), f.ownerToken, nil)
	f.denied(t, "PUT", "/folders/"+private.ID+"/items/conference/"+f.direct.ID, f.ownerToken, nil)
	f.api.expect(t, "PUT", "/folders/"+private.ID+"/items/recording/"+f.direct.ID, f.ownerToken, nil, 422, nil)
	for _, target := range []struct{ kind, id string }{{"conversation", f.direct.ID}, {"conversation", f.group.ID}, {"conference", f.meeting.ID}} {
		f.add(t, f.ownerToken, private.ID, target.kind, target.id)
		f.add(t, f.ownerToken, private.ID, target.kind, target.id)
	}
	item := f.get(t, f.ownerToken, private.ID)
	if item.ItemCount != 3 || item.ConversationCount != 2 || item.ConferenceCount != 1 {
		t.Fatal("duplicate mappings or mixed count", item)
	}
	f.add(t, f.ownerToken, second.ID, "conversation", f.direct.ID)
	var picker struct{ Items []folderView }
	f.api.expect(t, "GET", "/folders?itemKind=conversation&itemId="+f.direct.ID, f.ownerToken, nil, 200, &picker)
	if len(picker.Items) != 2 {
		t.Fatal("folder picker page", picker)
	}
	for _, folder := range picker.Items {
		if folder.Contains == nil || !*folder.Contains {
			t.Fatal("multi-folder checkbox omitted", folder)
		}
	}
	f.api.expect(t, "DELETE", "/folders/"+private.ID+"/items/conversation/"+f.direct.ID, f.ownerToken, nil, 200, nil)
	f.api.expect(t, "DELETE", "/folders/"+private.ID+"/items/conversation/"+f.direct.ID, f.ownerToken, nil, 200, nil)
	if f.get(t, f.ownerToken, private.ID).ItemCount != 2 || f.get(t, f.ownerToken, second.ID).ItemCount != 1 {
		t.Fatal("remove changed another folder")
	}
	path := "/folders/" + private.ID + "/items"
	if len(f.items(t, f.ownerToken, path+"?type=conversation").Items) != 1 || len(f.items(t, f.ownerToken, path+"?type=conference").Items) != 1 {
		t.Fatal("mixed type filters")
	}
	response := f.api.expect(t, "GET", path, f.ownerToken, nil, 200, nil)
	if response.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(response.Body.String(), f.meeting.InviteCode) || strings.Contains(response.Body.String(), "avatar_key") {
		t.Fatal("folder response exposed secret/cacheable data", response.Body.String())
	}
	f.api.expect(t, "PUT", "/folders/order", f.ownerToken, map[string]any{"ids": []string{second.ID, private.ID}}, 200, nil)
	var listed struct{ Items []folderView }
	f.api.expect(t, "GET", "/folders", f.ownerToken, nil, 200, &listed)
	if len(listed.Items) != 2 || listed.Items[0].ID != second.ID || listed.Items[1].ID != private.ID {
		t.Fatal("order not persisted", listed)
	}
	for _, ids := range [][]string{{private.ID}, {private.ID, private.ID}, {private.ID, memberFolder.ID}} {
		response := f.raw(t, "PUT", "/folders/order", f.ownerToken, map[string]any{"ids": ids})
		if response.Code < 400 || response.Code >= 500 {
			t.Fatal("invalid reorder accepted", ids, response.Code, response.Body.String())
		}
	}
	f.api.expect(t, "PATCH", "/folders/"+private.ID, f.ownerToken, map[string]any{"name": "Renamed"}, 200, nil)
	if f.get(t, f.ownerToken, private.ID).Name != "Renamed" || f.get(t, f.memberToken, memberFolder.ID).Name != "Private folder" {
		t.Fatal("rename leaked across accounts")
	}
	f.api.expect(t, "DELETE", "/folders/"+private.ID, f.ownerToken, nil, 200, nil)
	f.denied(t, "GET", "/folders/"+private.ID, f.ownerToken, nil)
	if _, err := f.personal.Get(context.Background(), f.owner.ID, f.group.ID); err != nil {
		t.Fatal("folder deletion deleted group", err)
	}
	if _, err := f.personal.Get(context.Background(), f.owner.ID, f.direct.ID); err != nil {
		t.Fatal("folder deletion deleted direct", err)
	}
	if _, err := f.conferences.Get(context.Background(), f.meeting.ID); err != nil {
		t.Fatal("folder deletion deleted meeting", err)
	}
	if f.get(t, f.ownerToken, second.ID).ItemCount != 1 {
		t.Fatal("folder deletion changed sibling mapping")
	}
}

func TestFoldersNormalizedNamesAndConcurrentLimit(t *testing.T) {
	f := newFolderFixture(t)
	item := f.create(t, f.ownerToken, "  Cafe\u0301  ")
	if item.Name != "Café" {
		t.Fatal("name was not trimmed/NFC", item.Name)
	}
	f.api.expect(t, "POST", "/folders", f.ownerToken, map[string]any{"name": "CAFÉ"}, 409, nil)
	f.create(t, f.memberToken, "CAFÉ")
	f.create(t, f.ownerToken, strings.Repeat("я", 50))
	for _, name := range []string{" ", strings.Repeat("я", 51), "New\nline", "Null\x00name", "Bidi\u202ename"} {
		f.api.expect(t, "POST", "/folders", f.ownerToken, map[string]any{"name": name}, 422, nil)
	}
	f.api.expect(t, "POST", "/folders", f.ownerToken, map[string]any{"name": "Forged owner", "userId": f.member.ID}, 400, nil)
	for i := 0; i < 97; i++ {
		f.create(t, f.ownerToken, fmt.Sprintf("Capacity %03d", i))
	}
	var responses [6]*httptest.ResponseRecorder
	runConcurrent(len(responses), func(i int) {
		responses[i] = f.raw(t, "POST", "/folders", f.ownerToken, map[string]any{"name": fmt.Sprintf("Racing %d", i)})
	})
	created := 0
	for _, response := range responses {
		if response.Code == 201 {
			created++
		} else if response.Code < 400 || response.Code >= 500 {
			t.Fatal("bad limit response", response.Code, response.Body.String())
		}
	}
	var result struct{ Items []folderView }
	f.api.expect(t, "GET", "/folders", f.ownerToken, nil, 200, &result)
	if created != 1 || len(result.Items) != 100 {
		t.Fatalf("concurrent limit violated: created=%d folders=%d", created, len(result.Items))
	}
}

func (f *folderFixture) activeMeeting(t *testing.T, name string) conferences.View {
	t.Helper()
	ctx := context.Background()
	meeting, err := f.conferenceService.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferenceService.Join(ctx, f.owner.ID, meeting.ID, conferences.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferenceService.Transition(ctx, f.owner.ID, meeting.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	return meeting
}

func TestFoldersHideLostAccessBeforeCountsAndLimit(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	folder := f.create(t, f.memberToken, "Member organization")
	deleted, _, err := f.personal.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Deleted private group", MemberIDs: []string{f.member.ID}})
	if err != nil {
		t.Fatal(err)
	}
	invited := f.activeMeeting(t, "Invitation before joining")
	if _, err = pg.NewConferenceInvitationRepository(f.db).Invite(ctx, f.owner.ID, invited.ID, conferences.InvitationRequest{UserIDs: []string{f.member.ID}}); err != nil {
		t.Fatal(err)
	}
	ordinaryLeft := f.activeMeeting(t, "Ordinary former participant")
	if _, err = f.conferenceService.Join(ctx, f.member.ID, ordinaryLeft.ID, conferences.JoinRequest{InviteCode: ordinaryLeft.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferenceService.Leave(ctx, f.member.ID, ordinaryLeft.ID); err != nil {
		t.Fatal(err)
	}
	waiting := f.activeMeeting(t, "Waiting limited card")
	if err = f.db.Create(&conferences.Participant{ID: uuid.NewString(), ConferenceID: waiting.ID, UserID: &f.member.ID, DisplayName: "Bob", Role: conferences.ParticipantRole, Status: conferences.Waiting, AdmissionState: conferences.AdmissionWaiting}).Error; err != nil {
		t.Fatal(err)
	}
	chatLeft := f.activeMeeting(t, "Explicitly left chat")
	if _, err = f.conferenceService.Join(ctx, f.member.ID, chatLeft.ID, conferences.JoinRequest{InviteCode: chatLeft.InviteCode}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct{ kind, id string }{{"conversation", f.direct.ID}, {"conversation", f.group.ID}, {"conversation", deleted.ID}, {"conference", f.meeting.ID}, {"conference", invited.ID}, {"conference", ordinaryLeft.ID}, {"conference", waiting.ID}, {"conference", chatLeft.ID}} {
		f.add(t, f.memberToken, folder.ID, target.kind, target.id)
	}
	if f.get(t, f.memberToken, folder.ID).ItemCount != 8 {
		t.Fatal("invited/left/waiting membership wrongly hidden")
	}
	page := f.items(t, f.memberToken, "/folders/"+folder.ID+"/items?limit=100")
	for _, item := range page.Items {
		if folderItemID(t, item) == "conference:"+waiting.ID {
			var data map[string]any
			if err = json.Unmarshal(item.Item, &data); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"participantCount", "inviteCode", "inviteUrl", "participants", "recordings"} {
				if value, exists := data[field]; exists && value != nil && value != "" {
					t.Fatalf("waiting folder card leaked %s: %s", field, item.Item)
				}
			}
		}
	}
	// Revoked rows are newer than all remaining rows: filtering after LIMIT would
	// produce an empty first page despite accessible entries farther below.
	if err = f.db.Exec("UPDATE conversations SET last_message_at=clock_timestamp()+INTERVAL '1 day' WHERE id IN ?", []string{f.group.ID, deleted.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.personal.RemoveGroupMember(ctx, f.owner.ID, f.group.ID, f.member.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.personal.DeleteGroup(ctx, f.owner.ID, deleted.ID); err != nil {
		t.Fatal(err)
	}
	member, err := f.conferences.Membership(ctx, f.meeting.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Moderate(ctx, f.meeting.ID, f.owner.ID, member.ID, conferences.ModerationRequest{Action: "kick"}); err != nil {
		t.Fatal(err)
	}
	waitingMember, err := f.conferences.Membership(ctx, waiting.ID, f.member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.DecideAdmission(ctx, waiting.ID, f.owner.ID, waitingMember.ID, conferences.AdmissionRequest{Decision: "reject"}); err != nil {
		t.Fatal(err)
	}
	if err = pg.NewChatRepository(f.db).LeaveChat(ctx, f.member.ID, chatLeft.ID); err != nil {
		t.Fatal(err)
	}
	visible := f.get(t, f.memberToken, folder.ID)
	if visible.ItemCount != 3 || visible.ConversationCount != 1 || visible.ConferenceCount != 2 {
		t.Fatal("lost access leaked counts", visible)
	}
	page = f.items(t, f.memberToken, "/folders/"+folder.ID+"/items?limit=1")
	if len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal("visibility filtering happened after limit", page)
	}
	seen := map[string]bool{}
	for {
		for _, item := range page.Items {
			key := folderItemID(t, item)
			if seen[key] {
				t.Fatal("duplicate pagination", key)
			}
			seen[key] = true
		}
		if page.NextCursor == "" {
			break
		}
		page = f.items(t, f.memberToken, "/folders/"+folder.ID+"/items?limit=1&before="+url.QueryEscape(page.NextCursor))
	}
	for _, key := range []string{"conversation:" + f.direct.ID, "conference:" + invited.ID, "conference:" + ordinaryLeft.ID} {
		if !seen[key] {
			t.Fatal("permitted item lost", key, seen)
		}
	}
	if len(seen) != 3 {
		t.Fatal("revoked item appeared in folder", seen)
	}
	unmapped, _, err := f.personal.GetOrCreate(ctx, f.member.ID, f.outsider.ID)
	if err != nil {
		t.Fatal(err)
	}
	candidates := f.items(t, f.memberToken, "/folder-items?folderId="+folder.ID+"&limit=100")
	if len(candidates.Items) != 4 {
		t.Fatal("candidate picker leaked stale mappings", candidates)
	}
	for _, item := range candidates.Items {
		key := folderItemID(t, item)
		mapped := key != "conversation:"+unmapped.ID
		if item.InFolder == nil || *item.InFolder != mapped || (mapped && !seen[key]) {
			t.Fatal("candidate membership mismatch", item)
		}
	}
	var picker struct{ Items []folderView }
	f.api.expect(t, "GET", "/folders?itemKind=conversation&itemId="+f.group.ID, f.memberToken, nil, 200, &picker)
	if len(picker.Items) != 1 || picker.Items[0].Contains == nil || *picker.Items[0].Contains || picker.Items[0].ItemCount != 3 {
		t.Fatal("revoked item leaked checkbox membership", picker)
	}
	// Ownership alone permits removing a stale reference; it never restores access.
	f.api.expect(t, "DELETE", "/folders/"+folder.ID+"/items/conversation/"+f.group.ID, f.memberToken, nil, 200, nil)
	f.denied(t, "PUT", "/folders/"+folder.ID+"/items/conversation/"+f.group.ID, f.memberToken, nil)
	for _, id := range []string{f.meeting.ID, waiting.ID, chatLeft.ID} {
		f.denied(t, "PUT", "/folders/"+folder.ID+"/items/conference/"+id, f.memberToken, nil)
	}
	if f.get(t, f.memberToken, folder.ID).ItemCount != 3 {
		t.Fatal("stale reference removal changed visible count")
	}
}

func TestFolderCursorsAreBoundToOwnerScopeAndFilter(t *testing.T) {
	f := newFolderFixture(t)
	first := f.create(t, f.ownerToken, "First")
	second := f.create(t, f.ownerToken, "Second")
	for _, target := range []struct{ kind, id string }{{"conversation", f.direct.ID}, {"conversation", f.group.ID}, {"conference", f.meeting.ID}} {
		f.add(t, f.ownerToken, first.ID, target.kind, target.id)
		f.add(t, f.ownerToken, second.ID, target.kind, target.id)
	}
	page := f.items(t, f.ownerToken, "/folders/"+first.ID+"/items?limit=1")
	if page.NextCursor == "" {
		t.Fatal("missing bounded page cursor")
	}
	cursor := url.QueryEscape(page.NextCursor)
	for _, path := range []string{"/folders/" + second.ID + "/items?before=" + cursor, "/folders/" + first.ID + "/items?type=conference&before=" + cursor, "/folders/" + first.ID + "/items?search=Different&before=" + cursor, "/folder-items?before=" + cursor} {
		f.api.expect(t, "GET", path, f.ownerToken, nil, 422, nil)
	}
	f.denied(t, "GET", "/folders/"+first.ID+"/items?before="+cursor, f.memberToken, nil)
	candidate := f.items(t, f.ownerToken, "/folder-items?limit=1")
	if candidate.NextCursor == "" {
		t.Fatal("candidate cursor missing")
	}
	f.api.expect(t, "GET", "/folder-items?before="+url.QueryEscape(candidate.NextCursor), f.memberToken, nil, 422, nil)
	f.api.expect(t, "GET", "/folders/"+first.ID+"/items?limit=101", f.ownerToken, nil, 422, nil)
	f.api.expect(t, "GET", "/folder-items?before=malformed", f.ownerToken, nil, 422, nil)
	// An exact shared UUID across both target tables must survive the kind tie-breaker.
	ctx := context.Background()
	shared := f.direct.ID
	meeting := conferences.Conference{ID: shared, OwnerID: f.owner.ID, Title: "Same UUID different kind", InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Created}
	actor := f.owner.ID
	_, err := f.conferences.Create(ctx, meeting, conferences.Participant{ID: uuid.NewString(), ConferenceID: shared, UserID: &actor, DisplayName: "Alice", Role: conferences.Owner, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted})
	if err != nil {
		t.Fatal(err)
	}
	f.add(t, f.ownerToken, first.ID, "conference", shared)
	if err = f.db.Exec("UPDATE conversations SET last_message_at=? WHERE id=?", time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), shared).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE conferences SET created_at=? WHERE id=?", time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC), shared).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	next := ""
	for {
		page = f.items(t, f.ownerToken, "/folders/"+first.ID+"/items?limit=1&before="+url.QueryEscape(next))
		for _, item := range page.Items {
			key := folderItemID(t, item)
			if seen[key] {
				t.Fatal("kind cursor duplicated item", key)
			}
			seen[key] = true
		}
		if page.NextCursor == "" {
			break
		}
		next = page.NextCursor
	}
	if !seen["conversation:"+shared] || !seen["conference:"+shared] || len(seen) != 4 {
		t.Fatal("mixed UUID collision skipped item", seen)
	}
}
