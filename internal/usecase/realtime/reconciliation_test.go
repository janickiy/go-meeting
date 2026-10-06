package realtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domain "github.com/janickiy/go-recorder/internal/domain/realtime"
)

type reconciliationStore struct {
	Store
	missing []string
	err     error
	checked []string
}

func (s *reconciliationStore) Missing(_ context.Context, ids []string) ([]string, error) {
	s.checked = append([]string(nil), ids...)
	return s.missing, s.err
}

type reconciliationRepository struct {
	Repository
	closed map[string]time.Time
}

func (r *reconciliationRepository) Close(_ context.Context, id string, seen time.Time) error {
	r.closed[id] = seen
	return nil
}

func TestReconciliationClosesOnlyConfirmedMissingRoutes(t *testing.T) {
	seen := time.Now().UTC()
	rows := []domain.Session{{ConnectionID: "live", LastSeenAt: seen}, {ConnectionID: "expired", LastSeenAt: seen.Add(-time.Minute)}}
	for _, failed := range []bool{false, true} {
		store := &reconciliationStore{missing: []string{"expired", "unknown"}}
		if failed {
			store.err = errors.New("partial Redis pipeline failure")
		}
		repo := &reconciliationRepository{closed: map[string]time.Time{}}
		hub := &Hub{repo: repo, store: store}
		hub.closeMissingSessions(context.Background(), rows)
		if !reflect.DeepEqual(store.checked, []string{"live", "expired"}) {
			t.Fatal(store.checked)
		}
		want := map[string]time.Time{}
		if !failed {
			want["expired"] = rows[1].LastSeenAt
		}
		if !reflect.DeepEqual(repo.closed, want) {
			t.Fatalf("failure=%v closed=%v want=%v", failed, repo.closed, want)
		}
	}
}
