package conferences

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type ControlRepository interface {
	Moderate(context.Context, string, string, string, domain.ModerationRequest) (domain.Participant, error)
	UpdateMediaState(context.Context, string, string, domain.MediaState) (domain.Participant, error)
	ReconcileParticipants(context.Context, string, int) ([]domain.Participant, error)
}
type PolicyController interface {
	SetParticipantPolicy(context.Context, string, string, media.ParticipantPolicy) error
}
type ControlEvents interface {
	Broadcast(context.Context, realtime.Envelope) error
	ConferenceChanged(context.Context, string)
}
type ControlService struct {
	repo   ControlRepository
	media  PolicyController
	events ControlEvents
}

func NewControlService(repo ControlRepository, controller PolicyController, events ControlEvents) *ControlService {
	return &ControlService{repo: repo, media: controller, events: events}
}

func (s *ControlService) Moderate(ctx context.Context, userID, conferenceID, participantID string, request domain.ModerationRequest) (domain.ParticipantView, error) {
	p, err := s.repo.Moderate(ctx, conferenceID, userID, participantID, request)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		kind := "participant.media.updated"
		if request.Action == "role" {
			kind = "participant.role.updated"
		}
		if request.Action == "kick" {
			kind = "participant.kicked"
		}
		_ = s.events.Broadcast(ctx, realtime.Event(kind, conferenceID, map[string]any{"participant": p.View()}))
		s.events.ConferenceChanged(ctx, conferenceID)
	}
	slog.Info("conference moderation", "conference_id", conferenceID, "actor_user_id", userID, "participant_id", participantID, "action", request.Action, "policy_version", p.MediaPolicyVersion)
	if s.media != nil {
		if err = s.media.SetParticipantPolicy(ctx, conferenceID, participantID, policy(p)); err != nil {
			return p.View(), apperrors.New(apperrors.ErrUnavailable, "moderation saved; media enforcement is retrying")
		}
	}
	return p.View(), nil
}

func (s *ControlService) UpdateMediaState(ctx context.Context, userID, conferenceID string, state domain.MediaState) (domain.ParticipantView, error) {
	p, err := s.repo.UpdateMediaState(ctx, conferenceID, userID, state)
	if err != nil {
		return domain.ParticipantView{}, err
	}
	if s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", conferenceID, map[string]any{"participant": p.View()}))
	}
	return p.View(), nil
}

// Wire after session closure in Hub.Unregister, alongside media cleanup.
func (s *ControlService) Disconnected(ctx context.Context, session realtime.Session) {
	repo, ok := s.repo.(interface {
		ClearDisconnectedMedia(context.Context, string, string) (domain.Participant, bool, error)
	})
	if !ok {
		return
	}
	p, changed, err := repo.ClearDisconnectedMedia(ctx, session.ConferenceID, session.ParticipantID)
	if err == nil && changed && s.events != nil {
		_ = s.events.Broadcast(ctx, realtime.Event("participant.media.updated", session.ConferenceID, map[string]any{"participant": p.View()}))
	}
}

func policy(p domain.Participant) media.ParticipantPolicy {
	return media.ParticipantPolicy{Version: p.MediaPolicyVersion, MicrophoneBlocked: p.MicrophoneBlocked, CameraBlocked: p.CameraBlocked, ScreenBlocked: p.ScreenBlocked, Kicked: p.Status != domain.Joined}
}

// Repairs a committed policy after a transient worker/Redis failure. Every API
// instance may run this: worker policy versions make duplicate application safe.
func (s *ControlService) Run(ctx context.Context) {
	if s.media == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	after := uuid.Nil.String()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		query, cancel := context.WithTimeout(ctx, 3*time.Second)
		rows, err := s.repo.ReconcileParticipants(query, after, 64)
		cancel()
		if err != nil {
			continue
		}
		if len(rows) < 64 {
			after = uuid.Nil.String()
		} else {
			after = rows[len(rows)-1].ID
		}
		var workers sync.WaitGroup
		jobs := make(chan domain.Participant)
		for range min(8, len(rows)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for p := range jobs {
					op, done := context.WithTimeout(ctx, 2*time.Second)
					_ = s.media.SetParticipantPolicy(op, p.ConferenceID, p.ID, policy(p))
					done()
				}
			}()
		}
		for _, p := range rows {
			select {
			case jobs <- p:
			case <-ctx.Done():
				close(jobs)
				workers.Wait()
				return
			}
		}
		close(jobs)
		workers.Wait()
	}
}
