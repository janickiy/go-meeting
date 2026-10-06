package conferences

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
)

type invitationValidationRepository struct {
	called  bool
	request domain.InvitationRequest
}

func (r *invitationValidationRepository) SearchInvitationUsers(context.Context, string, string, string) ([]domain.InvitationUser, error) {
	return nil, nil
}
func (r *invitationValidationRepository) Invite(_ context.Context, _, _ string, request domain.InvitationRequest) ([]domain.InvitationResult, error) {
	r.called = true
	r.request = request
	return []domain.InvitationResult{}, nil
}

func TestInvitationServiceRejectsInternationalAddressBeforeQueue(t *testing.T) {
	repo := &invitationValidationRepository{}
	service := &InvitationService{Repository: repo}
	for _, email := range []string{"тест@example.org", "member@пример.рф"} {
		_, err := service.Invite(context.Background(), "actor", "room", domain.InvitationRequest{Emails: []string{email}})
		if !errors.Is(err, apperrors.ErrInvalidInput) || !strings.Contains(err.Error(), "Международные адреса") || repo.called {
			t.Fatal("internationalized mail entered queue", err)
		}
	}
	if _, err := service.Invite(context.Background(), "actor", "room", domain.InvitationRequest{Emails: []string{" Member@Example.org "}}); err != nil || !repo.called || len(repo.request.Emails) != 1 || repo.request.Emails[0] != "member@example.org" {
		t.Fatal("normal account address changed", repo.request, err)
	}
}
