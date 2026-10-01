package main

import (
	"log"

	"github.com/janickiy/go-recorder/internal/app"
)

func main() {
	if err := app.RunAPI(); err != nil {
		log.Fatalf("api: %v", err)
	}
}
