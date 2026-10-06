package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"gorm.io/gorm"
)

type groupProjectionPauseKey struct{}

func TestGroupWriteProjectionStaysInsideAuthorizationTransaction(t *testing.T) {
	for _, operation := range []string{"send", "edit", "delete", "mark-read"} {
		t.Run(operation, func(t *testing.T) {
			f := stageTwo(t)
			for _, hub := range f.hubs {
				hub.Shutdown()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			groups := pg.NewPersonalRepository(f.db)
			messages := pg.NewDirectChatRepository(f.db)
			group, _, err := groups.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: "Projection lock", MemberIDs: []string{f.member.ID}})
			if err != nil {
				t.Fatal(err)
			}
			quote, _, err := messages.Send(ctx, f.owner.ID, group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "quote before removal"}, strings.Repeat("a", 64))
			if err != nil {
				t.Fatal(err)
			}
			prior, _, err := messages.Send(ctx, f.member.ID, group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "prior reply", ReplyTo: quote.ID}, strings.Repeat("b", 64))
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once, paused sync.Once
			unlock := func() { once.Do(func() { close(release) }) }
			defer unlock()
			callback := "group_projection_pause_" + uuid.NewString()
			hook := func(query *gorm.DB) {
				if query.Statement.Context.Value(groupProjectionPauseKey{}) != true {
					return
				}
				sql := query.Statement.SQL.String()
				match := strings.Contains(sql, "conversation_messages.*") && strings.Contains(sql, "sender_name")
				if operation == "mark-read" {
					match = strings.Contains(sql, "last_read_message_id,last_read_sequence")
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
			if err = f.db.Callback().Query().After("gorm:query").Register(callback, hook); err != nil {
				t.Fatal(err)
			}
			if err = f.db.Callback().Row().After("gorm:row").Register(callback, hook); err != nil {
				t.Fatal(err)
			}
			mutationDone := make(chan error, 1)
			var item chat.Message
			var state chat.ReadState
			go func() {
				opCtx := context.WithValue(ctx, groupProjectionPauseKey{}, true)
				var err error
				switch operation {
				case "send":
					item, _, err = messages.Send(opCtx, f.member.ID, group.ID, chat.SendRequest{ClientRequestID: uuid.NewString(), Text: "new reply", ReplyTo: quote.ID}, strings.Repeat("c", 64))
				case "edit":
					item, err = messages.Edit(opCtx, f.member.ID, group.ID, prior.ID, "edited reply")
				case "delete":
					item, err = messages.Delete(opCtx, f.member.ID, group.ID, prior.ID)
				case "mark-read":
					state, err = messages.MarkRead(opCtx, f.member.ID, group.ID, quote.ID)
				}
				mutationDone <- err
			}()
			select {
			case <-entered:
			case err := <-mutationDone:
				t.Fatalf("projection not reached: %v", err)
			case <-ctx.Done():
				t.Fatal("projection pause timeout")
			}
			removal := make(chan error, 1)
			go func() { _, _, err := groups.RemoveGroupMember(ctx, f.owner.ID, group.ID, f.member.ID); removal <- err }()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			waiting := false
			for !waiting {
				select {
				case err := <-removal:
					t.Fatalf("removal committed before write DTO was decorated: %v", err)
				case <-ctx.Done():
					t.Fatal("write projection failed to hold lock")
				case <-ticker.C:
					var n int64
					if err = f.db.WithContext(ctx).Raw(`SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%conversations%'`).Scan(&n).Error; err != nil {
						t.Fatal(err)
					}
					waiting = n > 0
				}
			}
			unlock()
			if err = <-mutationDone; err != nil {
				t.Fatal(err)
			}
			if err = <-removal; err != nil {
				t.Fatal(err)
			}
			if _, err = messages.Edit(ctx, f.owner.ID, group.ID, quote.ID, "private quote after removal"); err != nil {
				t.Fatal(err)
			}
			if operation == "mark-read" {
				if state.LastReadMessageID == nil || *state.LastReadMessageID != quote.ID {
					t.Fatalf("incorrect committed read DTO: %+v", state)
				}
			} else {
				if item.ReplyPreview == nil || item.ReplyPreview.Text != "quote before removal" {
					t.Fatalf("write response read private post-removal reply: %+v", item.ReplyPreview)
				}
			}
		})
	}
}
