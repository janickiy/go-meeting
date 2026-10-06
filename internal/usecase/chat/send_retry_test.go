package chat

import (
	"context"
	"testing"

	"github.com/google/uuid"
	domain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type retryRepository struct {
	Repository
	message domain.Message
	sends   int
}

func (r *retryRepository) Send(_ context.Context, _, _ string, request domain.SendRequest, _ string) (domain.Message, bool, error) {
	r.sends++
	r.message.ClientRequestID = request.ClientRequestID
	return r.message, r.sends == 1, nil
}

type retryEvents struct{ published int }

func (e *retryEvents) Broadcast(context.Context, realtime.Envelope) error { e.published++; return nil }
func (*retryEvents) SendToParticipant(context.Context, string, string, realtime.Envelope) error {
	return nil
}

func TestIdempotentSendRetryDoesNotCreateAnotherRealtimeNotification(t *testing.T) {
	request := domain.SendRequest{ClientRequestID: uuid.NewString(), Text: "Message"}
	repo := &retryRepository{message: domain.Message{ID: uuid.NewString(), ConversationID: uuid.NewString()}}
	events := &retryEvents{}
	s := &Service{repo: repo, events: events}
	first, created, err := s.Send(context.Background(), "user", "conversation", request)
	if err != nil || !created {
		t.Fatal("first persist failed", err)
	}
	second, created, err := s.Send(context.Background(), "user", "conversation", request)
	if err != nil || created || first.ID != second.ID {
		t.Fatal("retry changed message identity", err)
	}
	if events.published != 1 {
		t.Fatal("retry created duplicate notification", events.published)
	}
}
