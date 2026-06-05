package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/store"
	"watch_together/server/internal/timeline"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}
	if runtimeConfig.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required for outboxworker")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.OpenPostgres(ctx, runtimeConfig.DatabaseURL)
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
	dispatcher := timeline.NewOutboxDispatcher(
		outboxStore,
		publisher,
		runtimeConfig.OutboxWorker.BatchSize,
		time.Duration(runtimeConfig.OutboxWorker.PollIntervalMs)*time.Millisecond,
	)
	log.Printf("outboxworker started topic=%s brokers=%v", runtimeConfig.Kafka.TopicRoomTimeline, runtimeConfig.Kafka.Brokers)
	if err := dispatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("outboxworker stopped: %v", err)
	}
}
