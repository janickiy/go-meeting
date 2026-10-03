package webrtc

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestFailedSessionClosesBeforeFailureCallback проверяет сценарий «Failed сессия Closes до сбой Callback», фиксируя ошибки поведения как регрессию.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestFailedSessionClosesBeforeFailureCallback(t *testing.T) {
	m := &Manager{storage: t.TempDir(), logger: log.New(io.Discard, "", 0), sessions: make(map[string]*session)}
	if err := m.Prepare("test-record", 1); err != nil {
		t.Fatal(err)
	}
	s, err := m.session("test-record")
	if err != nil {
		t.Fatal(err)
	}
	pc, err := pionwebrtc.NewPeerConnection(pionwebrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	s.pc = pc
	calls := 0
	// Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	//
	// @args
	//   - _ (context.Context): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
	//   - recordID (string): внешний UUID задачи записи.
	//   - _ (error): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
	m.onFailed = func(_ context.Context, recordID string, _ error) {
		calls++
		if recordID != "test-record" || s.ctx.Err() == nil {
			t.Error("failure callback ran before session cancellation")
		}
		if pc.ConnectionState() != pionwebrtc.PeerConnectionStateClosed {
			t.Error("failure callback ran before peer connection was closed")
		}
		if _, err := m.session(recordID); err == nil {
			t.Error("failed session is still registered")
		}
	}
	s.fail(errors.New("track timeout"))
	s.fail(errors.New("late ICE failure"))
	if calls != 1 {
		t.Fatalf("failure callback called %d times", calls)
	}
	if err := s.startFFmpeg(); err == nil {
		t.Error("failed session can still start recording")
	}
}

// TestDuplicatePreparePreservesSession проверяет сохранность сессии при повторном Prepare.
//
// @args
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestDuplicatePreparePreservesSession(t *testing.T) {
	m := &Manager{storage: t.TempDir(), sessions: make(map[string]*session)}
	if err := m.Prepare("test-record", 5); err != nil {
		t.Fatal(err)
	}
	original, err := m.session("test-record")
	if err != nil {
		t.Fatal(err)
	}
	defer original.cancel()
	if err := m.Prepare("test-record", 30); err != nil {
		t.Fatal(err)
	}
	got, err := m.session("test-record")
	if err != nil || got != original || got.segmentDurationSec != 5 {
		t.Fatal("duplicate prepare changed the live session")
	}
}
