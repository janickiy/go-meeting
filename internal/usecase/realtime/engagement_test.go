package realtime

import (
	"encoding/json"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

// TestStateVisibilityIsPerRecipient проверяет индивидуальную видимость состояния для каждого получателя.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStateVisibilityIsPerRecipient(t *testing.T) {
	payload, err := json.Marshal(domain.State{})
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(payload, &fields) != nil {
		t.Fatal("invalid state JSON", err)
	}
	if _, exists := fields["hands"]; exists {
		t.Fatal("removed feature leaked into state")
	}
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

// TestInitialStateRequiresCurrentAdmission проверяет сценарий «Initial состояние Requires текущий допуск», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
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
