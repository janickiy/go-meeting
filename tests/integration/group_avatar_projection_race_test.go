package integration_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	personalusecase "github.com/janickiy/go-recorder/internal/usecase/personal"
	"gorm.io/gorm"
)

type avatarProjectionPauseKey struct{}

// Delay returning the committed snapshot until removal has committed. Any
// service-level post-commit Get would now return Forbidden for this caller.
type avatarSnapshotAfterCommitRepo struct {
	personalusecase.AssetRepository
	afterCommit func(context.Context) error
}

func (r avatarSnapshotAfterCommitRepo) ActivateAvatar(ctx context.Context, user, conversation, version string) (personal.Conversation, error) {
	item, err := r.AssetRepository.ActivateAvatar(ctx, user, conversation, version)
	if err == nil {
		err = r.afterCommit(ctx)
	}
	return item, err
}

func (r avatarSnapshotAfterCommitRepo) ClearAvatar(ctx context.Context, user, conversation string) (personal.Conversation, bool, error) {
	item, changed, err := r.AssetRepository.ClearAvatar(ctx, user, conversation)
	if err == nil {
		err = r.afterCommit(ctx)
	}
	return item, changed, err
}

func TestGroupAvatarProjectionStaysInsideAuthorizationTransaction(t *testing.T) {
	for _, operation := range []string{"activate", "clear"} {
		t.Run(operation, func(t *testing.T) {
			f := groupAssets(t)
			// Quiesce fixture janitors before registering GORM callbacks.
			for _, hub := range f.base.hubs {
				hub.Shutdown()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, err := f.repo.ChangeGroupRole(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID, personal.Admin); err != nil {
				t.Fatal(err)
			}
			data := groupPNG(t)
			if operation == "clear" {
				if _, err := f.assets.PutAvatar(ctx, f.base.owner.ID, f.group.ID, "image/png", bytes.NewReader(data)); err != nil {
					t.Fatal(err)
				}
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once, paused sync.Once
			unlock := func() { once.Do(func() { close(release) }) }
			defer unlock()
			callback := "avatar_projection_pause_" + uuid.NewString()
			if err := f.base.db.Callback().Row().After("gorm:row").Register(callback, func(query *gorm.DB) {
				if query.Statement.Context.Value(avatarProjectionPauseKey{}) != true || !strings.Contains(query.Statement.SQL.String(), "SELECT c.id,c.type,c.created_at,c.updated_at,c.last_message_at,c.last_message_id") {
					return
				}
				paused.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}); err != nil {
				t.Fatal(err)
			}
			removal := make(chan error, 1)
			repo := avatarSnapshotAfterCommitRepo{AssetRepository: f.assetsRepo, afterCommit: func(ctx context.Context) error {
				select {
				case err := <-removal:
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			}}
			service, err := personalusecase.NewAssetService(ctx, repo, f.storage, f.repo, nil)
			if err != nil {
				t.Fatal(err)
			}
			mutationDone := make(chan error, 1)
			var item personal.Conversation
			go func() {
				opCtx := context.WithValue(ctx, avatarProjectionPauseKey{}, true)
				var err error
				if operation == "activate" {
					item, err = service.PutAvatar(opCtx, f.base.member.ID, f.group.ID, "image/png", bytes.NewReader(data))
				} else {
					item, err = service.DeleteAvatar(opCtx, f.base.member.ID, f.group.ID)
				}
				mutationDone <- err
			}()
			select {
			case <-entered:
			case err := <-mutationDone:
				t.Fatalf("avatar projection not reached: %v", err)
			case <-ctx.Done():
				t.Fatal("avatar projection pause timeout")
			}
			go func() {
				_, _, err := f.repo.RemoveGroupMember(ctx, f.base.owner.ID, f.group.ID, f.base.member.ID)
				removal <- err
			}()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			waiting := false
			for !waiting {
				select {
				case err := <-removal:
					t.Fatalf("removal committed before avatar DTO projection: %v", err)
				case <-ctx.Done():
					t.Fatal("avatar projection failed to hold lock")
				case <-ticker.C:
					var n int64
					if err = f.base.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&n).Error; err != nil {
						t.Fatal(err)
					}
					waiting = n > 0
				}
			}
			unlock()
			if err = <-mutationDone; err != nil {
				t.Fatalf("committed avatar returned post-removal failure: %v", err)
			}
			if item.ID != f.group.ID || item.MyRole != personal.Admin || (operation == "activate") != (item.AvatarVersion != nil) {
				t.Fatalf("wrong committed avatar snapshot: %+v", item)
			}
			if _, err = f.repo.Get(ctx, f.base.member.ID, f.group.ID); !errors.Is(err, apperrors.ErrForbidden) {
				t.Fatalf("removal did not commit before response: %v", err)
			}
			current, err := f.repo.Get(ctx, f.base.owner.ID, f.group.ID)
			if err != nil || (current.AvatarVersion == nil) != (item.AvatarVersion == nil) || (item.AvatarVersion != nil && *current.AvatarVersion != *item.AvatarVersion) {
				t.Fatalf("snapshot differs from committed avatar: %+v %v", current, err)
			}
		})
	}
}
