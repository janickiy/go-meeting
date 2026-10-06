package chat

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	chatdomain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
)

type ActionRepository interface {
	ChatInfo(context.Context, string, string) (chatdomain.Info, error)
	ChatMembers(context.Context, string, string, string, int) ([]conferences.ParticipantView, string, error)
	UpdateChatInfo(context.Context, string, string, chatdomain.UpdateInfoRequest) (chatdomain.Info, error)
	ChatPreferences(context.Context, string, string) (chatdomain.Preferences, error)
	SetChatPreferences(context.Context, string, string, bool) (chatdomain.Preferences, error)
	LeaveChat(context.Context, string, string) error
	SearchMessages(context.Context, string, string, string, string, int) (chatdomain.MessagePage, error)
	ChatMessageContext(context.Context, string, string, string) (chatdomain.Page, error)
	ChatMaterials(context.Context, string, string, string, string, int) (chatdomain.MaterialPage, error)
	ChatPins(context.Context, string, string, string, int) (chatdomain.MessagePage, error)
	SetChatPin(context.Context, string, string, string, bool) error
}

type actionObserver interface{ ConferenceChanged(context.Context, string) }

type ActionService struct {
	repo     ActionRepository
	observer actionObserver
}

func NewActionService(repo ActionRepository, observer actionObserver) *ActionService {
	return &ActionService{repo: repo, observer: observer}
}

func (s *ActionService) Info(ctx context.Context, user, conference string) (chatdomain.Info, error) {
	return s.repo.ChatInfo(ctx, user, conference)
}

func (s *ActionService) Members(ctx context.Context, user, conference, cursor string, limit int) ([]conferences.ParticipantView, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", apperrors.ErrInvalidInput
	}
	return s.repo.ChatMembers(ctx, user, conference, cursor, limit)
}

func (s *ActionService) UpdateInfo(ctx context.Context, user, conference string, request chatdomain.UpdateInfoRequest) (chatdomain.Info, error) {
	if request.Title == nil && request.Description == nil {
		return chatdomain.Info{}, apperrors.ErrInvalidInput
	}
	if request.Title != nil {
		title, err := conferences.NormalizeTitle(*request.Title)
		if err != nil {
			return chatdomain.Info{}, err
		}
		request.Title = &title
	}
	if request.Description != nil {
		value := strings.TrimSpace(*request.Description)
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 1000 {
			return chatdomain.Info{}, apperrors.New(apperrors.ErrInvalidInput, "description must be at most 1000 characters")
		}
		request.Description = &value
	}
	info, err := s.repo.UpdateChatInfo(ctx, user, conference, request)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return info, err
}

func (s *ActionService) Preferences(ctx context.Context, user, conference string) (chatdomain.Preferences, error) {
	return s.repo.ChatPreferences(ctx, user, conference)
}

func (s *ActionService) SetPreferences(ctx context.Context, user, conference string, enabled bool) (chatdomain.Preferences, error) {
	return s.repo.SetChatPreferences(ctx, user, conference, enabled)
}

func (s *ActionService) Leave(ctx context.Context, user, conference string) error {
	err := s.repo.LeaveChat(ctx, user, conference)
	if err == nil && s.observer != nil {
		s.observer.ConferenceChanged(ctx, conference)
	}
	return err
}

func (s *ActionService) Search(ctx context.Context, user, conference, query, cursor string, limit int) (chatdomain.MessagePage, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 200 {
		return chatdomain.MessagePage{}, apperrors.New(apperrors.ErrInvalidInput, "query must contain 2 to 200 characters")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.SearchMessages(ctx, user, conference, query, cursor, limit)
}

func (s *ActionService) Context(ctx context.Context, user, conference, messageID string) (chatdomain.Page, error) {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return chatdomain.Page{}, err
	}
	return s.repo.ChatMessageContext(ctx, user, conference, id)
}

func (s *ActionService) Materials(ctx context.Context, user, conference, kind, cursor string, limit int) (chatdomain.MaterialPage, error) {
	if kind != "image" && kind != "file" && kind != "link" {
		return chatdomain.MaterialPage{}, apperrors.New(apperrors.ErrInvalidInput, "kind must be image, file or link")
	}
	if limit < 1 || limit > 50 {
		return chatdomain.MaterialPage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatMaterials(ctx, user, conference, kind, cursor, limit)
}

func (s *ActionService) Pins(ctx context.Context, user, conference, cursor string, limit int) (chatdomain.MessagePage, error) {
	if limit < 1 || limit > 50 {
		return chatdomain.MessagePage{}, apperrors.ErrInvalidInput
	}
	return s.repo.ChatPins(ctx, user, conference, cursor, limit)
}

func (s *ActionService) SetPin(ctx context.Context, user, conference, messageID string, important bool) error {
	id, err := chatdomain.UUID(messageID)
	if err != nil {
		return err
	}
	return s.repo.SetChatPin(ctx, user, conference, id, important)
}
