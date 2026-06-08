package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	databaseURL := timelineDatabaseURL(runtimeConfig)
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "TIMELINE_DATABASE_URL or DATABASE_URL is required for outboxworker")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serviceConfig := servicekit.Config{
		ServiceName:    serviceName(runtimeConfig.Service.Name, "watch-together-outboxworker"),
		ServiceVersion: runtimeConfig.Service.Version,
		InstanceID:     runtimeConfig.InstanceID,
	}.Normalized("watch-together-outboxworker")
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

	publisher, err := timeline.NewKafkaPublisher(runtimeConfig.Kafka.Brokers, runtimeConfig.Kafka.ClientID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open kafka publisher: %v\n", err)
		os.Exit(1)
	}
	defer publisher.Close()

	outboxStore := store.NewPostgresTimelineOutboxStore(db, runtimeConfig.Kafka.TopicRoomTimeline)
	metrics := observability.NewMetrics()
	observability.StartMetricsServer(ctx, observability.Config{
		MetricsEnabled: runtimeConfig.Observability.MetricsEnabled,
		MetricsAddr:    runtimeConfig.Observability.MetricsAddr,
		MetricsPath:    runtimeConfig.Observability.MetricsPath,
	}, metrics)
	dispatcher := timeline.NewOutboxDispatcher(
		outboxStore,
		publisher,
		runtimeConfig.OutboxWorker.BatchSize,
		time.Duration(runtimeConfig.OutboxWorker.PollIntervalMs)*time.Millisecond,
	)
	dispatcher.SetObserver(metrics)
	log.Printf("outboxworker started topic=%s brokers=%v", runtimeConfig.Kafka.TopicRoomTimeline, runtimeConfig.Kafka.Brokers)
	if err := dispatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("outboxworker stopped: %v", err)
	}
}

func serviceName(configured string, fallback string) string {
	if configured != "" && configured != "watch-together-roomserver" {
		return configured
	}
	return fallback
}

func timelineDatabaseURL(config wtconfig.ServerRuntimeConfig) string {
	if strings.TrimSpace(config.TimelineDatabaseURL) != "" {
		return strings.TrimSpace(config.TimelineDatabaseURL)
	}
	return strings.TrimSpace(config.DatabaseURL)
}
