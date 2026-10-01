package integration_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

func TestStageFiveWaitingAdmissionAndProtectedResources(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	api := stageOneAPI{router: f.servers[0].Config.Handler.(*gin.Engine)}
	c, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Waiting room", WaitingRoomEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	path := "/conferences/" + c.ID
	api.expect(t, "POST", path+"/join", f.ownerToken, nil, 200, nil)
	api.expect(t, "POST", path+"/start", f.ownerToken, nil, 200, nil)
	ownerSocket := f.connect(t, 0, f.ownerToken, c.ID)
	var pending struct{ Item conferences.ParticipantView }
	api.expect(t, "POST", "/conference-invites/"+c.InviteCode+"/join", f.memberToken, nil, 200, &pending)
	if pending.Item.Status != conferences.Waiting || pending.Item.AdmissionState != conferences.AdmissionWaiting || pending.Item.JoinedAt != nil {
		t.Fatalf("invalid waiting membership: %+v", pending.Item)
	}
	ownerSocket.wait(t, func(e realtime.Envelope) bool { return e.Type == "participant.waiting" })
	id := pending.Item.ID
	blocked := true
	for _, request := range []conferences.ModerationRequest{{Action: "kick"}, {Action: "mute", Blocked: &blocked}, {Action: "role", Role: conferences.CoHost}} {
		if _, err = repo.Moderate(ctx, c.ID, f.owner.ID, id, request); !errors.Is(err, apperrors.ErrForbidden) {
			t.Fatalf("pending moderation leaked into room: %+v %v", request, err)
		}
	}
	for range 3 {
		api.expect(t, "POST", path+"/join", f.memberToken, nil, 200, &pending)
		if pending.Item.ID != id || pending.Item.AdmissionState != conferences.AdmissionWaiting {
			t.Fatal("reconnect bypassed waiting or duplicated participant")
		}
	}
	var own struct{ Item conferences.ParticipantView }
	api.expect(t, "GET", path+"/participants/me", f.memberToken, nil, 200, &own)
	if own.Item.ID != id {
		t.Fatal("self membership incorrect")
	}
	api.expect(t, "GET", path+"/participants", f.memberToken, nil, 403, nil)
	api.expect(t, "GET", path+"/history", f.memberToken, nil, 403, nil)
	var preview struct{ Item conferences.View }
	api.expect(t, "GET", path, f.memberToken, nil, 200, &preview)
	if preview.Item.InviteCode != "" || preview.Item.InviteURL != "" {
		t.Fatal("waiting preview leaked invitation")
	}
	api.expect(t, "POST", path+"/ws-ticket", f.memberToken, nil, 403, nil)
	if _, err = pg.NewSessionRepository(f.db).Authorize(ctx, c.ID, f.member.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("waiting authorized session: %v", err)
	}
	policy, err := repo.MediaPolicy(ctx, c.ID, id)
	if err != nil || !policy.Kicked {
		t.Fatalf("waiting media policy allowed: %+v %v", policy, err)
	}
	if _, err = pg.NewConferenceRecordingRepository(f.db).List(ctx, f.member.ID, c.ID, 10, 0); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("waiting recording read: %v", err)
	}
	api.expect(t, "POST", path+"/participants/"+id+"/admission", f.memberToken, map[string]any{"decision": "admit"}, 403, nil)
	api.expect(t, "POST", path+"/participants/"+id+"/admission", f.ownerToken, map[string]any{"decision": "admit"}, 200, &pending)
	if pending.Item.AdmissionState != conferences.AdmissionAdmitted || pending.Item.Status != conferences.Joined || pending.Item.AdmissionDecidedAt == nil || pending.Item.AdmissionVersion != 2 {
		t.Fatalf("invalid admitted membership: %+v", pending.Item)
	}
	api.expect(t, "POST", path+"/participants/"+id+"/admission", f.ownerToken, map[string]any{"decision": "admit"}, 200, &pending)
	if pending.Item.AdmissionVersion != 2 {
		t.Fatal("duplicate admission advanced version")
	}
	api.expect(t, "POST", path+"/participants/"+id+"/admission", f.ownerToken, map[string]any{"decision": "reject"}, 409, nil)
	api.expect(t, "POST", path+"/ws-ticket", f.memberToken, nil, http.StatusCreated, nil)
	f.connect(t, 1, f.memberToken, c.ID)
	api.expect(t, "POST", path+"/leave", f.memberToken, nil, 200, nil)
	api.expect(t, "POST", path+"/join", f.memberToken, nil, 200, &pending)
	if pending.Item.ID != id || pending.Item.AdmissionState != conferences.AdmissionAdmitted {
		t.Fatal("admitted reconnect lost decision")
	}
}

