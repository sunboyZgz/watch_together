package timeline

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
)

type RoomEventReader interface {
	ReadRoomEvents(ctx context.Context, roomID string) ([]Event, error)
}

type KafkaRoomEventReader struct {
	brokers     []string
	topic       string
	idleTimeout time.Duration
}

func NewKafkaRoomEventReader(
	brokers []string,
	topic string,
	idleTimeout time.Duration,
) (*KafkaRoomEventReader, error) {
	cleaned := make([]string, 0, len(brokers))
	for _, broker := range brokers {
		broker = strings.TrimSpace(broker)
		if broker != "" {
			cleaned = append(cleaned, broker)
		}
	}
	if len(cleaned) == 0 {
		return nil, errors.New("kafka brokers are required")
	}
	if strings.TrimSpace(topic) == "" {
		topic = DefaultCanonicalTopic
	}
	if idleTimeout <= 0 {
		idleTimeout = 500 * time.Millisecond
	}
	return &KafkaRoomEventReader{
		brokers:     cleaned,
		topic:       topic,
		idleTimeout: idleTimeout,
	}, nil
}

func (r *KafkaRoomEventReader) ReadRoomEvents(ctx context.Context, roomID string) ([]Event, error) {
	ctx, span := otel.Tracer("watch_together/kafka").Start(ctx, "kafka.read_room_events")
	defer span.End()
	if r == nil || len(r.brokers) == 0 {
		return nil, errors.New("kafka room event reader is disabled")
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, errors.New("roomID is required")
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     r.brokers,
		Topic:       r.topic,
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     r.idleTimeout,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	events := make([]Event, 0)
	for {
		readCtx, cancel := context.WithTimeout(ctx, r.idleTimeout)
		message, err := reader.ReadMessage(readCtx)
		cancel()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return events, nil
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		event, err := UnmarshalEvent(message.Value)
		if err != nil || event.RoomID != roomID {
			continue
		}
		events = append(events, event)
	}
}
