package ffmpeg

import (
	"context"
	"github.com/janickiy/go-recorder/internal/operations"
	"os/exec"
	"syscall"
	"time"
)

// runBounded выполняет бинарный файл напрямую, без shell и интерполяции команд.
// ctx задаёт дедлайн, binary — доверенный исполняемый файл, args — отдельные
// аргументы. Возвращает не более 64 КиБ диагностики и ошибку процесса. Отмена
// сначала посылает SIGTERM, через 3 секунды Go завершает процесс принудительно.
func runBounded(ctx context.Context, binary string, args ...string) ([]byte, error) {
	started := time.Now()
	operations.FFmpegActive(1)
	defer func() { operations.FFmpegActive(-1); operations.Observe("ffmpeg", time.Since(started).Seconds()) }()
	cmd := exec.CommandContext(ctx, binary, args...)
	output := &logTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if err != nil {
		operations.Event("ffmpeg_failed")
	}
	return []byte(output.String()), err
}
