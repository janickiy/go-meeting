package main

import (
	"log"

	"github.com/janickiy/go-recorder/internal/app"
)

// main запускает исполняемый компонент, собирает зависимости и обрабатывает завершение процесса.
func main() {
	if err := app.RunMediaWorker(); err != nil {
		log.Fatalf("media-worker: %v", err)
	}
}
