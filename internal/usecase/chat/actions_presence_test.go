package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	chatdomain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type memberRepository struct {
	ActionRepository
	items []chatdomain.MemberView
	err   error
	calls int
}

func (r *memberRepository) ChatMembers(context.Context, string, string, string, int) ([]chatdomain.MemberView, string, error) {
	r.calls++
	return append([]chatdomain.MemberView{}, r.items...), "next-page", r.err
}

type memberPresence struct {
	ids      []string
	statuses map[string]bool
	sessions []realtime.Session
	err      error
	accounts int
	guests   int
}

func (p *memberPresence) Online(_ context.Context, ids []string) (map[string]bool, error) {
	p.accounts++
	p.ids = append([]string{}, ids...)
	return p.statuses, p.err
}
func (p *memberPresence) GetActiveSessions(context.Context, string) ([]realtime.Session, error) {
	p.guests++
	return p.sessions, p.err
}

func TestMembersPresenceRequiresAuthorizedPage(t *testing.T) {
	repo := &memberRepository{err: apperrors.ErrForbidden}
	presence := &memberPresence{}
	service := NewActionService(repo, nil).WithPresence(presence, presence)
	items, cursor, err := service.Members(context.Background(), "outsider", "meeting", "", 50)
	if !errors.Is(err, apperrors.ErrForbidden) || items != nil || cursor != "" || presence.accounts != 0 || presence.guests != 0 {
		t.Fatalf("unauthorized page reached Redis: items=%v cursor=%q err=%v presence=%+v", items, cursor, err, presence)
	}
	for _, limit := range []int{0, 101} {
		if _, _, err = service.Members(context.Background(), "user", "meeting", "", limit); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Fatalf("invalid page bound %d: %v", limit, err)
		}
	}
	if repo.calls != 1 {
		t.Fatal("invalid pagination reached repository")
	}
}

func TestMembersSeparateMembershipAndLivePresence(t *testing.T) {
	account, offline, guest := "account", "offline", "guest"
	repo := &memberRepository{items: []chatdomain.MemberView{
		{ParticipantView: conferences.ParticipantView{UserID: &account, Status: conferences.Left}},
		{ParticipantView: conferences.ParticipantView{UserID: &offline, Status: conferences.Joined}},
		{ParticipantView: conferences.ParticipantView{UserID: &guest, Status: conferences.Joined}, IsGuest: true},
	}}
	presence := &memberPresence{statuses: map[string]bool{account: true}, sessions: []realtime.Session{
		{ConferenceID: "meeting", UserID: guest}, {ConferenceID: "meeting", UserID: guest},
	}}
	items, cursor, err := NewActionService(repo, nil).WithPresence(presence, presence).Members(context.Background(), account, "meeting", "", 100)
	if err != nil || cursor != "next-page" || len(items) != 3 || items[0].Online == nil || !*items[0].Online || items[1].Online == nil || *items[1].Online || items[2].Online == nil || !*items[2].Online || !items[2].IsGuest {
		t.Fatalf("membership/presence projection: %v %q %v", items, cursor, err)
	}
	if presence.accounts != 1 || presence.guests != 1 || len(presence.ids) != 2 || presence.ids[0] != account || presence.ids[1] != offline {
		t.Fatalf("unbounded or guest account lookup: %+v", presence)
	}
	presence.sessions = []realtime.Session{{ConferenceID: "other-meeting", UserID: guest}}
	items, _, err = NewActionService(repo, nil).WithPresence(presence, presence).Members(context.Background(), account, "meeting", "", 100)
	if err != nil || items[2].Online == nil || *items[2].Online {
		t.Fatalf("unrelated conference disclosed guest presence: %v %v", items, err)
	}
}

func TestMembersPresenceFailurePreservesAuthorizedPageWithUnknownStatus(t *testing.T) {
	account, guest := "account", "guest"
	for _, test := range []struct {
		name string
		item chatdomain.MemberView
	}{
		{"account", chatdomain.MemberView{ParticipantView: conferences.ParticipantView{UserID: &account}}},
		{"guest", chatdomain.MemberView{ParticipantView: conferences.ParticipantView{UserID: &guest}, IsGuest: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &memberRepository{items: []chatdomain.MemberView{test.item}}
			failed := &memberPresence{err: errors.New("Redis unavailable")}
			items, cursor, err := NewActionService(repo, nil).WithPresence(failed, failed).Members(context.Background(), "actor", "meeting", "", 50)
			if err != nil || len(items) != 1 || items[0].Online != nil || cursor != "next-page" || items[0].IsGuest != test.item.IsGuest {
				t.Fatalf("presence failure hid members or returned false offline: %v %q %v", items, cursor, err)
			}
		})
	}
}

func TestMembersCancelledRequestDoesNotReturnSoftSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	account := "account"
	repo := &memberRepository{items: []chatdomain.MemberView{{ParticipantView: conferences.ParticipantView{UserID: &account}}}}
	items, cursor, err := NewActionService(repo, nil).WithPresence(&memberPresence{}, nil).Members(ctx, account, "meeting", "", 50)
	if !errors.Is(err, context.Canceled) || items != nil || cursor != "" {
		t.Fatalf("cancelled request returned soft success: %v %q %v", items, cursor, err)
	}
}

func TestMembersPresenceSourcesDegradeIndependently(t *testing.T) {
	account, guest := "account", "guest"
	repo := &memberRepository{items: []chatdomain.MemberView{
		{ParticipantView: conferences.ParticipantView{UserID: &account}},
		{ParticipantView: conferences.ParticipantView{UserID: &guest}, IsGuest: true},
	}}
	accounts := &memberPresence{err: errors.New("global presence unavailable")}
	guests := &memberPresence{sessions: []realtime.Session{{ConferenceID: "meeting", UserID: guest}}}
	items, _, err := NewActionService(repo, nil).WithPresence(accounts, guests).Members(context.Background(), account, "meeting", "", 50)
	if err != nil || items[0].Online != nil || items[1].Online == nil || !*items[1].Online {
		t.Fatalf("account presence failure hid healthy guest status: %v %v", items, err)
	}
	accounts.err, accounts.statuses = nil, map[string]bool{account: true}
	guests.err = errors.New("conference presence unavailable")
	items, _, err = NewActionService(repo, nil).WithPresence(accounts, guests).Members(context.Background(), account, "meeting", "", 50)
	if err != nil || items[0].Online == nil || !*items[0].Online || items[1].Online != nil {
		t.Fatalf("guest presence failure hid healthy account status: %v %v", items, err)
	}
}
