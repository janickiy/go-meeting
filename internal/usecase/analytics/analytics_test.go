package analytics

import (
	domain "github.com/janickiy/go-recorder/internal/domain/analytics"
	"testing"
	"time"
)

// TestAggregatePresenceUnion объединяет вкладки и обрезает интервалы границами встречи.
// @args t — исполнитель теста.
func TestAggregatePresenceUnion(t *testing.T) {
	start := time.Unix(100, 0)
	at := func(s int) time.Time { return start.Add(time.Duration(s) * time.Second) }
	value := Aggregate("c", start, at(10), []domain.Interval{{ParticipantID: "a", Start: at(-5), End: at(6)}, {ParticipantID: "a", Start: at(4), End: at(8)}, {ParticipantID: "b", Start: at(5), End: at(20)}})
	if value.DurationMS != 10000 || value.ParticipantCount != 2 || value.Participants[0].ParticipationMS != 8000 || value.Participants[1].ParticipationMS != 5000 {
		t.Fatalf("bad aggregates %#v", value)
	}
	if len(value.Timeline) != 4 || value.Timeline[1].Count != 2 || value.Timeline[3].Count != 0 {
		t.Fatalf("bad timeline %#v", value.Timeline)
	}
}

// TestAggregateTimelineBound фиксирует предел памяти для частых переподключений.
// @args t — исполнитель теста.
func TestAggregateTimelineBound(t *testing.T) {
	start := time.Unix(100, 0)
	values := []domain.Interval{}
	for i := 0; i < 10000; i++ {
		values = append(values, domain.Interval{ParticipantID: "a", Start: start.Add(time.Duration(i*2) * time.Millisecond), End: start.Add(time.Duration(i*2+1) * time.Millisecond)})
	}
	result := Aggregate("c", start, start.Add(20*time.Second), values)
	if len(result.Timeline) > 600 || result.Participants[0].ParticipationMS != 10000 {
		t.Fatal("unbounded/lost duration")
	}
}
