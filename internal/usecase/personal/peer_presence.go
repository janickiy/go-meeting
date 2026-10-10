package personal

import (
	"context"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	domain "github.com/janickiy/meet-space/internal/domain/personal"
)

// ConversationReader returns the conversation projection authorized for the actor.
type ConversationReader interface {
	Get(context.Context, string, string) (domain.Conversation, error)
}

// PresenceReader reads online state without creating or extending a session.
type PresenceReader interface {
	Online(context.Context, []string) (map[string]bool, error)
}

// ReadPeerPresence authorizes a direct conversation before looking up its server-selected peer.
// The caller supplies normalized actor and conversation UUIDs. Unknown presence is
// unavailable, never offline; cancellation is checked before and after each read.
func ReadPeerPresence(ctx context.Context, conversations ConversationReader, presence PresenceReader, actor, id string) (domain.PeerPresence, error) {
	if ctx.Err() != nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	item, err := conversations.Get(ctx, actor, id)
	if err != nil {
		return domain.PeerPresence{}, err
	}
	if item.ID != id || item.Type != "direct" || item.Peer == nil {
		return domain.PeerPresence{}, apperrors.ErrForbidden
	}
	peer, err := chat.UUID(item.Peer.ID)
	if err != nil || peer == actor {
		return domain.PeerPresence{}, apperrors.ErrForbidden
	}
	if ctx.Err() != nil || presence == nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statuses, err := presence.Online(lookup, []string{peer})
	if err != nil || lookup.Err() != nil || ctx.Err() != nil {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	online, known := statuses[peer]
	if !known {
		return domain.PeerPresence{}, apperrors.ErrUnavailable
	}
	return domain.PeerPresence{ConversationID: id, PeerID: peer, Online: online}, nil
}
