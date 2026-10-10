package main

import (
	"github.com/janickiy/meet-space/internal/app"
	"log/slog"
	"os"
)

// main запускает независимый live-worker; ошибки не раскрывают входное аудио и настройки секретов.
func main() {
	if app.RunLiveWorker() != nil {
		slog.Error("live worker stopped", "event_type", "live.bootstrap.failed")
		os.Exit(1)
	}
}
