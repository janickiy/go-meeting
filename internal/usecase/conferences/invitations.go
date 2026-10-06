package conferences

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

type InvitationRepository interface {
	SearchInvitationUsers(context.Context, string, string, string) ([]domain.InvitationUser, error)
	Invite(context.Context, string, string, domain.InvitationRequest) ([]domain.InvitationResult, error)
}

type InvitationService struct{ Repository InvitationRepository }

func (s *InvitationService) Search(ctx context.Context, actorID, conferenceID, query string) ([]domain.InvitationUser, error) {
	query = strings.TrimSpace(query)
	queryLength := utf8.RuneCountInString(query)
	if !utf8.ValidString(query) || queryLength < 2 || queryLength > 100 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 100 characters")
	}
	return s.Repository.SearchInvitationUsers(ctx, actorID, conferenceID, query)
}

func (s *InvitationService) Invite(ctx context.Context, actorID, conferenceID string, request domain.InvitationRequest) ([]domain.InvitationResult, error) {
	// The limit applies to submitted recipients, before deduplication.
	recipientCount := len(request.Emails) + len(request.UserIDs)
	if recipientCount == 0 || recipientCount > 20 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "select 1 to 20 recipients")
	}
	normalized := domain.InvitationRequest{}
	seenEmails, seenUserIDs := map[string]bool{}, map[string]bool{}
	for _, email := range request.Emails {
		email = users.NormalizeEmail(email)
		if err := domain.ValidateInvitationEmail(email); err != nil {
			return nil, err
		}
		if !seenEmails[email] {
			normalized.Emails = append(normalized.Emails, email)
			seenEmails[email] = true
		}
	}
	for _, userID := range request.UserIDs {
		parsed, err := uuid.Parse(userID)
		if err != nil || parsed == uuid.Nil {
			return nil, apperrors.ErrInvalidInput
		}
		userID = parsed.String()
		if !seenUserIDs[userID] {
			normalized.UserIDs = append(normalized.UserIDs, userID)
			seenUserIDs[userID] = true
		}
	}
	return s.Repository.Invite(ctx, actorID, conferenceID, normalized)
}
