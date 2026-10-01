package main

import (
	"github.com/janickiy/go-recorder/internal/app"
	"log"
)

func main() {
	if err := app.RunMediaWorker(); err != nil {
		log.Fatalf("media-worker: %v", err)
	}
}
