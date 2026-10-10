package conferences

import (
	"context"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/users"
)

type GuestRepository interface {
	JoinGuest(context.Context, string, string, string) (users.User, domain.Participant, error)
}

type GuestTokens interface {
	IssueGuest(string, string) (string, error)
}

type GuestSession struct {
	users.LoginResponse
	Item domain.ParticipantView `json:"item"`
}

type GuestService struct {
	Repository GuestRepository
	Tokens     GuestTokens
	Observer   interface{ ConferenceChanged(context.Context, string) }
}

// Join creates or resumes a guest only after an explicit invitation join.
func (s *GuestService) Join(ctx context.Context, code, name, resumeUserID string) (GuestSession, error) {
	if !validInvite(code) {
		return GuestSession{}, apperrors.ErrNotFound
	}
	name, err := users.NormalizeDisplayName(name)
	if err != nil {
		return GuestSession{}, err
	}
	user, participant, err := s.Repository.JoinGuest(ctx, code, name, resumeUserID)
	if err != nil {
		return GuestSession{}, err
	}
	token, err := s.Tokens.IssueGuest(user.ID, participant.ConferenceID)
	if err != nil {
		return GuestSession{}, err
	}
	if s.Observer != nil {
		s.Observer.ConferenceChanged(ctx, participant.ConferenceID)
	}
	return GuestSession{LoginResponse: users.LoginResponse{Status: "success", AccessToken: token, TokenType: "Bearer", ExpiresIn: 3600, User: user.View()}, Item: participant.View()}, nil
}
