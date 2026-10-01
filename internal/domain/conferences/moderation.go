package conferences

import "github.com/janickiy/go-recorder/internal/domain/apperrors"

type MediaState struct {
	ConnectionID      string `json:"connectionId"`
	Sequence          int64  `json:"sequence"`
	MicrophoneEnabled bool   `json:"microphoneEnabled"`
	CameraEnabled     bool   `json:"cameraEnabled"`
	ScreenSharing     bool   `json:"screenSharing"`
}

type ModerationRequest struct {
	Action  string `json:"action"`
	Blocked *bool  `json:"blocked,omitempty"`
	Role    Role   `json:"role,omitempty"`
}

// Owner is immutable. Co-hosts may moderate ordinary participants only, and
// cannot grant camera policy or recording privileges.
func CanModerate(actor, target Participant, action string) bool {
	switch action {
	case "mute", "camera", "screen", "kick", "role":
	default:
		return false
	}
	if !actor.CanParticipate() || actor.ConferenceID != target.ConferenceID || target.Role == Owner || actor.ID == target.ID {
		return false
	}
	// Pending people are private to the waiting room and must be handled via
	// admit/reject. General moderation events are visible to the admitted room.
	// Preserve idempotency for a retry of an already-applied kick.
	if !target.CanReadHistory() && !(action == "kick" && target.Status == Kicked) {
		return false
	}
	if actor.Role == Owner {
		return true
	}
	return actor.Role == CoHost && target.Role == ParticipantRole && (action == "mute" || action == "screen" || action == "kick")
}

func (r ModerationRequest) Validate() error {
	switch r.Action {
	case "mute", "camera", "screen":
		if r.Blocked == nil || r.Role != "" {
			return apperrors.New(apperrors.ErrInvalidInput, "blocked is required for media moderation")
		}
	case "kick":
		if r.Blocked != nil || r.Role != "" {
			return apperrors.ErrInvalidInput
		}
	case "role":
		if r.Blocked != nil || (r.Role != CoHost && r.Role != ParticipantRole) {
			return apperrors.New(apperrors.ErrInvalidInput, "role must be co_host or participant")
		}
	default:
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported moderation action")
	}
	return nil
}
