package personal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
)

type Role string

const (
	Owner           Role = "owner"
	Admin           Role = "admin"
	MemberRole      Role = "member"
	MaxGroupMembers      = 100
)

type Member struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Role        Role   `json:"role"`
	Online      *bool  `json:"online" gorm:"-"`
}

type CreateGroupRequest struct {
	ClientRequestID string   `json:"clientRequestId"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	MemberIDs       []string `json:"memberIds"`
}
type UpdateGroupRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}
type ListFilter struct {
	Type       string
	UnreadOnly bool
	Search     string
}

func normalizeMetadata(text string, maximum int, required bool) (string, error) {
	text = strings.TrimSpace(text)
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > maximum || (required && text == "") {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid group name or description length")
	}
	for _, r := range text {
		joiner := r == '\u200c' || r == '\u200d'
		lineBreak := !required && (r == '\n' || r == '\r')
		if (unicode.IsControl(r) && !lineBreak) || (unicode.Is(unicode.Cf, r) && !joiner) {
			return "", apperrors.New(apperrors.ErrInvalidInput, "group metadata contains unsupported control characters")
		}
	}
	return text, nil
}

func NormalizeMemberIDs(ids []string) ([]string, error) {
	if len(ids) > MaxGroupMembers {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "at most 100 group members are supported")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, raw := range ids {
		id, err := chat.UUID(raw)
		if err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result, nil
}

func NormalizeCreateGroup(actor string, request CreateGroupRequest) (CreateGroupRequest, string, error) {
	var err error
	request.ClientRequestID, err = chat.UUID(request.ClientRequestID)
	if err != nil {
		return request, "", err
	}
	request.Name, err = normalizeMetadata(request.Name, 50, true)
	if err != nil {
		return request, "", err
	}
	request.Description, err = normalizeMetadata(request.Description, 200, false)
	if err != nil {
		return request, "", err
	}
	ids, err := NormalizeMemberIDs(request.MemberIDs)
	if err != nil {
		return request, "", err
	}
	request.MemberIDs = []string{}
	for _, id := range ids {
		if id != actor {
			request.MemberIDs = append(request.MemberIDs, id)
		}
	}
	if len(request.MemberIDs)+1 > MaxGroupMembers {
		return request, "", apperrors.ErrInvalidInput
	}
	raw, _ := json.Marshal(request)
	hash := sha256.Sum256(raw)
	return request, hex.EncodeToString(hash[:]), nil
}

func NormalizeUpdateGroup(request UpdateGroupRequest) (UpdateGroupRequest, error) {
	if request.Name == nil && request.Description == nil {
		return request, apperrors.ErrInvalidInput
	}
	if request.Name != nil {
		value, err := normalizeMetadata(*request.Name, 50, true)
		if err != nil {
			return request, err
		}
		request.Name = &value
	}
	if request.Description != nil {
		value, err := normalizeMetadata(*request.Description, 200, false)
		if err != nil {
			return request, err
		}
		request.Description = &value
	}
	return request, nil
}

func NormalizeListFilter(filter ListFilter) (ListFilter, error) {
	if filter.Type != "" && filter.Type != "direct" && filter.Type != "group" {
		return filter, apperrors.ErrInvalidInput
	}
	filter.Search = strings.TrimSpace(filter.Search)
	if !utf8.ValidString(filter.Search) || utf8.RuneCountInString(filter.Search) > 100 || strings.ContainsRune(filter.Search, 0) {
		return filter, apperrors.ErrInvalidInput
	}
	return filter, nil
}
