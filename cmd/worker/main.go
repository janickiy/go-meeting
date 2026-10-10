package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

// main запускает отдельный исполняемый файл recorder-worker.
func main() {
	if err := app.RunWorker(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
