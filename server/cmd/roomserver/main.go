package main

import (
	"log"

	"watch_together/server/internal/app"
)

// main wires config, server assembly, and the HTTP listen lifecycle together.
func main() {
	config := app.LoadConfigFromEnv()
	server := app.NewServer(config)

	log.Printf("room server listening on %s", server.Address())
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
