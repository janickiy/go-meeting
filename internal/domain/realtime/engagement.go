package realtime

import "time"

type Hand struct {
	ParticipantID string    `json:"participantId"`
	RaisedAt      time.Time `json:"raisedAt"`
}

func AllowedReaction(emoji string) bool {
	return emoji == "👍" || emoji == "👏" || emoji == "❤️" || emoji == "😂"
}

// Recoverable/ephemeral collaboration events use a lossy, bounded socket queue.
// Chat's durable HTTP history and hand snapshots repair missed events. Signaling,
// moderation, state transitions and recording status retain the reliable queue.
func LowPriorityEvent(kind string) bool {
	switch kind {
	case "reaction.created", "hand.raised", "hand.lowered", "chat.message.created", "chat.message.updated", "chat.message.deleted", "chat.read.updated":
		return true
	default:
		return false
	}
}
