package integration_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/domain/notifications"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
)

func drainMeetingInvitations(t *testing.T, f *integrationFixture) {
	t.Helper()
	for range 100 {
		count := 0
		for _, kind := range []string{"integrations.conference", "integrations.event", "integrations.invitation", "integrations.delivery"} {
			for {
				job, found, err := f.jobs.Claim(context.Background(), kind, time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					break
				}
				count++
				err = f.service.Handle(context.Background(), job)
				state := "done"
				if errors.Is(err, jobs.ErrSkip) {
					state = "skipped"
				} else if err != nil {
					t.Fatal(kind, err)
				}
				if err = f.jobs.Finish(context.Background(), job, state, "", nil); err != nil {
					t.Fatal(err)
				}
			}
		}
		if count == 0 {
			return
		}
	}
	t.Fatal("invitation queue did not drain")
}

func TestMeetingInvitationsTransactionalDelivery(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	at := time.Now().UTC().Add(time.Hour)
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Planning", ScheduledAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	items, err := service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{Emails: []string{" External@Example.org ", f.member.Email}, UserIDs: []string{f.member.ID}})
	if err != nil || len(items) != 2 {
		t.Fatal(items, err)
	}
	for _, item := range items {
		if item.Status != "queued" {
			t.Fatal(item)
		}
	}
	var membership conferences.Participant
	if err = f.db.Where("conference_id=? AND user_id=?", c.ID, f.member.ID).Take(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if membership.Status != conferences.Left || membership.JoinedAt != nil || membership.AdmissionState != conferences.AdmissionAdmitted {
		t.Fatal("invitation falsified presence", membership)
	}
	listed, err := f.conferences.List(ctx, f.member.ID, 20, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != c.ID {
		t.Fatal("cabinet missing invitation", listed, err)
	}
	var notification notifications.Notification
	if err = f.db.Where("user_id=? AND type='conference.invited'", f.member.ID).Take(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if notification.Payload.InvitationID == "" || notification.Payload.ConferenceID != c.ID || notification.Payload.ScheduledAt == nil {
		t.Fatal("personal notification payload", notification)
	}
	prefs, err := f.service.Preferences(ctx, f.member.ID)
	if err != nil || prefs.Email {
		t.Fatal("test must exercise email=false default", prefs, err)
	}
	drainMeetingInvitations(t, f)
	var invitationCount, sentCount, notificationCount, genericEmails, enrollmentCount int64
	for _, check := range []struct {
		table, where string
		count        *int64
	}{
		{"conference_invitations", "conference_id=?", &invitationCount},
		{"conference_invitations", "conference_id=? AND status='sent' AND sent_at IS NOT NULL", &sentCount},
		{"notifications", "payload->>'conferenceId'=? AND user_id='" + f.member.ID + "' AND type='conference.invited'", &notificationCount},
		{"background_jobs", "conference_id=? AND kind='integrations.delivery' AND payload->>'channel'='email' AND user_id='" + f.member.ID + "'", &genericEmails},
		{"background_jobs", "conference_id=? AND dedup_key LIKE 'enrollment:%'", &enrollmentCount},
	} {
		if err = f.db.Table(check.table).Where(check.where, c.ID).Count(check.count).Error; err != nil {
			t.Fatal(err)
		}
	}
	if invitationCount != 2 || sentCount != 2 || notificationCount != 1 || genericEmails != 0 || enrollmentCount != 0 {
		t.Fatal("delivery/dedup", invitationCount, sentCount, notificationCount, genericEmails, enrollmentCount)
	}
	f.mu.Lock()
	delivered := len(f.sent)
	f.mu.Unlock()
	if delivered != 2 {
		t.Fatal("email was skipped or duplicated", delivered)
	}
	items, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{Emails: []string{f.member.Email, "external@example.org"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Status != "already_invited" {
			t.Fatal(item)
		}
	}
	drainMeetingInvitations(t, f)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) != 2 {
		t.Fatal("repeat invitation resent mail")
	}
}

func TestMeetingInvitationsPermissionsDirectoryAndGuest(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Permissions"})
	if err != nil {
		t.Fatal(err)
	}
	guest := users.User{ID: uuid.NewString(), Email: "member-guest@example.org", PasswordHash: "never-valid", GuestConferenceID: &c.ID}
	if err = f.db.Create(&guest).Error; err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{f.member.ID, guest.ID} {
		if _, err = service.Search(ctx, actor, c.ID, "member"); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("directory leaked", err)
		}
		if _, err = service.Invite(ctx, actor, c.ID, conferences.InvitationRequest{Emails: []string{"outside@example.org"}}); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatal("nonorganizer invited", err)
		}
	}
	items, err := service.Search(ctx, f.owner.ID, c.ID, "member")
	if err != nil || len(items) != 1 || items[0].ID != f.member.ID {
		t.Fatal("guest directory entry", items, err)
	}
	if _, err = service.Search(ctx, f.owner.ID, c.ID, "m"); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("short search", err)
	}
	items, err = service.Search(ctx, f.owner.ID, c.ID, "me%")
	if err != nil || len(items) != 0 {
		t.Fatal("search accepted wildcard", items, err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{guest.ID}}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("guest was selected as account", err)
	}
	if _, err = f.conferences.Join(ctx, f.member.ID, c.ID, conferences.JoinRequest{InviteCode: c.InviteCode}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&conferences.Participant{}).Where("conference_id=? AND user_id=?", c.ID, f.member.ID).Update("role", "co_host").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.member.ID, c.ID, conferences.InvitationRequest{Emails: []string{"outside@example.org"}}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("cohost invited before active", err)
	}
	if _, err = f.conferences.Transition(ctx, f.owner.ID, c.ID, conferences.Active); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.member.ID, c.ID, conferences.InvitationRequest{Emails: []string{"outside@example.org"}}); err != nil {
		t.Fatal("active cohost denied", err)
	}
	if _, err = f.conferences.Leave(ctx, f.member.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.member.ID, c.ID, conferences.InvitationRequest{Emails: []string{"outside2@example.org"}}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("left cohost invited", err)
	}
	if _, err = f.conferences.Transition(ctx, f.owner.ID, c.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{Emails: []string{"outside2@example.org"}}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal("finished room accepted invite", err)
	}
	drainMeetingInvitations(t, f)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) != 0 {
		t.Fatal("finished room delivered stale email")
	}
}

