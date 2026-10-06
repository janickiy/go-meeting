package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	chatapp "github.com/janickiy/go-recorder/internal/app/chat"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	chatusecase "github.com/janickiy/go-recorder/internal/usecase/chat"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
)

func TestConferenceChatActionsAndNotifications(t *testing.T) {
	db := stageOneDatabase(t)
	ctx := context.Background()
	userRepo := pg.NewUserRepository(db)
	owner := users.User{ID: uuid.NewString(), Email: "chat-owner@example.test", PasswordHash: "test"}
	member := users.User{ID: uuid.NewString(), Email: "chat-member@example.test", PasswordHash: "test"}
	outsider := users.User{ID: uuid.NewString(), Email: "chat-outsider@example.test", PasswordHash: "test"}
	for _, user := range []users.User{owner, member, outsider} {
		if _, err := userRepo.Create(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	conferenceRepo := pg.NewConferenceRepository(db)
	conferenceService := conferenceusecase.NewService(conferenceRepo, userRepo, security.GenerateInviteCode)
	meeting, err := conferenceService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Team chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conferenceService.Join(ctx, owner.ID, meeting.ID, conferences.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := conferenceService.Join(ctx, member.ID, meeting.ID, conferences.JoinRequest{InviteCode: meeting.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if _, err := conferenceService.Transition(ctx, owner.ID, meeting.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	repo := pg.NewChatRepository(db)
	actions := chatusecase.NewActionService(repo, nil).WithPresence(conferenceChatOfflinePresence{}, conferenceChatOfflinePresence{})
	tokens, err := security.NewTokenService("conference-chat-actions-test-secret-at-least-32-bytes")
	if err != nil {
		t.Fatal(err)
	}
	memberToken, err := tokens.Issue(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	outsiderToken, err := tokens.Issue(outsider.ID)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	httptransport.RegisterChatActionRoutes(router, &chatapp.ActionHandler{Service: actions}, httpmiddleware.Authenticate(tokens), nil)
	api := stageOneAPI{router: router}
	path := "/conferences/" + meeting.ID + "/chat"
	api.expect(t, "GET", path+"/info", "", nil, 401, nil)
	api.expect(t, "GET", path+"/info", outsiderToken, nil, 403, nil)
	if response := api.expect(t, "GET", path+"/info", memberToken, nil, 200, nil); response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("chat info response is cacheable")
	}
	var memberResponse struct{ Items []conferences.ParticipantView }
	api.expect(t, "GET", path+"/members?limit=50", memberToken, nil, 200, &memberResponse)
	if len(memberResponse.Items) != 2 {
		t.Fatalf("chat HTTP members: %+v", memberResponse)
	}
	info, err := actions.Info(ctx, owner.ID, meeting.ID)
	if err != nil || info.Title != "Team chat" || info.ParticipantCount != 2 || !info.CanEdit || !info.NotificationsEnabled {
		t.Fatalf("info: %+v %v", info, err)
	}
	members, cursor, err := actions.Members(ctx, owner.ID, meeting.ID, "", 1)
	if err != nil || len(members) != 1 || cursor == "" {
		t.Fatalf("first member page: %+v %q %v", members, cursor, err)
	}
	otherMembers, next, err := actions.Members(ctx, owner.ID, meeting.ID, cursor, 1)
	if err != nil || len(otherMembers) != 1 || otherMembers[0].ID == members[0].ID || next != "" {
		t.Fatalf("second member page: %+v %q %v", otherMembers, next, err)
	}
	if _, err := actions.Info(ctx, outsider.ID, meeting.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("outsider opened chat info: %v", err)
	}
	guest := users.User{ID: uuid.NewString(), Email: "chat-guest@guest.invalid", PasswordHash: "test", GuestConferenceID: &meeting.ID}
	if _, err := userRepo.Create(ctx, guest); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO conference_participants(id,conference_id,user_id,display_name,role,status,joined_at,admission_state)
		VALUES(gen_random_uuid(),?::uuid,?::uuid,'Guest','participant','joined',now(),'admitted')`, meeting.ID, guest.ID).Error; err != nil {
		t.Fatal(err)
	}
	updatedTitle, description := "Renamed chat", "Shared meeting notes"
	info, err = actions.UpdateInfo(ctx, owner.ID, meeting.ID, chat.UpdateInfoRequest{Title: &updatedTitle, Description: &description})
	if err != nil || info.Title != updatedTitle || info.Description != description {
		t.Fatalf("update info: %+v %v", info, err)
	}
	if _, err := actions.UpdateInfo(ctx, member.ID, meeting.ID, chat.UpdateInfoRequest{Title: &updatedTitle}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("member rename: %v", err)
	}
	if _, err := actions.SetPreferences(ctx, member.ID, meeting.ID, false); err != nil {
		t.Fatal(err)
	}
	send := func(text string) chat.Message {
		t.Helper()
		request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: text})
		if err != nil {
			t.Fatal(err)
		}
		message, created, err := repo.Send(ctx, owner.ID, meeting.ID, request, fingerprint)
		if err != nil || !created {
			t.Fatalf("send: %v %v", created, err)
		}
		return message
	}
	_ = send("Only a literal pattern here")
	notices := pg.NewNotificationRepository(db).DisableLegacyReminders()
	if err := notices.Generate(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := notices.List(ctx, member.ID, "", 20)
	if err != nil || page.UnreadCount != 0 {
		t.Fatalf("muted notice: %+v %v", page, err)
	}
	if _, err := actions.SetPreferences(ctx, member.ID, meeting.ID, true); err != nil {
		t.Fatal(err)
	}
	message := send("Literal %_! link https://example.test/path")
	if err := notices.Generate(ctx); err != nil {
		t.Fatal(err)
	}
	page, err = notices.List(ctx, member.ID, "", 20)
	if err != nil || page.UnreadCount != 1 || len(page.Items) != 1 || page.Items[0].Type != "chat.message" || page.Items[0].Payload.MessageID != message.ID {
		t.Fatalf("chat notice: %+v %v", page, err)
	}
	guestNotices, err := notices.List(ctx, guest.ID, "", 20)
	if err != nil || guestNotices.UnreadCount != 0 {
		t.Fatalf("guest received chat notice: %+v %v", guestNotices, err)
	}
	var externalJobs int64
	if err := db.Table("background_jobs").Where("entity_id=? AND kind='integrations.delivery'", page.Items[0].ID).Count(&externalJobs).Error; err != nil || externalJobs != 0 {
		t.Fatalf("chat notice created external delivery: %d %v", externalJobs, err)
	}
	if _, err := actions.SetPreferences(ctx, member.ID, meeting.ID, false); err != nil {
		t.Fatal(err)
	}
	pending, err := notices.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range pending {
		if item.ID == page.Items[0].ID {
			t.Fatal("muted queued notice was returned for SSE")
		}
	}
	if allowed, err := notices.Publishable(ctx, page.Items[0].ID); err != nil || allowed {
		t.Fatalf("muted notice is publishable: %v %v", allowed, err)
	}
	delivery, err := pg.NewIntegrationRepository(db).Delivery(ctx, jobs.Job{EntityID: page.Items[0].ID, ConferenceID: meeting.ID, UserID: &member.ID})
	if err != nil || delivery.Allowed {
		t.Fatalf("muted external delivery allowed: %v %v", delivery.Allowed, err)
	}
	if _, err := actions.SetPreferences(ctx, member.ID, meeting.ID, true); err != nil {
		t.Fatal(err)
	}
	ownerNotices, err := notices.List(ctx, owner.ID, "", 20)
	if err != nil || ownerNotices.UnreadCount != 0 {
		t.Fatalf("self notice: %+v %v", ownerNotices, err)
	}
	search, err := actions.Search(ctx, member.ID, meeting.ID, "%_!", "", 20)
	if err != nil || len(search.Items) != 1 || search.Items[0].ID != message.ID {
		t.Fatalf("literal search: %+v %v", search, err)
	}
	materials, err := actions.Materials(ctx, member.ID, meeting.ID, "link", "", 20)
	if err != nil || len(materials.Items) != 1 || materials.Items[0].URL != "https://example.test/path" {
		t.Fatalf("links: %+v %v", materials, err)
	}
	if err := actions.SetPin(ctx, member.ID, meeting.ID, message.ID, true); err != nil {
		t.Fatal(err)
	}
	history, err := repo.List(ctx, member.ID, meeting.ID, "", 20)
	if err != nil || !history.Items[len(history.Items)-1].Important {
		t.Fatalf("personal marker: %+v %v", history, err)
	}
	ownerHistory, err := repo.List(ctx, owner.ID, meeting.ID, "", 20)
	if err != nil || ownerHistory.Items[len(ownerHistory.Items)-1].Important {
		t.Fatalf("leaked marker: %+v %v", ownerHistory, err)
	}
	contextPage, err := actions.Context(ctx, member.ID, meeting.ID, message.ID)
	if err != nil || len(contextPage.Items) != 2 || !contextPage.Items[1].Important {
		t.Fatalf("context: %+v %v", contextPage, err)
	}
	otherMeeting, err := conferenceService.Create(ctx, owner.ID, conferences.CreateRequest{Title: "Another chat"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conferenceService.Join(ctx, owner.ID, otherMeeting.ID, conferences.JoinRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := conferenceService.Transition(ctx, owner.ID, otherMeeting.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	request, fingerprint, err := chat.NormalizeSend(chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "Other room"})
	if err != nil {
		t.Fatal(err)
	}
	otherMessage, created, err := repo.Send(ctx, owner.ID, otherMeeting.ID, request, fingerprint)
	if err != nil || !created {
		t.Fatalf("other room send: %v %v", created, err)
	}
	if err := actions.SetPin(ctx, member.ID, meeting.ID, otherMessage.ID, true); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-room pin accepted: %v", err)
	}
	if _, err := actions.Context(ctx, member.ID, meeting.ID, otherMessage.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("cross-room context accepted: %v", err)
	}
	multiLink := send("https://a.example.test/1 HTTPS://b.example.test/2 https://c.example.test/3")
	var materialURLs []string
	before := ""
	for i := 0; i < 3; i++ {
		one, err := actions.Materials(ctx, member.ID, meeting.ID, "link", before, 1)
		if err != nil || len(one.Items) != 1 || one.Items[0].Message.ID != multiLink.ID {
			t.Fatalf("material page %d: %+v %v", i, one, err)
		}
		materialURLs = append(materialURLs, one.Items[0].URL)
		before = one.NextCursor
		if before == "" {
			t.Fatalf("material page %d lost continuation", i)
		}
	}
	if materialURLs[0] == materialURLs[1] || materialURLs[1] == materialURLs[2] || materialURLs[0] == materialURLs[2] {
		t.Fatalf("material cursor repeated URLs: %+v", materialURLs)
	}
	one, err := actions.Materials(ctx, member.ID, meeting.ID, "link", before, 1)
	if err != nil || len(one.Items) != 1 || one.Items[0].Message.ID != message.ID || one.NextCursor != "" {
		t.Fatalf("material cursor lost older message: %+v %v", one, err)
	}
	api.expect(t, "DELETE", path+"/membership", memberToken, nil, 200, nil)
	api.expect(t, "GET", path+"/info", memberToken, nil, 403, nil)
	if err := actions.Leave(ctx, member.ID, meeting.ID); err != nil {
		t.Fatalf("repeat leave: %v", err)
	}
	info, err = actions.Info(ctx, owner.ID, meeting.ID)
	if err != nil || info.ParticipantCount != 2 {
		t.Fatalf("left participant counted: %+v %v", info, err)
	}
	listed, err := conferenceRepo.ListForUser(ctx, member.ID, 20, 0)
	if err != nil || len(listed) != 0 {
		t.Fatalf("left meeting still listed: %+v %v", listed, err)
	}
	if _, err := repo.List(ctx, member.ID, meeting.ID, "", 20); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("left chat accessible: %v", err)
	}
	if _, err := pg.NewSessionRepository(db).Authorize(ctx, meeting.ID, member.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("left realtime accessible: %v", err)
	}
	invites := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(db)}
	results, err := invites.Invite(ctx, owner.ID, meeting.ID, conferences.InvitationRequest{UserIDs: []string{member.ID}})
	if err != nil || len(results) != 1 || results[0].Status != "left_chat" {
		t.Fatalf("reinviting a left member restored chat: %+v %v", results, err)
	}
	var invitations int64
	if err := db.Table("conference_invitations").Where("conference_id=? AND user_id=?", meeting.ID, member.ID).Count(&invitations).Error; err != nil || invitations != 0 {
		t.Fatalf("left member was silently re-invited: %d %v", invitations, err)
	}
	if _, err := conferenceService.Join(ctx, member.ID, meeting.ID, conferences.JoinRequest{InviteCode: meeting.InviteCode}); err != nil {
		t.Fatal(err)
	}
	listed, err = conferenceRepo.ListForUser(ctx, member.ID, 20, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("rejoin did not restore list: %+v %v", listed, err)
	}
	if _, err := repo.Delete(ctx, owner.ID, meeting.ID, message.ID); err != nil {
		t.Fatal(err)
	}
	pins, err := actions.Pins(ctx, member.ID, meeting.ID, "", 20)
	if err != nil || len(pins.Items) != 0 {
		t.Fatalf("deleted pin visible: %+v %v", pins, err)
	}
	if err := actions.Leave(ctx, owner.ID, meeting.ID); err != nil {
		t.Fatalf("owner personal leave: %v", err)
	}
	if _, err := repo.List(ctx, owner.ID, meeting.ID, "", 20); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("owner retained chat access after leaving: %v", err)
	}
	if _, err := conferenceRepo.Get(ctx, meeting.ID); err != nil {
		t.Fatalf("owner personal leave deleted conference: %v", err)
	}
	if info, err := actions.Info(ctx, member.ID, meeting.ID); err != nil || info.ParticipantCount != 2 {
		t.Fatalf("owner leave removed member access or wrong count: %+v %v", info, err)
	}
}
