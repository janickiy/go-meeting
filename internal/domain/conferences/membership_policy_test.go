package conferences

import "testing"

func TestMembershipPolicyForStoredAndRealtimeViews(t *testing.T) {
	for _, status := range []ParticipantStatus{Joined, Left, Waiting, Rejected, Kicked, "", "unknown"} {
		for _, admission := range []AdmissionState{AdmissionAdmitted, AdmissionWaiting, AdmissionRejected, AdmissionKicked, "", "unknown"} {
			p := Participant{Status: status, AdmissionState: admission}
			view := ParticipantView{Status: status, AdmissionState: admission}
			admitted := admission == AdmissionAdmitted || admission == ""
			live := admitted && status == Joined
			history := admitted && (status == Joined || status == Left)
			if p.CanParticipate() != live || view.CanParticipate() != live || p.CanReadHistory() != history || view.CanReadHistory() != history {
				t.Fatalf("status=%q admission=%q: stored=%v/%v view=%v/%v expected=%v/%v", status, admission, p.CanParticipate(), p.CanReadHistory(), view.CanParticipate(), view.CanReadHistory(), live, history)
			}
		}
	}
}
