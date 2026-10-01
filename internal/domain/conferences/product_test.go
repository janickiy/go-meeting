package conferences

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAdmissionAuthorization(t *testing.T) {
	for _, state := range []AdmissionState{AdmissionWaiting, AdmissionAdmitted, AdmissionRejected, AdmissionKicked} {
		for _, status := range []ParticipantStatus{Joined, Left, Waiting, Rejected, Kicked} {
			p := Participant{AdmissionState: state, Status: status, Role: Owner}
			read := state == AdmissionAdmitted && (status == Joined || status == Left)
			if p.CanReadHistory() != read || p.CanParticipate() != (read && status == Joined) || p.CanAdmit() != (read && status == Joined) {
				t.Fatalf("incorrect authorization %s/%s", state, status)
			}
		}
	}
	p := Participant{Status: Joined, AdmissionState: AdmissionAdmitted, Role: ParticipantRole}
	if p.CanAdmit() {
		t.Fatal("ordinary participant can admit")
	}
	p.Role = CoHost
	if !p.CanAdmit() {
		t.Fatal("co-host cannot admit")
	}
	for _, decision := range []string{"admit", "reject"} {
		if (AdmissionRequest{Decision: decision}).Validate() != nil {
			t.Fatal(decision)
		}
	}
	if (AdmissionRequest{Decision: "join"}).Validate() == nil {
		t.Fatal("accepted invalid admission decision")
	}
	owner := Participant{ID: "owner", ConferenceID: "room", Role: Owner, Status: Joined, AdmissionState: AdmissionAdmitted}
	waiter := Participant{ID: "waiting", ConferenceID: "room", Role: ParticipantRole, Status: Waiting, AdmissionState: AdmissionWaiting}
	for _, action := range []string{"mute", "camera", "screen", "kick", "role"} {
		if CanModerate(owner, waiter, action) {
			t.Fatalf("pending target accepted general moderation %s", action)
		}
	}
	waiter.Status, waiter.AdmissionState = Kicked, AdmissionKicked
	if !CanModerate(owner, waiter, "kick") {
		t.Fatal("duplicate kick must remain idempotent")
	}
}

func TestScheduleAndTimelineValidation(t *testing.T) {
	for _, target := range []Status{Active, Cancelled} {
		if !CanTransition(Scheduled, target) {
			t.Fatal("scheduled transition denied")
		}
	}
	for _, target := range []Status{Created, Scheduled, Finished} {
		if CanTransition(Scheduled, target) {
			t.Fatal("invalid scheduled transition allowed")
		}
	}
	now := time.Now().UTC()
	future, past, duration := now.Add(time.Hour), now.Add(-time.Hour), 30
	if ValidateSchedule(&future, &duration, now) != nil || ValidateSchedule(nil, nil, now) != nil {
		t.Fatal("valid schedule denied")
	}
	if ValidateSchedule(&past, nil, now) == nil || ValidateSchedule(nil, &duration, now) == nil {
		t.Fatal("invalid schedule accepted")
	}
	duration = 1441
	if ValidateSchedule(&future, &duration, now) == nil {
		t.Fatal("invalid duration accepted")
	}
	c := Conference{ID: uuid.NewString(), CreatedAt: now, ScheduledAt: &future}
	encoded := EncodeTimelineCursor(c)
	decoded, err := DecodeTimelineCursor(encoded)
	if err != nil || decoded.ID != c.ID || !decoded.At.Equal(future) {
		t.Fatalf("cursor roundtrip: %+v %v", decoded, err)
	}
	query := TimelineQuery{View: "upcoming", Scope: "all", Limit: 100, Cursor: encoded}
	if query.Validate() != nil {
		t.Fatal("valid cursor query denied")
	}
	query.Cursor = "bad"
	if query.Validate() == nil {
		t.Fatal("invalid cursor accepted")
	}
}
