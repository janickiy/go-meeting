package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBoundedProcessHelper — дочерний исполняемый тестовый процесс; t не
// используется при обычном запуске. Режим задаётся последним аргументом.
func TestBoundedProcessHelper(t *testing.T) {
	if os.Getenv("RECORDER_PROCESS_HELPER") != "true" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "output":
		fmt.Print(strings.Repeat("x", 2<<20) + "TAIL")
	case "hang":
		signal.Ignore(syscall.SIGTERM)
		fmt.Print("READY")
		for {
			time.Sleep(time.Hour)
		}
	default:
		fmt.Print(os.Args[len(os.Args)-1])
	}
	os.Exit(0)
}

// TestRunBoundedOutputCancelAndArguments проверяет лимит вывода, SIGKILL после
// игнорирования SIGTERM и отсутствие shell-интерполяции аргументов. t задаёт env.
func TestRunBoundedOutputCancelAndArguments(t *testing.T) {
	t.Setenv("RECORDER_PROCESS_HELPER", "true")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runBounded(context.Background(), executable, "-test.run=^TestBoundedProcessHelper$", "--", "output")
	if err != nil || len(out) > maxStderrBytes || !strings.HasSuffix(string(out), "TAIL") {
		t.Fatalf("unbounded output: %d %v", len(out), err)
	}
	input := "literal; $(echo forbidden)"
	out, err = runBounded(context.Background(), executable, "-test.run=^TestBoundedProcessHelper$", "--", input)
	if err != nil || string(out) != input {
		t.Fatal("arguments were interpreted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = runBounded(ctx, executable, "-test.run=^TestBoundedProcessHelper$", "--", "hang")
	if err == nil || time.Since(started) > 5*time.Second {
		t.Fatal("child termination is not bounded")
	}
}
