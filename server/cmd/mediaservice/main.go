package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/media"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/store"
	"watch_together/server/internal/telemetry"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	databaseURL := mediaDatabaseURL(runtimeConfig)
	if strings.TrimSpace(databaseURL) == "" {
		fmt.Fprintln(os.Stderr, "MEDIA_DATABASE_URL or DATABASE_URL is required for mediaservice")
		os.Exit(1)
	}
	if strings.EqualFold(strings.TrimSpace(runtimeConfig.AppEnv), "prod") &&
		strings.TrimSpace(runtimeConfig.InternalRPC.AuthToken) == "" {
		fmt.Fprintln(os.Stderr, "INTERNAL_RPC_AUTH_TOKEN is required for mediaservice in prod")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-mediaservice"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-mediaservice")
	shutdownTelemetry, err := telemetry.Start(ctx, telemetry.Config{
		Enabled:      runtimeConfig.Telemetry.TracingEnabled,
		ServiceName:  runtimeConfig.Telemetry.ServiceName,
		OTLPEndpoint: runtimeConfig.Telemetry.OTLPEndpoint,
		SampleRatio:  runtimeConfig.Telemetry.TraceSampleRatio,
	}.Normalized(serviceConfig.ServiceName))
	if err != nil {
		log.Printf("failed to start telemetry; tracing disabled: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetry.Shutdown(shutdownCtx, shutdownTelemetry)
	}()

	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	mediaService := media.NewService(store.NewPostgresMediaStore(db))
	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(
		mux,
		runtimeConfig,
		serviceConfig,
		metrics,
		sqlDB,
	)
	media.RegisterInternalRPC(mux, runtimeConfig.InternalRPC.PathPrefix, runtimeConfig.InternalRPC.AuthToken, mediaService)

	addr := strings.TrimSpace(runtimeConfig.InternalRPC.Addr)
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("mediaservice listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("mediaservice graceful shutdown failed: %v", err)
		}
	}
}

func installServiceEndpoints(
	mux *http.ServeMux,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	metrics *observability.Metrics,
	sqlDB *sql.DB,
) {
	metricsPath := strings.TrimSpace(config.Observability.MetricsPath)
	if metricsPath == "" {
		metricsPath = observability.DefaultMetricsPath
	}
	readinessPath := strings.TrimSpace(config.Observability.ReadinessPath)
	if readinessPath == "" {
		readinessPath = observability.DefaultReadinessPath
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Watch-Together-Service", service.ServiceName)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc(readinessPath, func(w http.ResponseWriter, r *http.Request) {
		readiness := mediaserviceReadiness(r.Context(), config, sqlDB)
		status := http.StatusOK
		if readiness.Status != "ready" {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(readiness)
	})
	if config.Observability.MetricsEnabled {
		mux.Handle(metricsPath, metrics.Handler())
	}
}

func mediaserviceReadiness(ctx context.Context, config wtconfig.ServerRuntimeConfig, sqlDB *sql.DB) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		config.InstanceID,
		"mediaservice",
		[]observability.DependencyStatus{
			postgresDependency(ctx, sqlDB),
			{Name: "internal_rpc", Status: "ok", Required: true},
		},
	)
}

func postgresDependency(ctx context.Context, sqlDB *sql.DB) observability.DependencyStatus {
	status := "unavailable"
	if sqlDB != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if err := sqlDB.PingContext(pingCtx); err == nil {
			status = "ok"
		}
	}
	return observability.DependencyStatus{Name: "media_postgres", Status: status, Required: true}
}

func serviceName(configured string, fallback string) string {
	if strings.TrimSpace(configured) != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
}

func mediaDatabaseURL(config wtconfig.ServerRuntimeConfig) string {
	if strings.TrimSpace(config.MediaDatabaseURL) != "" {
		return strings.TrimSpace(config.MediaDatabaseURL)
	}
	return strings.TrimSpace(config.DatabaseURL)
}
