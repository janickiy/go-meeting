package main

import (
	"log"

	"github.com/janickiy/meet-space/internal/app"
)

func main() {
	if err := app.RunAPI(); err != nil {
		log.Fatalf("api: %v", err)
	}
}
