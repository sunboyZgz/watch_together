package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
		"room server config app_env=%s log_level=%s debug_sync=%t instance_id=%q room_runtime_mode=%s ws_cross_instance_broadcast=%t ws_event_bus=%s",
		runtimeConfig.AppEnv,
		runtimeConfig.LogLevel,
		runtimeConfig.DebugSync,
		runtimeConfig.InstanceID,
		runtimeConfig.RoomRuntimeMode,
		runtimeConfig.WebSocket.CrossInstanceBroadcast,
		runtimeConfig.WebSocket.EventBus,
	)
	server := app.NewServer(app.Config{
		AppEnv:          runtimeConfig.AppEnv,
		Host:            runtimeConfig.Host,
		Port:            runtimeConfig.Port,
		LogLevel:        runtimeConfig.LogLevel,
		InstanceID:      runtimeConfig.InstanceID,
		RoomRuntimeMode: runtimeConfig.RoomRuntimeMode,
		DatabaseURL:     runtimeConfig.DatabaseURL,
		DebugSync:       runtimeConfig.DebugSync,
		Auth: app.AuthTokenConfig{
			JWTSecret:      runtimeConfig.Auth.JWTSecret,
			AccessTokenTTL: time.Duration(runtimeConfig.Auth.AccessTokenTTLHours) * time.Hour,
		},
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
			SeekMinInterval:           time.Duration(runtimeConfig.WebSocket.SeekMinIntervalMs) * time.Millisecond,
			CrossInstanceBroadcast:    runtimeConfig.WebSocket.CrossInstanceBroadcast,
			EventBus:                  runtimeConfig.WebSocket.EventBus,
		},
		NATS: app.NATSConfig{
			URL:     runtimeConfig.NATS.URL,
			Name:    runtimeConfig.NATS.Name,
			Subject: runtimeConfig.NATS.SubjectRoomBroadcast,
		},
		Media: app.MediaPlaybackConfig{
			DeliveryMode:           runtimeConfig.Media.DeliveryMode,
			SigningSecret:          runtimeConfig.Media.SigningSecret,
			URLTTL:                 time.Duration(runtimeConfig.Media.URLTTLSeconds) * time.Second,
			PublicBaseURL:          runtimeConfig.Media.PublicBaseURL,
			InternalBaseURL:        runtimeConfig.Media.InternalBaseURL,
			StorageEndpoint:        runtimeConfig.Media.StorageEndpoint,
			StorageBucket:          runtimeConfig.Media.StorageBucket,
			StorageRegion:          runtimeConfig.Media.StorageRegion,
			StorageAccessKeyID:     runtimeConfig.Media.StorageAccessKeyID,
			StorageSecretAccessKey: runtimeConfig.Media.StorageSecretAccessKey,
			StorageForcePathStyle:  runtimeConfig.Media.StorageForcePathStyle,
		},
	})

	log.Printf("room server listening on %s", server.Address())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("room server graceful shutdown failed: %v", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("room server close failed: %v", closeErr)
			}
		}
	}
}
