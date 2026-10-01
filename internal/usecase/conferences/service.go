package conferences

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

type repository interface {
	Create(context.Context, domain.Conference, domain.Participant) (domain.Conference, error)
	Get(context.Context, string) (domain.Conference, error)
	GetByInvite(context.Context, string) (domain.Conference, error)
	ListForUser(context.Context, string, int, int) ([]domain.Conference, error)
	Membership(context.Context, string, string) (domain.Participant, error)
	Participants(context.Context, string, int, int) ([]domain.Participant, error)
	Transition(context.Context, string, string, domain.Status) (domain.Conference, error)
	Join(context.Context, string, users.User, string) (domain.Participant, error)
	Leave(context.Context, string, string) (domain.Participant, error)
}

type userRepository interface {
	GetByID(context.Context, string) (users.User, error)
}

type Service struct {
	repository repository
	users      userRepository
	invite     func() (string, error)
	observer   interface{ ConferenceChanged(context.Context, string) }
}

func NewService(repository repository, users userRepository, invite func() (string, error)) *Service {
	return &Service{repository: repository, users: users, invite: invite}
}

// Configure once during bootstrap, before serving any requests.
func (s *Service) SetObserver(observer interface{ ConferenceChanged(context.Context, string) }) {
	s.observer = observer
}
func (s *Service) changed(ctx context.Context, id string) {
	if s.observer != nil {
		s.observer.ConferenceChanged(ctx, id)
	}
}

func (s *Service) Create(ctx context.Context, userID string, request domain.CreateRequest) (domain.View, error) {
	if err := domain.ValidateSchedule(request.ScheduledAt, request.PlannedDurationMin, time.Now()); err != nil {
		return domain.View{}, err
	}
	title, err := domain.NormalizeTitle(request.Title)
	if err != nil {
		return domain.View{}, err
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.View{}, err
	}
	for range 5 {
		code, err := s.invite()
		if err != nil {
			return domain.View{}, err
		}
		if !validInvite(code) {
			return domain.View{}, fmt.Errorf("invalid generated invite code")
		}
		conference := domain.Conference{ID: uuid.NewString(), OwnerID: user.ID, Title: title, InviteCode: code, Status: domain.Created,
			WaitingRoomEnabled: request.WaitingRoomEnabled, ScheduledAt: request.ScheduledAt, PlannedDurationMin: request.PlannedDurationMin}
		if conference.ScheduledAt != nil {
			at := conference.ScheduledAt.UTC()
			conference.ScheduledAt = &at
			conference.Status = domain.Scheduled
		}
		ownerID := user.ID
		owner := domain.Participant{ID: uuid.NewString(), ConferenceID: conference.ID, UserID: &ownerID,
			DisplayName: user.ParticipantName(), Role: domain.Owner, Status: domain.Left, AdmissionState: domain.AdmissionAdmitted}
		created, err := s.repository.Create(ctx, conference, owner)
		if errors.Is(err, domain.ErrInviteCollision) {
			continue
		}
		if err != nil {
			return domain.View{}, err
		}
		return created.View(), nil
	}
	return domain.View{}, fmt.Errorf("unable to generate a unique invite code")
}

func (s *Service) List(ctx context.Context, userID string, limit, offset int) ([]domain.View, error) {
	items, err := s.repository.ListForUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	views := make([]domain.View, 0, len(items))
	for _, item := range items {
		view := item.View()
		view.InviteCode, view.InviteURL = "", ""
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) Read(ctx context.Context, userID, id string) (domain.View, error) {
	conference, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.View{}, err
	}
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return domain.View{}, err
	}
	view := conference.View()
	if !p.CanReadHistory() {
		view.InviteCode, view.InviteURL = "", ""
	}
	return view, nil
}

func (s *Service) Transition(ctx context.Context, userID, id string, target domain.Status) (domain.View, error) {
	conference, err := s.repository.Transition(ctx, id, userID, target)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return conference.View(), nil
}

func (s *Service) Participants(ctx context.Context, userID, id string, limit, offset int) ([]domain.ParticipantView, error) {
	if _, err := s.repository.Get(ctx, id); err != nil {
		return nil, err
	}
	actor, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if !actor.CanReadHistory() {
		return nil, apperrors.ErrForbidden
	}
	var items []domain.Participant
	if repo, ok := s.repository.(interface {
		ParticipantsVisible(context.Context, string, string, int, int) ([]domain.Participant, error)
	}); ok {
		items, err = repo.ParticipantsVisible(ctx, id, userID, limit, offset)
	} else {
		items, err = s.repository.Participants(ctx, id, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	views := make([]domain.ParticipantView, 0, len(items))
	for _, item := range items {
		if !item.IsAdmitted() && actor.Role != domain.Owner && actor.Role != domain.CoHost {
			continue
		}
		views = append(views, item.View())
	}
	return views, nil
}

func (s *Service) Join(ctx context.Context, userID, id string, request domain.JoinRequest) (domain.ParticipantView, error) {
	if request.InviteCode != "" && !validInvite(request.InviteCode) {
		return domain.ParticipantView{}, apperrors.New(apperrors.ErrInvalidInput, "invalid inviteCode")
	}
	user, err := s.currentUser(ctx, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	participant, err := s.repository.Join(ctx, id, user, request.InviteCode)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	if participant.Status == domain.Waiting {
		s.event(ctx, "participant.waiting", id, participant)
	}
	return participant.View(), nil
}

func (s *Service) Leave(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	participant, err := s.repository.Leave(ctx, id, userID)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	return participant.View(), nil
}

func (s *Service) LookupInvite(ctx context.Context, code string) (domain.InviteView, error) {
	if !validInvite(code) {
		return domain.InviteView{}, apperrors.ErrNotFound
	}
	conference, err := s.repository.GetByInvite(ctx, code)
	if err != nil {
		return domain.InviteView{}, err
	}
	return domain.InviteView{ID: conference.ID, Title: conference.Title, Status: conference.Status, WaitingRoomEnabled: conference.WaitingRoomEnabled, ScheduledAt: conference.ScheduledAt}, nil
}

func (s *Service) JoinInvite(ctx context.Context, userID, code string) (domain.ParticipantView, error) {
	invite, err := s.LookupInvite(ctx, code)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	return s.Join(ctx, userID, invite.ID, domain.JoinRequest{InviteCode: code})
}

func (s *Service) currentUser(ctx context.Context, id string) (users.User, error) {
	user, err := s.users.GetByID(ctx, id)
	if errors.Is(err, apperrors.ErrNotFound) {
		return users.User{}, apperrors.ErrUnauthorized
	}
	return user, err
}

func (s *Service) event(ctx context.Context, kind, id string, participant domain.Participant) {
	if events, ok := s.observer.(interface {
		Broadcast(context.Context, realtime.Envelope) error
	}); ok {
		_ = events.Broadcast(ctx, realtime.Event(kind, id, map[string]any{"participant": participant.View()}))
	}
}

func validInvite(code string) bool {
	if len(code) != 32 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(code)
	return err == nil && len(decoded) == 24
}
