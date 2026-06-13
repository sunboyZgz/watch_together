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

	"watch_together/server/internal/auth"
	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/internalrpc"
	"watch_together/server/internal/media"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/store"
	"watch_together/server/internal/telemetry"

	"gorm.io/gorm"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.DatabaseURL) == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required for roomservice")
		os.Exit(1)
	}
	if strings.EqualFold(strings.TrimSpace(runtimeConfig.AppEnv), "prod") &&
		strings.TrimSpace(runtimeConfig.InternalRPC.AuthToken) == "" {
		fmt.Fprintln(os.Stderr, "INTERNAL_RPC_AUTH_TOKEN is required for roomservice in prod")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-roomservice"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-roomservice")
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

	mainDB, err := store.OpenPostgres(ctx, runtimeConfig.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	mainSQLDB, err := mainDB.DB()
	if err == nil {
		defer mainSQLDB.Close()
	}

	roomDB := mainDB
	var roomSQLDB *sql.DB
	if strings.TrimSpace(runtimeConfig.RoomDatabaseURL) != "" {
		opened, err := store.OpenPostgres(ctx, runtimeConfig.RoomDatabaseURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connect room_postgres: %v\n", err)
			os.Exit(1)
		}
		roomDB = opened
		if sqlDB, err := opened.DB(); err == nil {
			roomSQLDB = sqlDB
			defer sqlDB.Close()
		}
	} else {
		roomSQLDB = mainSQLDB
	}

	identityService := newIdentityBoundary(runtimeConfig, serviceConfig, mainDB)
	if identityService == nil {
		fmt.Fprintln(os.Stderr, "identity boundary is required for roomservice")
		os.Exit(1)
	}
	mediaService := newMediaBoundary(ctx, runtimeConfig, serviceConfig, mainDB)
	if mediaService == nil {
		fmt.Fprintln(os.Stderr, "media boundary is required for roomservice")
		os.Exit(1)
	}
	roomService := roomapi.NewServiceWithBoundaries(store.NewPostgresRoomStore(roomDB), mediaService, identityService)
	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(
		mux,
		runtimeConfig,
		serviceConfig,
		metrics,
		roomSQLDB,
	)
	roomapi.RegisterInternalRPC(mux, runtimeConfig.InternalRPC.PathPrefix, runtimeConfig.InternalRPC.AuthToken, roomService)

	addr := strings.TrimSpace(runtimeConfig.InternalRPC.Addr)
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("roomservice listening on %s", addr)
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
			log.Printf("roomservice graceful shutdown failed: %v", err)
		}
	}
}

func newMediaBoundary(
	ctx context.Context,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	db *gorm.DB,
) *media.Service {
	if strings.EqualFold(strings.TrimSpace(config.ServiceClients.MediaMode), "rpc") {
		rpcStore := media.NewRPCStore(config.ServiceClients.MediaAddr, internalrpc.ClientConfig{
			PathPrefix: config.InternalRPC.PathPrefix,
			Timeout:    time.Duration(config.InternalRPC.TimeoutMs) * time.Millisecond,
			AuthToken:  config.InternalRPC.AuthToken,
			Service:    service,
		})
		if rpcStore == nil {
			return nil
		}
		return media.NewService(rpcStore)
	}
	mediaDB := db
	if strings.TrimSpace(config.MediaDatabaseURL) != "" {
		opened, err := store.OpenPostgres(ctx, config.MediaDatabaseURL)
		if err != nil {
			log.Printf("failed to connect media_postgres for roomservice; using unavailable media boundary: %v", err)
			return nil
		}
		mediaDB = opened
	}
	if mediaDB == nil {
		return nil
	}
	return media.NewService(store.NewPostgresMediaStore(mediaDB))
}

func newIdentityBoundary(
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	db *gorm.DB,
) auth.IdentityService {
	if strings.EqualFold(strings.TrimSpace(config.ServiceClients.IdentityMode), "rpc") {
		return auth.NewRPCClient(config.ServiceClients.IdentityAddr, internalrpc.ClientConfig{
			PathPrefix: config.InternalRPC.PathPrefix,
			Timeout:    time.Duration(config.InternalRPC.TimeoutMs) * time.Millisecond,
			AuthToken:  config.InternalRPC.AuthToken,
			Service:    service,
		})
	}
	if db == nil {
		return nil
	}
	return auth.NewService(store.NewPostgresUserStore(db))
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
		readiness := roomserviceReadiness(r.Context(), config, sqlDB)
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

func roomserviceReadiness(ctx context.Context, config wtconfig.ServerRuntimeConfig, sqlDB *sql.DB) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		config.InstanceID,
		"roomservice",
		[]observability.DependencyStatus{
			roomPostgresDependency(ctx, sqlDB),
			{Name: "internal_rpc", Status: "ok", Required: true},
			roomIdentityDependency(ctx, config),
			roomMediaDependency(ctx, config),
		},
	)
}

func roomPostgresDependency(ctx context.Context, sqlDB *sql.DB) observability.DependencyStatus {
	status := "unavailable"
	if sqlDB != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if err := sqlDB.PingContext(pingCtx); err == nil {
			status = "ok"
		}
	}
	return observability.DependencyStatus{Name: "room_postgres", Status: status, Required: true}
}

func roomIdentityDependency(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	if !strings.EqualFold(strings.TrimSpace(config.ServiceClients.IdentityMode), "rpc") {
		return observability.DependencyStatus{Name: "identity_rpc", Status: "disabled", Required: false}
	}
	return rpcReadinessDependency(ctx, "identity_rpc", config.ServiceClients.IdentityAddr, config)
}

func roomMediaDependency(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	if !strings.EqualFold(strings.TrimSpace(config.ServiceClients.MediaMode), "rpc") {
		return observability.DependencyStatus{Name: "media_rpc", Status: "disabled", Required: false}
	}
	return rpcReadinessDependency(ctx, "media_rpc", config.ServiceClients.MediaAddr, config)
}

func rpcReadinessDependency(ctx context.Context, name string, addr string, config wtconfig.ServerRuntimeConfig) observability.DependencyStatus {
	baseURL := internalrpc.NormalizeBaseURL(addr)
	if baseURL == "" {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	readinessPath := strings.TrimSpace(config.Observability.ReadinessPath)
	if readinessPath == "" {
		readinessPath = observability.DefaultReadinessPath
	}
	if !strings.HasPrefix(readinessPath, "/") {
		readinessPath = "/" + readinessPath
	}
	probeCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+readinessPath, nil)
	if err != nil {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	response, err := (&http.Client{Timeout: 300 * time.Millisecond}).Do(request)
	if err != nil {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	var snapshot observability.ReadinessSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil || snapshot.Status != "ready" {
		return observability.DependencyStatus{Name: name, Status: "unavailable", Required: true}
	}
	return observability.DependencyStatus{Name: name, Status: "ok", Required: true}
}

func serviceName(configured string, fallback string) string {
	if strings.TrimSpace(configured) != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
}
