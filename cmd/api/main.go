package main

import (
	"log"

	"github.com/janickiy/go-recorder/internal/app"
)

// main запускает отдельный API binary.
// Параметры: нет.
// Возвращает: ничего; при критической ошибке завершает процесс через log.Fatalf.
func main() {
	if err := app.RunAPI(); err != nil {
		log.Fatalf("api: %v", err)
	}
}
