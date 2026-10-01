package realtime

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

type HandStore interface {
	Raise(context.Context, string, string) (domain.Hand, bool, error)
	Lower(context.Context, string, string) (bool, error)
	List(context.Context, string) ([]domain.Hand, error)
}
type Engagement struct {
	repo  Repository
	hands HandStore
	hub   *Hub
}

func NewEngagement(repo Repository, hands HandStore, hub *Hub) *Engagement {
	return &Engagement{repo: repo, hands: hands, hub: hub}
}
func (s *Engagement) Authorize(ctx context.Context, conferenceID, userID string) error {
	_, _, err := s.authorized(ctx, conferenceID, userID)
	return err
}
func (s *Engagement) authorized(ctx context.Context, conferenceID, userID string) (conferences.Participant, []conferences.Participant, error) {
	actor, err := s.repo.Authorize(ctx, conferenceID, userID)
	if err != nil {
		return actor, nil, err
	}
	status, roster, err := s.repo.Roster(ctx, conferenceID)
	if err != nil {
		return actor, nil, err
	}
	if status != conferences.Active {
		return actor, nil, apperrors.New(apperrors.ErrConflict, "conference is not active")
	}
	// Recheck from the newest snapshot; Authorize may have raced a kick.
	for _, p := range roster {
		if p.ID == actor.ID && p.CanParticipate() {
			return p, roster, nil
		}
	}
	return actor, nil, apperrors.ErrForbidden
}
func (s *Engagement) List(ctx context.Context, conferenceID, userID string) ([]domain.Hand, error) {
	_, roster, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return nil, err
	}
	hands, err := s.hands.List(ctx, conferenceID)
	if err != nil {
		return nil, err
	}
	eligible := map[string]bool{}
	for _, p := range roster {
		eligible[p.ID] = p.CanParticipate()
	}
	items := make([]domain.Hand, 0, len(hands))
	for _, h := range hands {
		if eligible[h.ParticipantID] {
			items = append(items, h)
		}
	}
	return items, nil
}
func (s *Engagement) Hand(ctx context.Context, conferenceID, userID, participantID string, raised bool) (*domain.Hand, error) {
	actor, roster, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return nil, err
	}
	var target *conferences.Participant
	for _, p := range roster {
		if p.ID == participantID {
			copy := p
			target = &copy
			break
		}
	}
	if target == nil || !target.CanParticipate() {
		return nil, apperrors.ErrNotFound
	}
	if actor.ID != participantID {
		if raised || !actor.CanAdmit() || (actor.Role == conferences.CoHost && target.Role != conferences.ParticipantRole) {
			return nil, apperrors.ErrForbidden
		}
	}
	if raised {
		hand, changed, err := s.hands.Raise(ctx, conferenceID, participantID)
		if err != nil {
			return nil, err
		}
		if changed {
			_ = s.hub.Broadcast(ctx, domain.Event("hand.raised", conferenceID, hand))
		}
		return &hand, nil
	}
	changed, err := s.hands.Lower(ctx, conferenceID, participantID)
	if err != nil {
		return nil, err
	}
	if changed {
		_ = s.hub.Broadcast(ctx, domain.Event("hand.lowered", conferenceID, map[string]string{"participantId": participantID}))
	}
	return nil, nil
}
func (s *Engagement) Reaction(ctx context.Context, conferenceID, userID, emoji string) error {
	if !domain.AllowedReaction(emoji) {
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported reaction")
	}
	actor, _, err := s.authorized(ctx, conferenceID, userID)
	if err != nil {
		return err
	}
	return s.hub.Broadcast(ctx, domain.Event("reaction.created", conferenceID, map[string]string{"participantId": actor.ID, "emoji": emoji}))
}
