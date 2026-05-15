package main

import (
	"fmt"
	"log"
	"os"
	"time"

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
		Redis: app.RedisConfig{
			Addr:       runtimeConfig.Redis.Addr,
			Username:   runtimeConfig.Redis.Username,
			Password:   runtimeConfig.Redis.Password,
			DB:         runtimeConfig.Redis.DB,
			TLSEnabled: runtimeConfig.Redis.TLSEnabled,
			Required:   runtimeConfig.Redis.Required,
		},
		WebSocket: app.WebSocketRuntimeConfig{
			BroadcastConcurrencyLimit: runtimeConfig.WebSocket.BroadcastConcurrencyLimit,
			BroadcastTimeout:          time.Duration(runtimeConfig.WebSocket.BroadcastTimeoutMs) * time.Millisecond,
			BroadcastEnqueueTimeout:   time.Duration(runtimeConfig.WebSocket.BroadcastEnqueueTimeoutMs) * time.Millisecond,
			ClientOutboxCapacity:      runtimeConfig.WebSocket.ClientOutboxCapacity,
			MaxConnections:            runtimeConfig.WebSocket.MaxConnections,
			MaxRoomClients:            runtimeConfig.WebSocket.MaxRoomClients,
		},
	})

	log.Printf("room server listening on %s", server.Address())
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
