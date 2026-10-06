package chat

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	chatdomain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type ActionRepository interface {
	ChatInfo(context.Context, string, string) (chatdomain.Info, error)
	ChatMembers(context.Context, string, string, string, int) ([]chatdomain.MemberView, string, error)
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

type AccountPresence interface {
	Online(context.Context, []string) (map[string]bool, error)
}

type ConferencePresence interface {
	GetActiveSessions(context.Context, string) ([]realtime.Session, error)
}

type ActionService struct {
	repo     ActionRepository
	observer actionObserver
	presence AccountPresence
	guests   ConferencePresence
}

func NewActionService(repo ActionRepository, observer actionObserver) *ActionService {
	return &ActionService{repo: repo, observer: observer}
}

func (s *ActionService) WithPresence(accounts AccountPresence, guests ConferencePresence) *ActionService {
	s.presence, s.guests = accounts, guests
	return s
}

func (s *ActionService) Info(ctx context.Context, user, conference string) (chatdomain.Info, error) {
	return s.repo.ChatInfo(ctx, user, conference)
}

func (s *ActionService) Members(ctx context.Context, user, conference, cursor string, limit int) ([]chatdomain.MemberView, string, error) {
	if limit < 1 || limit > 100 {
		return nil, "", apperrors.ErrInvalidInput
	}
	// Authorization and page bounds precede Redis access: this is not a user directory.
	items, next, err := s.repo.ChatMembers(ctx, user, conference, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if len(items) == 0 {
		return items, next, nil
	}
	ids, hasGuests := []string{}, false
	for _, item := range items {
		if item.UserID == nil {
			continue
		}
		if item.IsGuest {
			hasGuests = true
		} else {
			ids = append(ids, *item.UserID)
		}
	}
	lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	statuses := map[string]bool{}
	accountAvailable := s.presence != nil
	if len(ids) > 0 && accountAvailable {
		statuses, err = s.presence.Online(lookup, ids)
		accountAvailable = err == nil
	}
	guestStatuses := map[string]bool{}
	guestAvailable := s.guests != nil
	if hasGuests && guestAvailable {
		sessions, err := s.guests.GetActiveSessions(lookup, conference)
		guestAvailable = err == nil
		if guestAvailable {
			for _, session := range sessions {
				if session.ConferenceID == conference {
					guestStatuses[session.UserID] = true
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	for i := range items {
		// A presence outage must not turn a valid PG member into a false offline status.
		items[i].Online = nil
		if items[i].UserID == nil {
			continue
		}
		if items[i].IsGuest {
			if guestAvailable {
				online := guestStatuses[*items[i].UserID]
				items[i].Online = &online
			}
		} else if accountAvailable {
			online := statuses[*items[i].UserID]
			items[i].Online = &online
		}
	}
	return items, next, nil
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
