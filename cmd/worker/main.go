package main

import (
	"log"

	"github.com/janickiy/go-recorder/internal/app"
)

// main запускает отдельный recorder-worker binary.
func main() {
	if err := app.RunWorker(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
