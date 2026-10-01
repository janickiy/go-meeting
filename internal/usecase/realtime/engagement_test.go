package realtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

func TestOutOfOrderHandEventsUseCurrentState(t *testing.T) {
	allowed := map[string]bool{"member": true}
	oldRaise := domain.Event("hand.raised", "conference", domain.Hand{ParticipantID: "member", RaisedAt: time.Now()})
	if got := currentHandEvent(oldRaise, nil, allowed); got.Type != "hand.lowered" {
		t.Fatal("stale raise resurrected a lowered hand")
	}
	latest := domain.Hand{ParticipantID: "member", RaisedAt: time.Now().Add(time.Second)}
	oldLower := domain.Event("hand.lowered", "conference", map[string]string{"participantId": "member"})
	got := currentHandEvent(oldLower, []domain.Hand{latest}, allowed)
	var hand domain.Hand
	if got.Type != "hand.raised" || json.Unmarshal(got.Data, &hand) != nil || !hand.RaisedAt.Equal(latest.RaisedAt) {
		t.Fatal("stale lower erased latest raise")
	}
	if currentHandEvent(oldRaise, []domain.Hand{latest}, map[string]bool{}).Type != "hand.lowered" {
		t.Fatal("ineligible hand leaked")
	}
}

func TestStateVisibilityIsPerRecipient(t *testing.T) {
	state := domain.State{Participants: []domain.Presence{
		{ParticipantView: conferences.ParticipantView{ID: "owner", Role: conferences.Owner, Status: conferences.Joined, AdmissionState: conferences.AdmissionAdmitted}},
		{ParticipantView: conferences.ParticipantView{ID: "member", Role: conferences.ParticipantRole, Status: conferences.Joined, AdmissionState: conferences.AdmissionAdmitted}},
		{ParticipantView: conferences.ParticipantView{ID: "waiting", Role: conferences.ParticipantRole, Status: conferences.Waiting, AdmissionState: conferences.AdmissionWaiting}},
		{ParticipantView: conferences.ParticipantView{ID: "rejected", Role: conferences.ParticipantRole, Status: conferences.Rejected, AdmissionState: conferences.AdmissionRejected}},
	}}
	member := stateFor(state, domain.Session{ParticipantID: "member", ConnectionID: "tab2"})
	if len(member.Participants) != 2 || member.ConnectionID != "tab2" {
		t.Fatal("waiting/rejected roster leaked")
	}
	owner := stateFor(state, domain.Session{ParticipantID: "owner", ConnectionID: "tab1"})
	if len(owner.Participants) != 4 || len(state.Participants) != 4 {
		t.Fatal("canonical snapshot was mutated")
	}
}
func TestInitialStateRequiresCurrentAdmission(t *testing.T) {
	for _, test := range []struct {
		status     conferences.Status
		membership conferences.ParticipantStatus
		admission  conferences.AdmissionState
		allowed    bool
	}{
		{conferences.Active, conferences.Joined, conferences.AdmissionAdmitted, true},
		{conferences.Created, conferences.Joined, conferences.AdmissionAdmitted, true},
		{conferences.Active, conferences.Kicked, conferences.AdmissionKicked, false},
		{conferences.Active, conferences.Waiting, conferences.AdmissionWaiting, false},
		{conferences.Finished, conferences.Joined, conferences.AdmissionAdmitted, false},
		{conferences.Scheduled, conferences.Joined, conferences.AdmissionAdmitted, false},
	} {
		state := domain.State{Status: test.status, Participants: []domain.Presence{{ParticipantView: conferences.ParticipantView{ID: "self", Status: test.membership, AdmissionState: test.admission}}}}
		if stateAllows(state, "self") != test.allowed {
			t.Fatalf("incorrect initial authorization: %+v", test)
		}
	}
}
