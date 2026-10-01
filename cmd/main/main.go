package main

import (
	"log"
	"os"

	"github.com/janickiy/go-recorder/internal/app"
)

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