func TestMeetingInvitationsConcurrentDedupAndFailedRetry(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan []conferences.InvitationResult, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{f.member.ID}})
			results <- items
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	queued, already := 0, 0
	for items := range results {
		if len(items) != 1 {
			t.Fatal(items)
		}
		switch items[0].Status {
		case "queued":
			queued++
		case "already_invited":
			already++
		}
	}
	if queued != 1 || already != 1 {
		t.Fatal("concurrent request dedup", queued, already)
	}
	job, found, err := f.jobs.Claim(ctx, "integrations.invitation", time.Minute)
	if err != nil || !found {
		t.Fatal(found, err)
	}
	f.mu.Lock()
	f.fail = true
	f.mu.Unlock()
	if err = f.service.Handle(ctx, job); err == nil {
		t.Fatal("failed provider accepted")
	}
	if err = f.service.FailJob(ctx, job, "provider_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err = f.jobs.Finish(ctx, job, "failed", "provider_unavailable", nil); err != nil {
		t.Fatal(err)
	}
	items, err := service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{f.member.ID}})
	if err != nil || len(items) != 1 || items[0].Status != "queued" || items[0].ID != job.EntityID {
		t.Fatal("failed retry did not requeue", items, err)
	}
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	drainMeetingInvitations(t, f)
	var count int64
	if err = f.db.Table("notifications").Where("user_id=? AND type='conference.invited'", f.member.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("retry duplicated notification", count, err)
	}
	var invitation conferences.Invitation
	if err = f.db.Where("id=?", job.EntityID).Take(&invitation).Error; err != nil || invitation.Status != "sent" {
		t.Fatal("retry delivery", invitation, err)
	}
}

