package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/folders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type folderRacePauseKey struct{}

func TestFolderMapAddSerializesWithRevocation(t *testing.T) {
	for _, kind := range []string{"conversation", "conference"} {
		for _, ordering := range []string{"add-first", "revoke-first"} {
			t.Run(kind+"/"+ordering, func(t *testing.T) {
				f := newFolderFixture(t)
				folder := f.create(t, f.memberToken, "Racing organization")
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				id, table := f.group.ID, "conversations"
				if kind == "conference" {
					id, table = f.meeting.ID, "conferences"
				}
				entered, release := make(chan struct{}), make(chan struct{})
				var once, paused sync.Once
				unlock := func() { once.Do(func() { close(release) }) }
				defer unlock()
				hook := func(query *gorm.DB) {
					if query.Statement.Context.Value(folderRacePauseKey{}) != true {
						return
					}
					sql := query.Statement.SQL.String()
					match := strings.HasPrefix(sql, "SELECT count(*) FROM (")
					if ordering == "revoke-first" {
						match = query.Statement.Table == "conversation_members"
						if kind == "conference" {
							match = strings.Contains(sql, "INSERT INTO conference_chat_preferences")
						}
					}
					if match {
						paused.Do(func() {
							close(entered)
							select {
							case <-release:
							case <-ctx.Done():
							}
						})
					}
				}
				callback := "folder_add_revoke_" + uuid.NewString()
				for _, register := range []func() error{
					func() error { return f.db.Callback().Row().After("gorm:row").Register(callback, hook) },
					func() error { return f.db.Callback().Update().After("gorm:update").Register(callback, hook) },
					func() error { return f.db.Callback().Raw().After("gorm:raw").Register(callback, hook) },
				} {
					if err := register(); err != nil {
						t.Fatal(err)
					}
				}
				addDone, revokeDone := make(chan error, 1), make(chan error, 1)
				add := func(mark bool) {
					opCtx := ctx
					if mark {
						opCtx = context.WithValue(ctx, folderRacePauseKey{}, true)
					}
					_, err := f.repo.SetItem(opCtx, f.member.ID, folder.ID, kind, id, true)
					addDone <- err
				}
				revoke := func(mark bool) {
					opCtx := ctx
					if mark {
						opCtx = context.WithValue(ctx, folderRacePauseKey{}, true)
					}
					var err error
					if kind == "conversation" {
						_, _, err = f.personal.RemoveGroupMember(opCtx, f.owner.ID, id, f.member.ID)
					} else {
						err = pg.NewChatRepository(f.db).LeaveChat(opCtx, f.member.ID, id)
					}
					revokeDone <- err
				}
				if ordering == "add-first" {
					go add(true)
				} else {
					go revoke(true)
				}
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("first transaction never reached barrier")
				}
				if ordering == "add-first" {
					go revoke(false)
				} else {
					go add(false)
				}
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				waiting := false
				for !waiting {
					select {
					case <-ctx.Done():
						t.Fatal("competing folder/revoke transaction did not wait on target lock")
					case <-ticker.C:
						var n int64
						if err := f.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE ?`, "%"+table+"%").Scan(&n).Error; err != nil {
							t.Fatal(err)
						}
						waiting = n > 0
					}
				}
				unlock()
				if err := <-revokeDone; err != nil {
					t.Fatal(err)
				}
				addErr := <-addDone
				if ordering == "add-first" && addErr != nil {
					t.Fatal("authorized add failed before removal", addErr)
				}
				if ordering == "revoke-first" && !errors.Is(addErr, apperrors.ErrForbidden) {
					t.Fatal("add authorized stale membership", addErr)
				}
				if f.get(t, f.memberToken, folder.ID).ItemCount != 0 {
					t.Fatal("mapping granted access after removal")
				}
				mapTable, column := "folder_conversations", "conversation_id"
				if kind == "conference" {
					mapTable, column = "folder_conferences", "conference_id"
				}
				var count int64
				if err := f.db.Table(mapTable).Where("folder_id=? AND "+column+"=?", folder.ID, id).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				want := int64(0)
				if ordering == "add-first" {
					want = 1
				}
				if count != want {
					t.Fatalf("wrong serialized mapping count=%d want%d", count, want)
				}
			})
		}
	}
}

func TestFolderReadProjectsOneAuthorizedSnapshot(t *testing.T) {
	for _, boundary := range []string{"before-snapshot", "after-snapshot"} {
		t.Run(boundary, func(t *testing.T) {
			f := newFolderFixture(t)
			folder := f.create(t, f.memberToken, "Read organization")
			f.add(t, f.memberToken, folder.ID, "conversation", f.group.ID)
			messages := pg.NewDirectChatRepository(f.db)
			groupSend(t, messages, f.owner.ID, f.group.ID, "Preview before revocation")
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			var once, paused sync.Once
			unlock := func() { once.Do(func() { close(release) }) }
			defer unlock()
			hook := func(query *gorm.DB) {
				if query.Statement.Context.Value(folderRacePauseKey{}) != true || !strings.Contains(query.Statement.SQL.String(), "FROM selected s LEFT JOIN LATERAL") {
					return
				}
				paused.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-ctx.Done():
					}
				})
			}
			callback := "folder_read_snapshot_" + uuid.NewString()
			var err error
			if boundary == "before-snapshot" {
				err = f.db.Callback().Row().Before("gorm:row").Register(callback, hook)
			} else {
				err = f.db.Callback().Row().After("gorm:row").Register(callback, hook)
			}
			if err != nil {
				t.Fatal(err)
			}
			readDone := make(chan error, 1)
			var page folders.Page
			go func() {
				var err error
				page, err = f.repo.Items(context.WithValue(ctx, folderRacePauseKey{}, true), f.member.ID, folder.ID, "", 50, folders.Filter{Type: "all"})
				readDone <- err
			}()
			select {
			case <-entered:
			case err := <-readDone:
				t.Fatalf("read projection never reached barrier: %v", err)
			case <-ctx.Done():
				t.Fatal("read projection barrier timeout")
			}
			if _, _, err = f.personal.RemoveGroupMember(ctx, f.owner.ID, f.group.ID, f.member.ID); err != nil {
				t.Fatal(err)
			}
			groupSend(t, messages, f.owner.ID, f.group.ID, "PRIVATE AFTER REVOCATION")
			unlock()
			if err = <-readDone; err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "PRIVATE AFTER REVOCATION") {
				t.Fatal("folder read leaked metadata beyond authorization snapshot")
			}
			want := 0
			if boundary == "after-snapshot" {
				want = 1
			}
			if len(page.Items) != want {
				t.Fatalf("snapshot boundary %s produced %d items want%d", boundary, len(page.Items), want)
			}
			if want == 1 && !strings.Contains(string(encoded), "Preview before revocation") {
				t.Fatal("pre-revocation snapshot did not preserve original preview")
			}
			if f.get(t, f.memberToken, folder.ID).ItemCount != 0 || len(f.items(t, f.memberToken, "/folders/"+folder.ID+"/items").Items) != 0 {
				t.Fatal("subsequent folder read retained revoked metadata")
			}
		})
	}
}
