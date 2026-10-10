package main

import (
	"log/slog"
	"os"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает только фоновые продуктовые задания; ошибки не раскрывают секреты конфигурации.
func main() {
	if app.RunProductWorker() != nil {
		slog.Error("product worker stopped", "event_type", "product.bootstrap.failed")
		os.Exit(1)
	}
}
