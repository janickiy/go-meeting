package recorder

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
)

type retentionFixture struct {
	deleted  map[int64]bool
	attempts map[int64]int
}

func (f *retentionFixture) ExpireNextRecording(ctx context.Context, attempted []int64, remove func(context.Context, records.Record) error) (int64, error) {
	for id := int64(1); id <= 2; id++ {
		seen := f.deleted[id]
		for _, previous := range attempted {
			seen = seen || previous == id
		}
		if seen {
			continue
		}
		f.attempts[id]++
		err := remove(ctx, records.Record{ID: id})
		if err == nil {
			f.deleted[id] = true
		}
		return id, err
	}
	return 0, nil
}

type retentionStorage struct{ fail bool }

func (s retentionStorage) RemoveRecording(_ context.Context, record records.Record) error {
	if s.fail && record.ID == 1 {
		return errors.New("temporary storage error")
	}
	return nil
}

func TestRetentionFailureDoesNotBlockOtherRecordings(t *testing.T) {
	f := &retentionFixture{deleted: map[int64]bool{}, attempts: map[int64]int{}}
	count, err := SweepRetention(context.Background(), f, retentionStorage{fail: true})
	if count != 1 || err == nil || f.deleted[1] || !f.deleted[2] || f.attempts[1] != 1 {
		t.Fatal("failure blocked cleanup or was acknowledged", count, err, f)
	}
	count, err = SweepRetention(context.Background(), f, retentionStorage{})
	if count != 1 || err != nil || !f.deleted[1] || f.attempts[1] != 2 || f.attempts[2] != 1 {
		t.Fatal("failed recording was not retried", count, err, f)
	}
}
