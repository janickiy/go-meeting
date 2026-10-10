package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/janickiy/meet-space/internal/app"
	"github.com/janickiy/meet-space/internal/buildinfo"
)

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "version":
		_ = json.NewEncoder(os.Stdout).Encode(buildinfo.Current())
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