func TestStageFiveAdmissionRacesRejectAndCoHost(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	if err := f.db.Model(&conferences.Conference{}).Where("id = ?", f.conference.ID).Update("waiting_room_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	member, _ := repo.Membership(ctx, f.conference.ID, f.member.ID)
	if _, err := repo.Moderate(ctx, f.conference.ID, f.owner.ID, member.ID, conferences.ModerationRequest{Action: "role", Role: conferences.CoHost}); err != nil {
		t.Fatal(err)
	}
	newWaiter := func() users.User {
		u := users.User{ID: uuid.NewString(), Email: uuid.NewString() + "@stage5.example", PasswordHash: f.owner.PasswordHash}
		if _, err := pg.NewUserRepository(f.db).Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Join(ctx, f.conference.ID, u, f.conference.InviteCode); err != nil {
			t.Fatal(err)
		}
		return u
	}
	u := newWaiter()
	p, _ := repo.Membership(ctx, f.conference.ID, u.ID)
	results := make([]error, 20)
	runConcurrent(len(results), func(i int) {
		decision := "admit"
		if i%2 == 0 {
			decision = "reject"
		}
		_, results[i] = repo.DecideAdmission(ctx, f.conference.ID, f.member.ID, p.ID, conferences.AdmissionRequest{Decision: decision})
	})
	p, err := repo.Membership(ctx, f.conference.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.AdmissionVersion != 2 || p.AdmissionDecidedAt == nil {
		t.Fatalf("race decision repeated: %+v", p)
	}
	for i, err := range results {
		matches := (i%2 == 0) == (p.AdmissionState == conferences.AdmissionRejected)
		if matches && err != nil || !matches && !errors.Is(err, apperrors.ErrConflict) {
			t.Fatalf("decision %d: state=%s err=%v", i, p.AdmissionState, err)
		}
	}
	v := newWaiter()
	rejected, _ := repo.Membership(ctx, f.conference.ID, v.ID)
	if _, err = repo.DecideAdmission(ctx, f.conference.ID, f.owner.ID, rejected.ID, conferences.AdmissionRequest{Decision: "reject"}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Join(ctx, f.conference.ID, v, f.conference.InviteCode); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("rejected invite bypass: %v", err)
	}
	if _, err = pg.NewSessionRepository(f.db).Authorize(ctx, f.conference.ID, v.ID); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("rejected session: %v", err)
	}
	if _, err = pg.NewConferenceRecordingRepository(f.db).List(ctx, v.ID, f.conference.ID, 10, 0); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("rejected recording access: %v", err)
	}
}

