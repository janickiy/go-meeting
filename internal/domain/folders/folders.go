package folders

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const MaxFolders = 100

type Folder struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Position          int       `json:"position"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	ItemCount         int64     `json:"itemCount"`
	ConversationCount int64     `json:"conversationCount"`
	ConferenceCount   int64     `json:"conferenceCount"`
	Contains          *bool     `json:"contains,omitempty" gorm:"-"`
}

// Invite fields are always empty here; a folder never grants invitation access.
type ConferenceView struct {
	conferences.View
	ParticipantCount *int64 `json:"participantCount,omitempty"`
}
type Item struct {
	Type     string `json:"type"`
	Item     any    `json:"item"`
	InFolder *bool  `json:"inFolder,omitempty"`
}
type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}
type Filter struct {
	Type   string
	Search string
}

func NormalizeName(raw string) (name, key string, err error) {
	name, err = normalizeText(raw, 50, true)
	if err != nil {
		return "", "", err
	}
	return name, norm.NFC.String(cases.Fold().String(name)), nil
}
func NormalizeFilter(filter Filter) (Filter, error) {
	if filter.Type == "" {
		filter.Type = "all"
	}
	if filter.Type != "all" && !ValidKind(filter.Type) {
		return filter, apperrors.ErrInvalidInput
	}
	var err error
	filter.Search, err = normalizeText(filter.Search, 100, false)
	return filter, err
}
func ValidKind(kind string) bool { return kind == "conversation" || kind == "conference" }

func normalizeText(raw string, maximum int, required bool) (string, error) {
	if !utf8.ValidString(raw) {
		return "", apperrors.ErrInvalidInput
	}
	text := norm.NFC.String(strings.TrimSpace(raw))
	if utf8.RuneCountInString(text) > maximum || (required && text == "") {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid folder name or search length")
	}
	for _, r := range text {
		joiner := r == '\u200c' || r == '\u200d'
		if unicode.IsControl(r) || (unicode.Is(unicode.Cf, r) && !joiner) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "unsupported control characters")
		}
	}
	return text, nil
}
