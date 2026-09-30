package conferences

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

func TestCreateRetriesInviteCollisionAndDoesNotJoinOwner(t *testing.T) {
	userID := uuid.NewString()
	repo := &collisionRepository{}
	service := NewService(repo, &testUserRepository{id: userID}, func() (string, error) { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil })
	view, err := service.Create(context.Background(), userID, domain.CreateRequest{Title: " Test meeting "})
	if err != nil {
		t.Fatal(err)
	}
	if repo.attempts != 2 || view.OwnerID != userID || view.Title != "Test meeting" {
		t.Fatal("invite retry or authenticated ownership failed")
	}
	if repo.owner.Role != domain.Owner || repo.owner.JoinedAt != nil || repo.owner.LeftAt != nil || repo.owner.Status != domain.Left {
		t.Fatal("owner membership incorrectly represents an actual join")
	}
}

type collisionRepository struct {
	repository
	attempts int
	owner    domain.Participant
}

func (r *collisionRepository) Create(_ context.Context, conference domain.Conference, owner domain.Participant) (domain.Conference, error) {
	r.attempts++
	r.owner = owner
	if r.attempts == 1 {
		return domain.Conference{}, domain.ErrInviteCollision
	}
	return conference, nil
}

type testUserRepository struct{ id string }

func (r *testUserRepository) GetByID(_ context.Context, id string) (users.User, error) {
	if id != r.id {
		return users.User{}, apperrors.ErrNotFound
	}
	return users.User{ID: id, Email: "test@example.com"}, nil
}
