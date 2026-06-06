package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	wtconfig "watch_together/server/internal/config"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/timeline"
)

func main() {
	runtimeConfig, err := wtconfig.LoadServerRuntimeConfig(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load server config: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reader, err := timeline.NewKafkaTimelineReader(
		runtimeConfig.Kafka.Brokers,
		runtimeConfig.Kafka.TopicRoomTimeline,
		runtimeConfig.Kafka.DerivedConsumerGroupID,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open kafka reader: %v\n", err)
		os.Exit(1)
	}
	defer reader.Close()

	publisher, err := timeline.NewKafkaPublisher(runtimeConfig.Kafka.Brokers, runtimeConfig.Kafka.ClientID+"-derived")
	if err != nil {
		fmt.Fprintf(os.Stderr, "open kafka publisher: %v\n", err)
		os.Exit(1)
	}
	defer publisher.Close()

	metrics := observability.NewMetrics()
	observability.StartMetricsServer(ctx, observability.Config{
		MetricsEnabled: runtimeConfig.Observability.MetricsEnabled,
		MetricsAddr:    runtimeConfig.Observability.MetricsAddr,
		MetricsPath:    runtimeConfig.Observability.MetricsPath,
	}, metrics)
	dispatcher := timeline.NewDerivedDispatcher(reader, publisher, timeline.Topics{
		Canonical:     runtimeConfig.Kafka.TopicRoomTimeline,
		ControlResult: runtimeConfig.Kafka.TopicRoomControlResult,
		Membership:    runtimeConfig.Kafka.TopicRoomMembership,
	})
	dispatcher.SetObserver(metrics)
	log.Printf("derivedworker started canonical=%s control=%s membership=%s brokers=%v",
		runtimeConfig.Kafka.TopicRoomTimeline,
		runtimeConfig.Kafka.TopicRoomControlResult,
		runtimeConfig.Kafka.TopicRoomMembership,
		runtimeConfig.Kafka.Brokers,
	)
	if err := dispatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("derivedworker stopped: %v", err)
	}
}
