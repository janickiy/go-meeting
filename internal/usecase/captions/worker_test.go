package captions

import (
	domain "github.com/janickiy/go-recorder/internal/domain/captions"
	"sync"
	"testing"
)

// TestCaptionRevisionAndValidation проверяет порядок, terminal final и границы недоверенного текста.
// @args t — изолированный исполнитель теста.
func TestCaptionRevisionAndValidation(t *testing.T) {
	old := domain.Event{UtteranceID: "a", Sequence: 2, Revision: 2, Text: "Привет", Language: "ru", StartMS: 0, EndMS: 100}
	if !ValidEvent(old, 20, 1000) {
		t.Fatal("valid event rejected")
	}
	for _, bad := range []domain.Event{{Text: ""}, {UtteranceID: "a", Text: "a\x00", Language: "ru"}, {UtteranceID: "a", Text: "a", Language: "xx"}} {
		if ValidEvent(bad, 20, 1000) {
			t.Fatal("bad event accepted")
		}
	}
	next := old
	next.Final = true
	if !Newer(old, next) || Newer(next, old) {
		t.Fatal("final ordering")
	}
	next = old
	next.Revision++
	next.Sequence--
	if Newer(old, next) {
		t.Fatal("sequence regressed")
	}
	next = old
	next.Revision++
	if !Newer(old, next) {
		t.Fatal("revision rejected")
	}
}

// TestActivityUnion проверяет, что два устройства не удваивают время речи, включая параллельный доступ.
// @args t — исполнитель теста с возможным race detector.
func TestActivityUnion(t *testing.T) {
	meter := &activityMeter{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var speech, observed int64
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, o := meter.observe("p", 0, 1000, true)
			mu.Lock()
			speech += s
			observed += o
			mu.Unlock()
		}()
	}
	wg.Wait()
	if speech != 1000 || observed != 1000 {
		t.Fatalf("double count %d/%d", speech, observed)
	}
	if s, o := meter.observe("p", 0, 1000, true); s != 0 || o != 0 {
		t.Fatal("duplicate audio")
	}
	if s, o := meter.observe("p", 1000, 2000, false); s != 0 || o != 1000 {
		t.Fatal("silence counted as speech")
	}
	if s, o := meter.observe("p", 1000, 2000, true); s != 1000 || o != 0 {
		t.Fatal("second device activity lost")
	}
	meter.observe("p", 60000, 61000, true)
	if s, o := meter.observe("p", 0, 1000, true); s != 0 || o != 0 {
		t.Fatal("stale frames counted")
	}
}
