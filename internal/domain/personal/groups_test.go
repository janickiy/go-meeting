package personal

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

func TestGroupMetadataBoundsAndNormalization(t *testing.T) {
	actor, peer := uuid.NewString(), uuid.NewString()
	base := CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "  Группа  ", Description: strings.Repeat("я", 200), MemberIDs: []string{peer, actor, peer}}
	normalized, fingerprint, err := NormalizeCreateGroup(actor, base)
	if err != nil || normalized.Name != "Группа" || len(normalized.MemberIDs) != 1 || len(fingerprint) != 64 {
		t.Fatalf("normalize: %+v %s %v", normalized, fingerprint, err)
	}
	base.MemberIDs = []string{peer}
	base.Name = "Группа"
	_, same, err := NormalizeCreateGroup(actor, base)
	if err != nil || same != fingerprint {
		t.Fatal("equivalent repeated create changed fingerprint")
	}
	for _, name := range []string{"", strings.Repeat("я", 51), "a\x00b", "x\u202ey", string([]byte{0xff})} {
		bad := base
		bad.Name = name
		if _, _, err = NormalizeCreateGroup(actor, bad); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("bad name accepted: %q %v", name, err)
		}
	}
	base.Name = strings.Repeat("я", 50)
	if _, _, err = NormalizeCreateGroup(actor, base); err != nil {
		t.Fatal("valid Unicode boundary denied", err)
	}
	base.Description = strings.Repeat("x", 201)
	if _, _, err = NormalizeCreateGroup(actor, base); err == nil {
		t.Fatal("long description accepted")
	}
	base.Name = "Команда 👨‍👩‍👧‍👦"
	base.Description = "Первая строка\nВторая строка\r\nПример\u200c"
	if _, _, err = NormalizeCreateGroup(actor, base); err != nil {
		t.Fatal("valid joiners or multiline description denied", err)
	}
	for _, field := range []string{"name", "description"} {
		for _, text := range []string{"text\x00bad", "text\tbad", "text\u202ebad", "text\u2066bad"} {
			bad := base
			if field == "name" {
				bad.Name = text
			} else {
				bad.Description = text
			}
			if _, _, err = NormalizeCreateGroup(actor, bad); err == nil {
				t.Fatalf("unsupported %s control accepted: %q", field, text)
			}
		}
	}
	base.Name = "Первая\nВторая"
	if _, _, err = NormalizeCreateGroup(actor, base); err == nil {
		t.Fatal("multiline name accepted")
	}
}

func TestGroupMemberBoundsAndFingerprint(t *testing.T) {
	actor := uuid.NewString()
	ids := []string{}
	for i := 0; i < 99; i++ {
		ids = append(ids, uuid.NewString())
	}
	req := CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Boundary", MemberIDs: ids}
	_, first, err := NormalizeCreateGroup(actor, req)
	if err != nil {
		t.Fatal(err)
	}
	req.MemberIDs = append(req.MemberIDs, actor)
	_, second, err := NormalizeCreateGroup(actor, req)
	if err != nil || first != second {
		t.Fatal("owner counted twice")
	}
	req.MemberIDs[99] = uuid.NewString()
	if _, _, err = NormalizeCreateGroup(actor, req); err == nil {
		t.Fatal("101 active members accepted")
	}
	for _, id := range []string{"invalid", "00000000-0000-0000-0000-000000000000"} {
		if _, err = NormalizeMemberIDs([]string{id}); err == nil {
			t.Fatal("invalid member UUID accepted")
		}
	}
}

func TestGroupListAndUpdateValidation(t *testing.T) {
	if _, err := NormalizeUpdateGroup(UpdateGroupRequest{}); err == nil {
		t.Fatal("empty patch accepted")
	}
	empty := ""
	if _, err := NormalizeUpdateGroup(UpdateGroupRequest{Name: &empty}); err == nil {
		t.Fatal("blank name accepted")
	}
	if _, err := NormalizeUpdateGroup(UpdateGroupRequest{Description: &empty}); err != nil {
		t.Fatal("description cannot be cleared")
	}
	if _, err := NormalizeListFilter(ListFilter{Type: "conference"}); err == nil {
		t.Fatal("unrelated conversation type accepted")
	}
	if _, err := NormalizeListFilter(ListFilter{Search: strings.Repeat("я", 101)}); err == nil {
		t.Fatal("unbounded search accepted")
	}
	filter, err := NormalizeListFilter(ListFilter{Type: "group", UnreadOnly: true, Search: "  Команда  "})
	if err != nil || filter.Search != "Команда" {
		t.Fatal("filter normalization", filter, err)
	}
}
