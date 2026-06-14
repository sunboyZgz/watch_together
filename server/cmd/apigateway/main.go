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

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load api gateway config: %v\n", err)
		os.Exit(1)
	}
	runtimeConfig.EdgeMode = app.EdgeModeAPIGateway

	log.Printf(
		"api gateway config app_env=%s log_level=%s identity_mode=%s room_mode=%s media_mode=%s progress_mode=%s home_mode=%s authority_mode=%s",
		runtimeConfig.AppEnv,
		runtimeConfig.LogLevel,
		runtimeConfig.ServiceClients.IdentityMode,
		runtimeConfig.ServiceClients.RoomMode,
		runtimeConfig.ServiceClients.MediaMode,
		runtimeConfig.ServiceClients.ProgressMode,
		runtimeConfig.ServiceClients.HomeMode,
		runtimeConfig.ServiceClients.AuthorityMode,
	)

	server := app.NewServer(app.ConfigFromRuntime(runtimeConfig))
	log.Printf("api gateway listening on %s", server.Address())

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
			log.Printf("api gateway graceful shutdown failed: %v", err)
			if closeErr := server.Close(); closeErr != nil {
				log.Printf("api gateway close failed: %v", closeErr)
			}
		}
	}
}
