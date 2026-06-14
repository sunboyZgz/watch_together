package main

import (
	"context"
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
	"watch_together/server/internal/home"
	"watch_together/server/internal/internalrpc"
	"watch_together/server/internal/media"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/progress"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/telemetry"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	if strings.EqualFold(strings.TrimSpace(runtimeConfig.AppEnv), "prod") &&
		strings.TrimSpace(runtimeConfig.InternalRPC.AuthToken) == "" {
		fmt.Fprintln(os.Stderr, "INTERNAL_RPC_AUTH_TOKEN is required for homecompositionservice in prod")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-homecompositionservice"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-homecompositionservice")
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

	identityService := newIdentityBoundary(runtimeConfig, serviceConfig)
	progressService := newProgressBoundary(runtimeConfig, serviceConfig)
	mediaService := newMediaBoundary(runtimeConfig, serviceConfig)
	if identityService == nil || progressService == nil || mediaService == nil {
		fmt.Fprintln(os.Stderr, "identity, progress, and media boundaries are required for homecompositionservice")
		os.Exit(1)
	}

	homeService := home.NewServiceWithComposition(identityService, progressService, mediaService)
	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(mux, runtimeConfig, serviceConfig, metrics)
	home.RegisterInternalRPC(mux, runtimeConfig.InternalRPC.PathPrefix, runtimeConfig.InternalRPC.AuthToken, homeService)

	addr := strings.TrimSpace(runtimeConfig.InternalRPC.Addr)
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("homecompositionservice listening on %s", addr)
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
			log.Printf("homecompositionservice graceful shutdown failed: %v", err)
		}
	}
}

func newIdentityBoundary(config wtconfig.ServerRuntimeConfig, service servicekit.Config) auth.IdentityService {
	if strings.EqualFold(strings.TrimSpace(config.ServiceClients.IdentityMode), "rpc") {
		return auth.NewRPCClient(config.ServiceClients.IdentityAddr, internalRPCClientConfig(config, service))
	}
	return nil
}

func newProgressBoundary(config wtconfig.ServerRuntimeConfig, service servicekit.Config) progress.BusinessService {
	if strings.EqualFold(strings.TrimSpace(config.ServiceClients.ProgressMode), "rpc") {
		return progress.NewRPCClient(config.ServiceClients.ProgressAddr, internalRPCClientConfig(config, service))
	}
	return nil
}

func newMediaBoundary(config wtconfig.ServerRuntimeConfig, service servicekit.Config) *media.Service {
	if strings.EqualFold(strings.TrimSpace(config.ServiceClients.MediaMode), "rpc") {
		rpcStore := media.NewRPCStore(config.ServiceClients.MediaAddr, internalRPCClientConfig(config, service))
		if rpcStore == nil {
			return nil
		}
		return media.NewService(rpcStore)
	}
	return nil
}

func installServiceEndpoints(
	mux *http.ServeMux,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	metrics *observability.Metrics,
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
		readiness := homecompositionReadiness(r.Context(), config)
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

func homecompositionReadiness(ctx context.Context, config wtconfig.ServerRuntimeConfig) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot(
		config.AppEnv,
		config.InstanceID,
		"homecompositionservice",
		[]observability.DependencyStatus{
			{Name: "internal_rpc", Status: "ok", Required: true},
			rpcDependency(ctx, "identity_rpc", config.ServiceClients.IdentityMode, config.ServiceClients.IdentityAddr, config),
			rpcDependency(ctx, "progress_rpc", config.ServiceClients.ProgressMode, config.ServiceClients.ProgressAddr, config),
			rpcDependency(ctx, "media_rpc", config.ServiceClients.MediaMode, config.ServiceClients.MediaAddr, config),
		},
	)
}

func rpcDependency(
	ctx context.Context,
	name string,
	mode string,
	addr string,
	config wtconfig.ServerRuntimeConfig,
) observability.DependencyStatus {
	if !strings.EqualFold(strings.TrimSpace(mode), "rpc") {
		return observability.DependencyStatus{Name: name, Status: "disabled", Required: false}
	}
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

func internalRPCClientConfig(config wtconfig.ServerRuntimeConfig, service servicekit.Config) internalrpc.ClientConfig {
	return internalrpc.ClientConfig{
		PathPrefix: config.InternalRPC.PathPrefix,
		Timeout:    time.Duration(config.InternalRPC.TimeoutMs) * time.Millisecond,
		AuthToken:  config.InternalRPC.AuthToken,
		Service:    service,
	}
}

func serviceName(configured string, fallback string) string {
	if strings.TrimSpace(configured) != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
}
