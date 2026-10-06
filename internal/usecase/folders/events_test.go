package folders

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

type eventBus struct {
	user  string
	event realtime.Envelope
	calls int
	err   error
}

func (b *eventBus) Publish(_ context.Context, user string, event realtime.Envelope) error {
	b.user, b.event = user, event
	b.calls++
	return b.err
}

func TestFolderEventsAreOwnerOnlyAndMinimal(t *testing.T) {
	owner, folder := uuid.NewString(), uuid.NewString()
	for _, kind := range []string{"folder.created", "folder.updated", "folder.deleted", "folder.items.updated", "folder.reordered"} {
		t.Run(kind, func(t *testing.T) {
			b := &eventBus{}
			id := folder
			if kind == "folder.reordered" {
				id = ""
			}
			if err := (&Events{Bus: b}).PublishFolder(context.Background(), owner, kind, id); err != nil {
				t.Fatal(err)
			}
			var data map[string]string
			if err := json.Unmarshal(b.event.Data, &data); err != nil {
				t.Fatal(err)
			}
			wantSize := 1
			if kind == "folder.reordered" {
				wantSize = 0
			}
			if b.calls != 1 || b.user != owner || b.event.Type != kind || len(data) != wantSize || (wantSize == 1 && data["folderId"] != folder) {
				t.Fatalf("unexpected event: %#v %#v", b, data)
			}
		})
	}
}

func TestFolderEventsRejectInvalidInputAndRespectCancellation(t *testing.T) {
	owner, folder := uuid.NewString(), uuid.NewString()
	b := &eventBus{}
	e := &Events{Bus: b}
	for _, args := range [][3]string{{"", "folder.created", folder}, {owner, "message.created", folder}, {owner, "folder.created", ""}, {owner, "folder.reordered", folder}, {owner, "folder.created", uuid.Nil.String()}} {
		if e.PublishFolder(context.Background(), args[0], args[1], args[2]) == nil {
			t.Fatal("accepted invalid input")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.PublishFolder(ctx, owner, "folder.created", folder); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if b.calls != 0 {
		t.Fatal("published invalid or cancelled event")
	}
	b.err = errors.New("bus unavailable")
	if err := e.PublishFolder(context.Background(), owner, "folder.created", folder); !errors.Is(err, b.err) {
		t.Fatal(err)
	}
}
