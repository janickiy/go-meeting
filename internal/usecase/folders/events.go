package folders

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

type Bus interface {
	Publish(context.Context, string, realtime.Envelope) error
}

// Events carries only invalidation identifiers to the folder owner's global
// channel. Membership of an item must never make its other users recipients.
type Events struct{ Bus Bus }

func (e *Events) PublishFolder(ctx context.Context, userID, kind, folderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	user, err := uuid.Parse(userID)
	if err != nil || user == uuid.Nil || e == nil || e.Bus == nil {
		return errors.New("invalid folder event recipient or bus")
	}
	data := map[string]string{}
	switch kind {
	case "folder.created", "folder.updated", "folder.deleted", "folder.items.updated":
		id, err := uuid.Parse(folderID)
		if err != nil || id == uuid.Nil {
			return errors.New("invalid folder event identifier")
		}
		data["folderId"] = id.String()
	case "folder.reordered":
		if folderID != "" {
			return errors.New("reorder event must not contain an item")
		}
	default:
		return errors.New("unsupported folder event")
	}
	return e.Bus.Publish(ctx, user.String(), realtime.Event(kind, "", data))
}
