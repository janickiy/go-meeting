package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type groupReadPauseKey struct{}

// Pause immediately after the fresh membership query, not before the HTTP
// request starts. This reproduces the former READ COMMITTED authorization gap.
func TestGroupHistoryAndReadStateHoldAuthorizationUntilProjection(t *testing.T) {
	for _, operation := range []string{"history", "read-state"} {
		t.Run(operation, func(t *testing.T) {
			f := stageTwo(t)
			// This test instruments GORM, not realtime. Stop the fixture janitors
			// before changing its callback registry.
			for _, hub := range f.hubs {
				hub.Shutdown()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			groups := pg.NewPersonalRepository(f.db)
			messages := pg.NewDirectChatRepository(f.db)
			group, _, err := groups.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Read lock", MemberIDs: []string{f.member.ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err = messages.Send(ctx, f.owner.ID, group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "before removal"}, strings.Repeat("a", 64)); err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once, paused sync.Once
			unlock := func() { once.Do(func() { close(release) }) }
			defer unlock()
			callback := "group_read_pause_" + uuid.NewString()
			if err = f.db.Callback().Query().After("gorm:query").Register(callback, func(query *gorm.DB) {
				if query.Statement.Context.Value(groupReadPauseKey{}) != true || !strings.Contains(query.Statement.SQL.String(), "conversation_members m") || !strings.Contains(query.Statement.SQL.String(), "count(*)") {
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
			// The isolated DB is closed by this fixture; retain its callback until
			// then rather than mutate the registry during a failed concurrent read.
			readDone := make(chan error, 1)
			var page chat.Page
			var state chat.ReadState
			go func() {
				readCtx := context.WithValue(ctx, groupReadPauseKey{}, true)
				var err error
				if operation == "history" {
					page, err = messages.List(readCtx, f.member.ID, group.ID, "", 50)
				} else {
					state, err = messages.ReadState(readCtx, f.member.ID, group.ID)
				}
				readDone <- err
			}()
			select {
			case <-entered:
			case err := <-readDone:
				t.Fatalf("read failed before authorization pause: %v", err)
			case <-ctx.Done():
				t.Fatal("read did not pause")
			}
			removal := make(chan error, 1)
			go func() { _, _, err := groups.RemoveGroupMember(ctx, f.owner.ID, group.ID, f.member.ID); removal <- err }()
			// Require an observed real lock wait, not an assumed scheduling delay.
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			waiting := false
			for !waiting {
				select {
				case err := <-removal:
					t.Fatalf("removal bypassed authorized projection: %v", err)
				case <-ctx.Done():
					t.Fatal("no removal lock wait")
				case <-ticker.C:
					var n int64
					if err = f.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&n).Error; err != nil {
						t.Fatal(err)
					}
					waiting = n > 0
				}
			}
			unlock()
			if err = <-readDone; err != nil {
				t.Fatal(err)
			}
			if err = <-removal; err != nil {
				t.Fatal(err)
			}
			if _, _, err = messages.Send(ctx, f.owner.ID, group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "private after removal"}, strings.Repeat("b", 64)); err != nil {
				t.Fatal(err)
			}
			if operation == "history" {
				if len(page.Items) != 1 || page.Items[0].Text != "before removal" {
					t.Fatalf("read crossed removal frontier: %+v", page.Items)
				}
				if _, err = messages.List(ctx, f.member.ID, group.ID, "", 50); !errors.Is(err, apperrors.ErrForbidden) {
					t.Fatalf("removed history reader: %v", err)
				}
			} else {
				if state.UnreadCount != 1 {
					t.Fatalf("read-state included post-removal data: %+v", state)
				}
				if _, err = messages.ReadState(ctx, f.member.ID, group.ID); !errors.Is(err, apperrors.ErrForbidden) {
					t.Fatalf("removed read-state reader: %v", err)
				}
			}
		})
	}
}
