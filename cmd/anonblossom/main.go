package main

import (
	"log"
	"os"

	"anonblossom/internal/app"
	"anonblossom/internal/config"
)

// main starts the anonblossom HTTP server.
func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	server, err := app.NewServer(cfg)
	if err != nil {
		return err
	}
	defer server.Close()

	return server.Run()
}
