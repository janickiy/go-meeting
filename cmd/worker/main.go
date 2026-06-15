package main

import (
	"log"

	"git.svc-dev.net/board/go-recorder/internal/app"
)

// main запускает отдельный recorder-worker binary.
// Параметры: нет.
// Возвращает: ничего; при критической ошибке завершает процесс через log.Fatalf.
func main() {
	if err := app.RunWorker(); err != nil {
		log.Fatalf("worker: %v", err)
	}
}
