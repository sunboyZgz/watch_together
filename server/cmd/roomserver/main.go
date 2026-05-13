package main

import (
	"fmt"
	"log"
	"os"

	"watch_together/server/internal/app"
	wtconfig "watch_together/server/internal/config"
)

// main wires config, server assembly, and the HTTP listen lifecycle together.
func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	log.Printf(
		"room server config app_env=%s log_level=%s debug_sync=%t",
		runtimeConfig.AppEnv,
		runtimeConfig.LogLevel,
		runtimeConfig.DebugSync,
	)
	server := app.NewServer(app.Config{
		AppEnv:      runtimeConfig.AppEnv,
		Host:        runtimeConfig.Host,
		Port:        runtimeConfig.Port,
		LogLevel:    runtimeConfig.LogLevel,
		DatabaseURL: runtimeConfig.DatabaseURL,
		DebugSync:   runtimeConfig.DebugSync,
	})

	log.Printf("room server listening on %s", server.Address())
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
