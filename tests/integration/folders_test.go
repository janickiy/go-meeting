package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/folders"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
)

func TestFolderRepositoryUsesCurrentBoundedConversationProjection(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	folder, err := f.repo.Create(ctx, f.member.ID, "Current metadata")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{f.direct.ID, f.group.ID} {
		if _, err = f.repo.SetItem(ctx, f.member.ID, folder.ID, "conversation", id, true); err != nil {
			t.Fatal(err)
		}
	}
	message := groupSend(t, pg.NewDirectChatRepository(f.db), f.owner.ID, f.group.ID, "Новое сообщение после добавления")
	name := "Renamed after mapping"
	if _, err = f.personal.UpdateGroup(ctx, f.owner.ID, f.group.ID, personal.UpdateGroupRequest{Name: &name}); err != nil {
		t.Fatal(err)
	}
	page, err := f.repo.Items(ctx, f.member.ID, folder.ID, "", 50, folders.Filter{Type: "conversation"})
	if err != nil || len(page.Items) != 2 {
		t.Fatal("projection page", page, err)
	}
	for _, item := range page.Items {
		actual, ok := item.Item.(personal.Conversation)
		if !ok {
			t.Fatalf("unexpected projection type: %T", item.Item)
		}
		expected, err := f.personal.Get(ctx, f.member.ID, actual.ID)
		if err != nil {
			t.Fatal(err)
		}
		// ActivityAt is an internal ordering key, not an API field.
		expected.ActivityAt = actual.ActivityAt
		// PostgreSQL JSON timestamps use UTC; the normal GORM projection uses
		// the session's location. The instant and public metadata must agree.
		expected.CreatedAt, actual.CreatedAt = expected.CreatedAt.UTC(), actual.CreatedAt.UTC()
		expected.UpdatedAt, actual.UpdatedAt = expected.UpdatedAt.UTC(), actual.UpdatedAt.UTC()
		if expected.LastMessageAt != nil && actual.LastMessageAt != nil {
			expectedAt, actualAt := expected.LastMessageAt.UTC(), actual.LastMessageAt.UTC()
			expected.LastMessageAt, actual.LastMessageAt = &expectedAt, &actualAt
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("folder projection drift: actual=%+v expected=%+v", actual, expected)
		}
		if actual.ID == f.group.ID && (actual.Name != name || actual.Preview != message.Text || actual.UnreadCount != 1 || actual.LastSender == nil || actual.LastSender.ID != f.owner.ID) {
			t.Fatal("stale group metadata/message/read projection", actual)
		}
	}
	if _, _, err = f.personal.RemoveGroupMember(ctx, f.owner.ID, f.group.ID, f.member.ID); err != nil {
		t.Fatal(err)
	}
	view, err := f.repo.Get(ctx, f.member.ID, folder.ID)
	if err != nil || view.ItemCount != 1 {
		t.Fatal("removed membership still counted", view, err)
	}
	if _, _, err = f.personal.AddGroupMembers(ctx, f.owner.ID, f.group.ID, []string{f.member.ID}); err != nil {
		t.Fatal(err)
	}
	view, err = f.repo.Get(ctx, f.member.ID, folder.ID)
	if err != nil || view.ItemCount != 2 {
		t.Fatal("retained mapping failed after authorized rejoin", view, err)
	}
}

func TestFolderMappingsConcurrentIdempotencyAndIndependentOwnership(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	first, err := f.repo.Create(ctx, f.owner.ID, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.repo.Create(ctx, f.owner.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	var failures [8]error
	runConcurrent(len(failures), func(i int) {
		_, failures[i] = f.repo.SetItem(ctx, f.owner.ID, first.ID, "conversation", f.direct.ID, true)
	})
	for _, err := range failures {
		if err != nil {
			t.Fatal("idempotent concurrent map", err)
		}
	}
	if _, err = f.repo.SetItem(ctx, f.owner.ID, second.ID, "conversation", f.direct.ID, true); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = f.db.Table("folder_conversations").Where("folder_id=?", first.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate map", count, err)
	}
	if _, err = f.repo.SetItem(ctx, f.member.ID, first.ID, "conversation", f.direct.ID, true); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("cross-owner map did not return scoped denial", err)
	}
	runConcurrent(len(failures), func(i int) {
		_, failures[i] = f.repo.SetItem(ctx, f.owner.ID, first.ID, "conversation", f.direct.ID, false)
	})
	for _, err := range failures {
		if err != nil {
			t.Fatal("idempotent concurrent unmap", err)
		}
	}
	view, err := f.repo.Get(ctx, f.owner.ID, second.ID)
	if err != nil || view.ItemCount != 1 {
		t.Fatal("sibling mapping changed", view, err)
	}
	if err = f.repo.Delete(ctx, f.owner.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	view, err = f.repo.Get(ctx, f.owner.ID, second.ID)
	if err != nil || view.Position != 0 || view.ItemCount != 1 {
		t.Fatal("position compaction lost mapping", view, err)
	}
	if _, err = f.personal.Get(ctx, f.owner.ID, f.direct.ID); err != nil {
		t.Fatal("folder deletion changed underlying chat", err)
	}
}

func TestFoldersMigrationRollbackPreservesOrganizationData(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	folder, err := f.repo.Create(ctx, f.owner.ID, "Must survive rollback")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.repo.SetItem(ctx, f.owner.ID, folder.ID, "conversation", f.direct.ID, true); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("..", "..", "database", "migrations")
	down, err := os.ReadFile(filepath.Join(dir, "000032_personal_folders.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(down)).Error; err == nil {
		t.Fatal("populated rollback discarded folders")
	}
	view, err := f.repo.Get(ctx, f.owner.ID, folder.ID)
	if err != nil || view.ItemCount != 1 {
		t.Fatal("rollback guard changed folder data", view, err)
	}
	if err = f.repo.Delete(ctx, f.owner.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(down)).Error; err != nil {
		t.Fatal("empty rollback failed", err)
	}
	if err = f.db.Exec(`DELETE FROM release_schema_migrations WHERE name='000032_personal_folders.up.sql'`).Error; err != nil {
		t.Fatal(err)
	}
	if err = pg.RunMigrations(f.db, dir); err != nil {
		t.Fatal("migration replay failed", err)
	}
	view, err = f.repo.Create(ctx, f.owner.ID, "Replayed")
	if err != nil || view.ID == "" {
		t.Fatal("replayed schema failed", view, err)
	}
	if _, err = f.personal.Get(ctx, f.owner.ID, f.direct.ID); err != nil {
		t.Fatal("rollback/replay changed underlying conversation", err)
	}
}
