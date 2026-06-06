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

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/store"
	"watch_together/server/internal/telemetry"
	"watch_together/server/internal/timeline"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(runtimeConfig.DatabaseURL) == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required for timelineservice")
		os.Exit(1)
	}
	if strings.EqualFold(strings.TrimSpace(runtimeConfig.AppEnv), "prod") &&
		strings.TrimSpace(runtimeConfig.InternalRPC.AuthToken) == "" {
		fmt.Fprintln(os.Stderr, "INTERNAL_RPC_AUTH_TOKEN is required for timelineservice in prod")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-timelineservice"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-timelineservice")
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

	db, err := store.OpenPostgres(ctx, runtimeConfig.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	sqlDB, err := db.DB()
	if err == nil {
		defer sqlDB.Close()
	}
	outboxStore := store.NewPostgresTimelineOutboxStore(db, runtimeConfig.Kafka.TopicRoomTimeline)

	var roomReader timeline.RoomEventReader
	if len(runtimeConfig.Kafka.Brokers) > 0 {
		roomReader, err = timeline.NewKafkaRoomEventReader(
			runtimeConfig.Kafka.Brokers,
			runtimeConfig.Kafka.TopicRoomTimeline,
			time.Duration(runtimeConfig.AuthorityRecovery.KafkaReplayTimeoutMs)*time.Millisecond,
		)
		if err != nil {
			log.Printf("kafka room event reader disabled: %v", err)
		}
	}

	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	installServiceEndpoints(
		mux,
		runtimeConfig,
		serviceConfig,
		metrics,
		observability.NewReadinessSnapshot(
			runtimeConfig.AppEnv,
			runtimeConfig.InstanceID,
			"timelineservice",
			[]observability.DependencyStatus{
				{Name: "postgres", Status: "ok", Required: true},
				dependencyStatus("kafka", roomReader != nil, false),
				{Name: "internal_rpc", Status: "ok", Required: true},
			},
		),
	)
	timeline.RegisterInternalRPC(
		mux,
		runtimeConfig.InternalRPC.PathPrefix,
		runtimeConfig.InternalRPC.AuthToken,
		outboxStore,
		roomReader,
		outboxStore,
	)

	addr := strings.TrimSpace(runtimeConfig.InternalRPC.Addr)
	if addr == "" {
		addr = ":8090"
	}
	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("timelineservice listening on %s", addr)
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
			log.Printf("timelineservice graceful shutdown failed: %v", err)
		}
	}
}

func installServiceEndpoints(
	mux *http.ServeMux,
	config wtconfig.ServerRuntimeConfig,
	service servicekit.Config,
	metrics *observability.Metrics,
	readiness observability.ReadinessSnapshot,
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

func dependencyStatus(name string, ok bool, required bool) observability.DependencyStatus {
	status := "disabled"
	if ok {
		status = "ok"
	} else if required {
		status = "unavailable"
	}
	return observability.DependencyStatus{Name: name, Status: status, Required: required}
}

func serviceName(configured string, fallback string) string {
	if strings.TrimSpace(configured) != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
}
