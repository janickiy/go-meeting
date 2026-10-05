package conferences

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	d "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

type InvitationRepository interface {
	SearchInvitationUsers(context.Context, string, string, string) ([]d.InvitationUser, error)
	Invite(context.Context, string, string, d.InvitationRequest) ([]d.InvitationResult, error)
}

type InvitationService struct{ Repository InvitationRepository }

func (s *InvitationService) Search(ctx context.Context, actor, conference, query string) ([]d.InvitationUser, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 100 characters")
	}
	return s.Repository.SearchInvitationUsers(ctx, actor, conference, query)
}

func (s *InvitationService) Invite(ctx context.Context, actor, conference string, request d.InvitationRequest) ([]d.InvitationResult, error) {
	if len(request.Emails)+len(request.UserIDs) == 0 || len(request.Emails)+len(request.UserIDs) > 20 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "select 1 to 20 recipients")
	}
	clean := d.InvitationRequest{}
	seenEmails, seenUsers := map[string]bool{}, map[string]bool{}
	for _, email := range request.Emails {
		email = users.NormalizeEmail(email)
		if err := d.ValidateInvitationEmail(email); err != nil {
			return nil, err
		}
		if !seenEmails[email] {
			clean.Emails = append(clean.Emails, email)
			seenEmails[email] = true
		}
	}
	for _, id := range request.UserIDs {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return nil, apperrors.ErrInvalidInput
		}
		id = parsed.String()
		if !seenUsers[id] {
			clean.UserIDs = append(clean.UserIDs, id)
			seenUsers[id] = true
		}
	}
	return s.Repository.Invite(ctx, actor, conference, clean)
}
