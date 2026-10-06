package integration_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	"github.com/janickiy/go-recorder/internal/usecase/recordings"
	"gorm.io/gorm"
)

// Exercise the production database adapter: page order, nullable fields, nested
// DTO equivalence, membership and availability checks must survive batching.
func TestP1RecordingBatchPreservesPage(t *testing.T) {
	db := stageOneDatabase(t)
	seedP1DB(t, db, 20)
	ctx := context.Background()
	uid, cid := p1ID("user", 1), p1ID("conference", 1)
	// Include records with absent artifacts and varied nullable metadata.
	for _, q := range []string{
		`DELETE FROM record_file WHERE record_id=(SELECT id FROM record WHERE uuid=?)`,
		`DELETE FROM record_segment WHERE record_id=(SELECT id FROM record WHERE uuid=?)`,
		`DELETE FROM record_event WHERE record_id=(SELECT id FROM record WHERE uuid=?)`,
		`UPDATE record SET duration_sec=NULL,size_bytes=NULL,storage_object_key=NULL,metadata_json=NULL WHERE uuid=?`,
	} {
		if err := db.Exec(q, p1ID("record", 1)).Error; err != nil {
			t.Fatal(err)
		}
	}
	rr := pg.NewRecordRepository(db)
	for i, mode := range []string{records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus} {
		if err := db.Exec("UPDATE record SET mode=? WHERE uuid=?", mode, p1ID("record", i+2)).Error; err != nil {
			t.Fatal(err)
		}
	}
	reader := recorder.NewService(rr, nil, nil, nil)
	repo := pg.NewConferenceRecordingRepository(db)
	svc := recordings.NewConferenceService(repo, reader, nil)
	rows, err := repo.List(ctx, uid, cid, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	expected := make([]records.RecordCard, 0, len(rows))
	for _, row := range rows {
		card, e := reader.ReadComposite(ctx, row.UUID)
		if e != nil {
			t.Fatal(e)
		}
		card.Status = records.PublicStatus(card.Status)
		expected = append(expected, card)
	}
	var count atomic.Int64
	if err = db.Callback().Query().After("gorm:query").Register("p1-regression-count", func(*gorm.DB) { count.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err = db.Callback().Row().After("gorm:row").Register("p1-regression-row-count", func(*gorm.DB) { count.Add(1) }); err != nil {
		t.Fatal(err)
	}
	actual, err := svc.List(ctx, uid, cid, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if count.Load() != 6 {
		t.Fatalf("page 20 SQL count=%d, want 6", count.Load())
	}
	if !reflect.DeepEqual(expected, actual) {
		for i := range expected {
			if !reflect.DeepEqual(expected[i], actual[i]) {
				t.Fatalf("card %d differs from single read: record=%t files=%t segments=%t events=%t", i, reflect.DeepEqual(expected[i].Record, actual[i].Record), reflect.DeepEqual(expected[i].Files, actual[i].Files), reflect.DeepEqual(expected[i].Segments, actual[i].Segments), reflect.DeepEqual(expected[i].Events, actual[i].Events))
			}
		}
	}
	count.Store(0)
	if _, err = svc.List(ctx, uid, cid, 1, 0); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 6 {
		t.Fatalf("page 1 SQL count=%d", count.Load())
	}
	if _, err = svc.List(ctx, uuid.NewString(), cid, 20, 0); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("outsider read: %v", err)
	}
	if err = db.Exec("UPDATE conference_participants SET admission_state='waiting',status='waiting' WHERE conference_id=? AND user_id=?", cid, uid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = svc.List(ctx, uid, cid, 20, 0); !errors.Is(err, apperrors.ErrForbidden) {
		t.Fatalf("waiting read: %v", err)
	}
	if err = db.Exec("UPDATE conference_participants SET admission_state='admitted',status='left' WHERE conference_id=? AND user_id=?", cid, uid).Error; err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"deleted_at=now()", "ended_at=now()-interval '8 days'"} {
		rid := p1ID("record", 20)
		fenced := recordings.NewConferenceService(&p1ChangedPage{ConferenceRecordingRepository: repo, afterList: func() {
			if e := db.Exec("UPDATE record SET "+change+" WHERE uuid=?", rid).Error; e != nil {
				t.Fatal(e)
			}
		}}, reader, nil)
		if items, e := fenced.List(ctx, uid, cid, 20, 0); !errors.Is(e, gorm.ErrRecordNotFound) || items != nil {
			t.Fatalf("availability recheck %s: rows=%d err=%v", change, len(items), e)
		}
		if err = db.Exec("UPDATE record SET deleted_at=NULL,ended_at=NULL WHERE uuid=?", rid).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Exec("UPDATE record SET mode='legacy',source_type='browser',platform_conference_id=NULL WHERE uuid=?", p1ID("record", 20)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadComposites(ctx, []string{p1ID("record", 20)}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("legacy record entered conference batch: %v", err)
	}
	empty, err := reader.ReadComposites(ctx, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty batch=%v %v", empty, err)
	}
	// The chat decorator already batches attachments and reply previews. Exercise
	// its non-empty branches rather than inferring a budget from empty fixtures.
	if err = db.Exec(`UPDATE chat_messages SET reply_to_id=(SELECT id FROM chat_messages WHERE conference_id=? ORDER BY sequence LIMIT 1) WHERE conference_id=?`, cid, cid).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO chat_attachments(id,conference_id,owner_user_id,client_request_id,filename,mime_type,size,status,message_id,object_key,expires_at)
	SELECT gen_random_uuid(),conference_id,sender_user_id,gen_random_uuid(),'fixture.txt','text/plain',10,'attached',id,'fixture/'||id,now()+interval '1 day' FROM chat_messages WHERE conference_id=?`, cid).Error; err != nil {
		t.Fatal(err)
	}
	chat := pg.NewChatRepository(db)
	// Seven original page/attachment/reply/read queries plus the later chat
	// leave-policy check and batched personal bookmarks. The budget remains
	// constant for one and twenty messages; those access checks must not be removed.
	const chatPageQueries = 9
	for _, limit := range []int{1, 20} {
		count.Store(0)
		page, e := chat.List(ctx, uid, cid, "", limit)
		if e != nil {
			t.Fatal(e)
		}
		if count.Load() != chatPageQueries || len(page.Items) != limit {
			t.Fatalf("chat limit=%d queries=%d rows=%d", limit, count.Load(), len(page.Items))
		}
		for _, message := range page.Items {
			if len(message.Attachments) != 1 || message.ReplyPreview == nil {
				t.Fatal("missing attachment or reply")
			}
		}
		t.Logf("chat page=%d attachments+replies SQL=%d", limit, count.Load())
	}
}

type p1ChangedPage struct {
	*pg.ConferenceRecordingRepository
	afterList func()
}

func (r *p1ChangedPage) List(ctx context.Context, u, c string, l, o int) ([]records.Record, error) {
	rows, e := r.ConferenceRecordingRepository.List(ctx, u, c, l, o)
	if e == nil {
		r.afterList()
	}
	return rows, e
}
