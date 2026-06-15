package main

import (
	"log"
	"os"

	"git.svc-dev.net/board/go-recorder/internal/app"
)

// main запускает общий entrypoint приложения.
// Параметры:
// - os.Args[1]: команда serve/api, worker или migrate; если команда не задана, используется serve.
// Возвращает: ничего; при критической ошибке завершает процесс через log.Fatalf.
func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "serve", "api":
		if err := app.RunAPI(); err != nil {
			log.Fatalf("api: %v", err)
		}
	case "worker":
		if err := app.RunWorker(); err != nil {
			log.Fatalf("worker: %v", err)
		}
	case "migrate":
		if err := app.RunMigrations(); err != nil {
			log.Fatalf("migrate: %v", err)
		}
	default:
		log.Fatalf("unknown command %q", command)
	}
}