func TestStageFiveScheduledHistoryAndCursor(t *testing.T) {
	f := stageTwo(t)
	ctx := context.Background()
	repo := pg.NewConferenceRepository(f.db)
	api := stageOneAPI{router: f.servers[0].Config.Handler.(*gin.Engine)}
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	duration := 45
	c, err := f.service.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: "Scheduled", ScheduledAt: &future, PlannedDurationMin: &duration})
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != conferences.Scheduled {
		t.Fatal("scheduled create did not set lifecycle")
	}
	p, err := repo.Join(ctx, c.ID, f.member, c.InviteCode)
	if err != nil || p.Status != conferences.Left || !p.IsAdmitted() {
		t.Fatalf("enrollment: %+v %v", p, err)
	}
	if _, err = pg.NewSessionRepository(f.db).Authorize(ctx, c.ID, f.member.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("scheduled media allowed: %v", err)
	}
	path := "/conferences/" + c.ID
	api.expect(t, "PUT", path+"/schedule", f.memberToken, map[string]any{"scheduledAt": future.Add(time.Hour)}, 403, nil)
	api.expect(t, "PUT", path+"/schedule", f.ownerToken, map[string]any{"scheduledAt": "2026-10-02T10:00:00"}, 400, nil)
	var timeline struct {
		Items      []conferences.View
		NextCursor *string
	}
	api.expect(t, "GET", "/me/conferences?view=upcoming&scope=participating", f.memberToken, nil, 200, &timeline)
	if len(timeline.Items) != 1 || timeline.Items[0].ID != c.ID {
		t.Fatal("enrollment missing from upcoming")
	}
	var updateErr, startErr error
	runConcurrent(2, func(i int) {
		if i == 0 {
			_, updateErr = repo.UpdateSchedule(ctx, c.ID, f.owner.ID, conferences.ScheduleRequest{ScheduledAt: future.Add(time.Hour)})
		} else {
			_, startErr = repo.Transition(ctx, c.ID, f.owner.ID, conferences.Active)
		}
	})
	if startErr != nil || updateErr != nil && !errors.Is(updateErr, apperrors.ErrConflict) {
		t.Fatalf("schedule/start race: %v / %v", updateErr, startErr)
	}
	api.expect(t, "PUT", path+"/schedule", f.ownerToken, map[string]any{"scheduledAt": future.Add(3 * time.Hour)}, 409, nil)
	if _, err = repo.Join(ctx, c.ID, f.owner, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Join(ctx, c.ID, f.member, ""); err != nil {
		t.Fatal(err)
	}
	record, _, err := pg.NewConferenceRecordingRepository(f.db).Start(ctx, f.owner.ID, c.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Transition(ctx, c.ID, f.owner.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&records.Record{}).Where("id = ?", record.ID).Update("status", records.StatusReady).Error; err != nil {
		t.Fatal(err)
	}
	var history struct{ Item conferences.HistoryView }
	api.expect(t, "GET", path+"/history", f.memberToken, nil, 200, &history)
	if history.Item.ParticipantCount != 2 || len(history.Item.Participants) != 2 || history.Item.Recordings.Ready != 1 || history.Item.Recordings.Total != 1 || !history.Item.ChatReadOnly || !history.Item.ChatAvailable || history.Item.DurationSec == nil {
		t.Fatalf("incomplete history: %+v", history.Item)
	}
	api.expect(t, "GET", "/me/conferences?view=past", f.memberToken, nil, 200, &timeline)
	if len(timeline.Items) != 1 || timeline.Items[0].ID != c.ID {
		t.Fatal("finished conference missing from history")
	}
	api.expect(t, "GET", "/me/conferences?view=upcoming&cursor=bad", f.ownerToken, nil, 422, nil)
	api.expect(t, "GET", "/me/conferences?from=2026-10-01", f.ownerToken, nil, 422, nil)
	// Bulk seed exercises the indexed chronological query, bounded pagination,
	// deterministic equal-date ties and no per-conference membership lookups.
	cs := make([]conferences.Conference, 1200)
	ps := make([]conferences.Participant, len(cs))
	ownerID := f.owner.ID
	for i := range cs {
		at := future.Add(time.Duration(i/3) * time.Minute)
		id := uuid.NewString()
		cs[i] = conferences.Conference{ID: id, OwnerID: ownerID, Title: fmt.Sprintf("Seed %d", i), InviteCode: strings.ReplaceAll(uuid.NewString(), "-", ""), Status: conferences.Scheduled, ScheduledAt: &at}
		ps[i] = conferences.Participant{ID: uuid.NewString(), ConferenceID: id, UserID: &ownerID, DisplayName: "Owner", Role: conferences.Owner, Status: conferences.Left, AdmissionState: conferences.AdmissionAdmitted}
	}
	if err = f.db.CreateInBatches(cs, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.CreateInBatches(ps, 100).Error; err != nil {
		t.Fatal(err)
	}
	// The isolated database has no autovacuum statistics yet. Analyze bulk
	// fixtures before evaluating a plan, as production autovacuum would do.
	if err = f.db.Exec("ANALYZE conferences").Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("ANALYZE conference_participants").Error; err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	query := conferences.TimelineQuery{View: "upcoming", Scope: "owned", Limit: 37}
	for {
		page, err := f.service.Timeline(ctx, ownerID, query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 37 {
			t.Fatal("unbounded history page")
		}
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate keyset row")
			}
			seen[item.ID] = true
		}
		if page.NextCursor == nil {
			break
		}
		query.Cursor = *page.NextCursor
	}
	if len(seen) != len(cs) {
		t.Fatalf("timeline omitted rows: %d/%d", len(seen), len(cs))
	}
	var plan []struct {
		QueryPlan string `gorm:"column:QUERY PLAN"`
	}
	if err = f.db.Raw(`EXPLAIN (ANALYZE, BUFFERS) SELECT c.id FROM conferences c JOIN conference_participants p ON p.conference_id=c.id AND p.user_id=? WHERE c.status='scheduled' AND p.admission_state='admitted' ORDER BY COALESCE(c.finished_at,c.scheduled_at,c.created_at) DESC,c.id DESC LIMIT 20`, ownerID).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	if len(plan) == 0 {
		t.Fatal("no query plan")
	}
	for _, row := range plan {
		t.Log(row.QueryPlan)
	}
}
