package main

import (
	"log/slog"
	"os"

	"github.com/janickiy/go-recorder/internal/app"
)

// main запускает только фоновые продуктовые задания; ошибка не выводит configuration secrets.
func main() {
	if app.RunProductWorker() != nil {
		slog.Error("product worker stopped", "event_type", "product.bootstrap.failed")
		os.Exit(1)
	}
}