func TestMeetingInvitationsDoesNotRestoreExcludedParticipants(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Excluded"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{f.member.ID}}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&conferences.Participant{}).Where("conference_id=? AND user_id=?", c.ID, f.member.ID).Updates(map[string]any{"status": "kicked", "admission_state": "kicked"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{Emails: []string{f.member.Email}}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("kicked member re-invited", err)
	}
	drainMeetingInvitations(t, f)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) != 0 {
		t.Fatal("kicked recipient received queued email")
	}
	var membership conferences.Participant
	if err = f.db.Where("conference_id=? AND user_id=?", c.ID, f.member.ID).Take(&membership).Error; err != nil || membership.Status != conferences.Kicked {
		t.Fatal("kick restored", membership, err)
	}
}

func TestMeetingInvitationsPreservesJoinedMemberAndRollsBackRejectedBatch(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Existing membership"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.conferences.Join(ctx, f.member.ID, c.ID, conferences.JoinRequest{InviteCode: c.InviteCode}); err != nil {
		t.Fatal(err)
	}
	var before, after conferences.Participant
	if err = f.db.Where("conference_id=? AND user_id=?", c.ID, f.member.ID).Take(&before).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{f.member.ID}}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Where("id=?", before.ID).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("explicit invitation changed existing participation")
	}
	c2, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Rejected batch"})
	if err != nil {
		t.Fatal(err)
	}
	membership := conferences.Participant{ID: uuid.NewString(), ConferenceID: c2.ID, UserID: &f.member.ID, DisplayName: "Rejected", Role: conferences.ParticipantRole, Status: conferences.Rejected, AdmissionState: conferences.AdmissionRejected}
	if err = f.db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c2.ID, conferences.InvitationRequest{Emails: []string{"aaa-external@example.org", f.member.Email}}); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatal("rejected membership restored", err)
	}
	var invitations, queued int64
	if err = f.db.Table("conference_invitations").Where("conference_id=?", c2.ID).Count(&invitations).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("background_jobs").Where("conference_id=? AND kind='integrations.invitation'", c2.ID).Count(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if invitations != 0 || queued != 0 {
		t.Fatal("rejected batch partly committed", invitations, queued)
	}
}

func TestMeetingInvitationsRejectsInternationalizedSelectedAccount(t *testing.T) {
	f := stageSevenIntegrations(t)
	ctx := context.Background()
	service := &conferenceusecase.InvitationService{Repository: pg.NewConferenceInvitationRepository(f.db)}
	c, err := f.conferences.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "SMTPUTF8"})
	if err != nil {
		t.Fatal(err)
	}
	account := users.User{ID: uuid.NewString(), Email: "тест@example.org", PasswordHash: "test-only-hash"}
	if err = f.db.Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = service.Invite(ctx, f.owner.ID, c.ID, conferences.InvitationRequest{UserIDs: []string{account.ID}}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatal("SMTPUTF8 account entered invitation queue", err)
	}
	var invitations, memberships int64
	if err = f.db.Table("conference_invitations").Where("conference_id=?", c.ID).Count(&invitations).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("conference_participants").Where("conference_id=? AND user_id=?", c.ID, account.ID).Count(&memberships).Error; err != nil {
		t.Fatal(err)
	}
	if invitations != 0 || memberships != 0 {
		t.Fatal("unsupported address partly committed", invitations, memberships)
	}
	var stored users.User
	if err = f.db.Where("id=?", account.ID).Take(&stored).Error; err != nil || stored.Email != account.Email {
		t.Fatal("account email changed", stored.Email, err)
	}
}
