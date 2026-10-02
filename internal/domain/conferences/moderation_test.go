package conferences

import "testing"

// TestModerationPermissionMatrix проверяет сценарий «Moderation Permission Matrix», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestModerationPermissionMatrix(t *testing.T) {
	roles := []Role{Owner, CoHost, ParticipantRole, Guest}
	actions := []string{"mute", "camera", "screen", "kick", "role"}
	for _, actorRole := range roles {
		for _, targetRole := range roles {
			for _, action := range actions {
				actor := Participant{ID: "actor", ConferenceID: "room", Role: actorRole, Status: Joined}
				target := Participant{ID: "target", ConferenceID: "room", Role: targetRole, Status: Joined}
				want := targetRole != Owner && (actorRole == Owner || (actorRole == CoHost && targetRole == ParticipantRole && (action == "mute" || action == "screen" || action == "kick")))
				if got := CanModerate(actor, target, action); got != want {
					t.Errorf("%s -> %s %s = %v want %v", actorRole, targetRole, action, got, want)
				}
				target.ConferenceID = "other"
				if CanModerate(actor, target, action) {
					t.Error("cross-conference moderation allowed")
				}
			}
		}
	}
	actor := Participant{ID: "actor", ConferenceID: "room", Role: Owner, Status: Left}
	if CanModerate(actor, Participant{ID: "target", ConferenceID: "room", Role: ParticipantRole}, "kick") {
		t.Fatal("absent owner can moderate")
	}
}

// TestModerationInputRejectsAmbiguousOrPrivilegedRole проверяет сценарий «Moderation вход Rejects Ambiguous Or Privileged Role», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestModerationInputRejectsAmbiguousOrPrivilegedRole(t *testing.T) {
	b := true
	for _, request := range []ModerationRequest{{Action: "role", Role: Owner}, {Action: "role", Role: CoHost, Blocked: &b}, {Action: "mute"}, {Action: "kick", Blocked: &b}, {Action: "invented"}} {
		if request.Validate() == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
	for _, request := range []ModerationRequest{{Action: "role", Role: CoHost}, {Action: "role", Role: ParticipantRole}, {Action: "mute", Blocked: &b}, {Action: "kick"}} {
		if err := request.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
