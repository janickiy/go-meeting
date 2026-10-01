package conferences

import (
	"context"
	"errors"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
)

type productRepository interface {
	DecideAdmission(context.Context, string, string, string, domain.AdmissionRequest) (domain.Participant, error)
	UpdateSchedule(context.Context, string, string, domain.ScheduleRequest) (domain.Conference, error)
	Timeline(context.Context, string, domain.TimelineQuery) ([]domain.Conference, error)
	History(context.Context, string, string) (domain.HistoryView, error)
}

func (s *Service) Self(ctx context.Context, userID, id string) (domain.ParticipantView, error) {
	p, err := s.repository.Membership(ctx, id, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		err = apperrors.ErrForbidden
	}
	return p.View(), err
}
func (s *Service) Admission(ctx context.Context, userID, id, participantID string, request domain.AdmissionRequest) (domain.ParticipantView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.ParticipantView{}, apperrors.ErrUnavailable
	}
	p, err := repo.DecideAdmission(ctx, id, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	s.changed(ctx, id)
	kind := "participant.admitted"
	if request.Decision == "reject" {
		kind = "participant.rejected"
	}
	s.event(ctx, kind, id, p)
	return p.View(), nil
}
func (s *Service) Schedule(ctx context.Context, userID, id string, request domain.ScheduleRequest) (domain.View, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.View{}, apperrors.ErrUnavailable
	}
	c, err := repo.UpdateSchedule(ctx, id, userID, request)
	if err != nil {
		return domain.View{}, err
	}
	s.changed(ctx, id)
	return c.View(), nil
}
func (s *Service) Timeline(ctx context.Context, userID string, query domain.TimelineQuery) (domain.TimelinePage, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.TimelinePage{}, apperrors.ErrUnavailable
	}
	rows, err := repo.Timeline(ctx, userID, query)
	if err != nil {
		return domain.TimelinePage{}, err
	}
	page := domain.TimelinePage{Items: []domain.View{}}
	if len(rows) > query.Limit {
		rows = rows[:query.Limit]
		cursor := domain.EncodeTimelineCursor(rows[len(rows)-1])
		page.NextCursor = &cursor
	}
	for _, c := range rows {
		// Invitations are not needed for list browsing. Keep pending membership
		// private without adding a membership lookup for every conference.
		view := c.View()
		view.InviteCode, view.InviteURL = "", ""
		page.Items = append(page.Items, view)
	}
	return page, nil
}
func (s *Service) History(ctx context.Context, userID, id string) (domain.HistoryView, error) {
	repo, ok := s.repository.(productRepository)
	if !ok {
		return domain.HistoryView{}, apperrors.ErrUnavailable
	}
	return repo.History(ctx, id, userID)
}
