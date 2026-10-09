package main

import (
	"log"

	"anonblossom/internal/app"
	"anonblossom/internal/config"
)

// main starts the anonblossom HTTP server.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	server, err := app.NewServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}
