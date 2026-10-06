package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"github.com/janickiy/go-recorder/internal/domain/folders"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"gorm.io/gorm"
)

func TestFoldersQueriesStayBoundedAndScoped(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	var groupIDs, meetingIDs []string
	for i := 0; i < 30; i++ {
		group, _, err := f.personal.CreateGroup(ctx, f.owner.ID, personal.CreateGroupRequest{ClientRequestID: uuid.NewString(), Name: fmt.Sprintf("Scoped group %02d", i), MemberIDs: []string{f.member.ID}})
		if err != nil {
			t.Fatal(err)
		}
		groupIDs = append(groupIDs, group.ID)
		meeting, err := f.conferenceService.Create(ctx, f.owner.ID, conferences.CreateRequest{Title: fmt.Sprintf("Scoped meeting %02d", i)})
		if err != nil {
			t.Fatal(err)
		}
		meetingIDs = append(meetingIDs, meeting.ID)
	}
	if err := f.db.Exec(`INSERT INTO users(id,email,password_hash)
	 SELECT gen_random_uuid(),'folder-noise-'||i||'@query.test','unused' FROM generate_series(1,30) i`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Exec(`INSERT INTO folders(id,user_id,name,name_key,position)
	 SELECT gen_random_uuid(),u.id,'Noise '||i,'noise '||i,i-1 FROM users u CROSS JOIN generate_series(1,20) i WHERE u.email LIKE 'folder-noise-%@query.test'`).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO folder_conversations(folder_id,conversation_id) SELECT f.id,c.id FROM folders f CROSS JOIN conversations c WHERE f.name LIKE 'Noise %' AND c.id IN ?`,
		`INSERT INTO folder_conferences(folder_id,conference_id) SELECT f.id,c.id FROM folders f CROSS JOIN conferences c WHERE f.name LIKE 'Noise %' AND c.id IN ?`,
	} {
		ids := groupIDs
		if strings.Contains(statement, "folder_conferences") {
			ids = meetingIDs
		}
		if err := f.db.Exec(statement, ids).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Every map is valid historical organization data. Noise users previously
	// belonged to these groups but no longer have active membership.
	if err := f.db.Exec(`INSERT INTO conversation_members(conversation_id,user_id,left_at)
	 SELECT c.id,u.id,clock_timestamp() FROM conversations c CROSS JOIN users u WHERE c.id IN ? AND u.email LIKE 'folder-noise-%@query.test'`, groupIDs).Error; err != nil {
		t.Fatal(err)
	}
	main := f.create(t, f.ownerToken, "Scoped folder 00")
	seedMaps := func(folderID string) {
		if err := f.db.Exec(`INSERT INTO folder_conversations(folder_id,conversation_id) SELECT ?::uuid,id FROM conversations WHERE id IN ?`, folderID, groupIDs).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.db.Exec(`INSERT INTO folder_conferences(folder_id,conference_id) SELECT ?::uuid,id FROM conferences WHERE id IN ?`, folderID, meetingIDs).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedMaps(main.ID)
	for _, statement := range []string{"ANALYZE folders", "ANALYZE folder_conversations", "ANALYZE folder_conferences", "ANALYZE conversation_members", "ANALYZE conversations", "ANALYZE conference_participants", "ANALYZE conference_chat_preferences", "ANALYZE conferences"} {
		if err := f.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	var capture atomic.Bool
	var mu sync.Mutex
	var captured []string
	hook := func(query *gorm.DB) {
		if !capture.Load() {
			return
		}
		mu.Lock()
		captured = append(captured, query.Dialector.Explain(query.Statement.SQL.String(), query.Statement.Vars...))
		mu.Unlock()
	}
	callback := "folder_query_capture_" + uuid.NewString()
	for _, register := range []func() error{
		func() error { return f.db.Callback().Query().After("gorm:query").Register(callback, hook) },
		func() error { return f.db.Callback().Row().After("gorm:row").Register(callback, hook) },
	} {
		if err := register(); err != nil {
			t.Fatal(err)
		}
	}
	measure := func(run func() error) []string {
		mu.Lock()
		captured = nil
		mu.Unlock()
		capture.Store(true)
		err := run()
		capture.Store(false)
		if err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, captured...)
	}
	smallList := measure(func() error {
		rows, err := f.repo.List(ctx, f.owner.ID, "", "")
		if err == nil && (len(rows) != 1 || rows[0].ItemCount != 60) {
			t.Fatalf("scoped count incorrect: %+v", rows)
		}
		return err
	})
	for i := 1; i < 50; i++ {
		row := f.create(t, f.ownerToken, fmt.Sprintf("Scoped folder %02d", i))
		seedMaps(row.ID)
	}
	for _, table := range []string{"folders", "folder_conversations", "folder_conferences"} {
		if err := f.db.Exec("ANALYZE " + table).Error; err != nil {
			t.Fatal(err)
		}
	}
	largeList := measure(func() error {
		rows, err := f.repo.List(ctx, f.owner.ID, "", "")
		if err == nil && len(rows) != 50 {
			t.Fatalf("folder list leaked other owners: %d", len(rows))
		}
		return err
	})
	if len(smallList) != len(largeList) || len(largeList) > 4 {
		t.Fatalf("folder list N+1: one=%d fifty=%d", len(smallList), len(largeList))
	}
	plans := map[string]any{}
	statements := map[string][]string{"list": largeList}
	for _, workload := range []struct {
		name string
		run  func(int) error
	}{
		{"items", func(limit int) error {
			page, err := f.repo.Items(ctx, f.owner.ID, main.ID, "", limit, folders.Filter{Type: "all"})
			if err == nil && len(page.Items) != limit {
				t.Fatalf("wrong bounded items: %d", len(page.Items))
			}
			return err
		}},
		{"candidates", func(limit int) error {
			page, err := f.repo.Candidates(ctx, f.owner.ID, main.ID, "", limit, folders.Filter{Type: "all"})
			if err == nil && len(page.Items) != limit {
				t.Fatalf("wrong bounded candidates: %d", len(page.Items))
			}
			return err
		}},
	} {
		one := measure(func() error { return workload.run(1) })
		many := measure(func() error { return workload.run(50) })
		if len(one) != len(many) || len(many) > 5 {
			t.Fatalf("%s metadata N+1: one=%d fifty=%d", workload.name, len(one), len(many))
		}
		statements[workload.name] = many
		t.Logf("%s SQL count=%d for1 and50 items", workload.name, len(many))
	}
	t.Logf("list SQL count=%d for1 and50 folders", len(largeList))
	for workload, queries := range statements {
		for i, statement := range queries {
			if !strings.Contains(statement, "folder_conversations") && !strings.Contains(statement, "folder_conferences") {
				continue
			}
			var rows []struct {
				Plan string `gorm:"column:QUERY PLAN"`
			}
			if err := f.db.Raw("EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) " + statement).Scan(&rows).Error; err != nil {
				t.Fatalf("%s explain: %v", workload, err)
			}
			if len(rows) != 1 {
				t.Fatal("EXPLAIN did not return JSON plan")
			}
			var plan any
			if err := json.Unmarshal([]byte(rows[0].Plan), &plan); err != nil {
				t.Fatal(err)
			}
			plans[fmt.Sprintf("%s-%d", workload, i)] = map[string]any{"sql": statement, "plan": plan}
			t.Logf("%s-%d EXPLAIN: %s", workload, i, rows[0].Plan)
		}
	}
	if len(plans) < 3 {
		t.Fatal("missing actual production EXPLAIN workloads", len(plans))
	}
	if dir := os.Getenv("RECORDER_FOLDERS_EVIDENCE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		output, err := json.MarshalIndent(map[string]any{"ownerFolders": 50, "otherFolders": 600, "ownerMappings": 3000, "otherMappings": 36000, "sqlCounts": map[string]int{"list": len(largeList), "items": len(statements["items"]), "candidates": len(statements["candidates"])}, "plans": plans}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "folder-query-plans.json"), output, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
