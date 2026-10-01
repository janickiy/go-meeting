package ffmpeg

import (
	"strings"
	"sync"
	"testing"
)

// TestLogTailRetainsNewestDiagnostics проверяет сценарий «Log Tail Retains Newest Diagnostics», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestLogTailRetainsNewestDiagnostics(t *testing.T) {
	var b logTail
	for _, chunk := range []string{strings.Repeat("x", maxStderrBytes*2), "last error"} {
		if n, err := b.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("Write() = %d, %v", n, err)
		}
	}
	if got := b.String(); len(got) != maxStderrBytes || !strings.HasSuffix(got, "last error") {
		t.Fatal("expected bounded log tail with the latest error")
	}
}

// TestLogTailConcurrentReadsAndWrites проверяет сценарий «Log Tail одновременный Reads и Writes», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestLogTailConcurrentReadsAndWrites(t *testing.T) {
	var b logTail
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer wg.Done()
			for range 1000 {
				_, _ = b.Write([]byte("ffmpeg diagnostic\n"))
				if len(b.String()) > maxStderrBytes {
					t.Error("log tail exceeded its limit")
				}
			}
		}()
	}
	wg.Wait()
}
