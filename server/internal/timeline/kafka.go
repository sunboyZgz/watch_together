package timeline

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
)

type Publisher interface {
	Publish(ctx context.Context, topic string, key []byte, value []byte) error
	Close() error
}

type KafkaPublisher struct {
	writer *kafka.Writer
}

func NewKafkaPublisher(brokers []string, clientID string) (*KafkaPublisher, error) {
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
	transport := &kafka.Transport{}
	if strings.TrimSpace(clientID) != "" {
		transport.ClientID = clientID
	}
	return &KafkaPublisher{
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(cleaned...),
			AllowAutoTopicCreation: true,
			Balancer:               &kafka.Hash{},
			Transport:              transport,
			WriteTimeout:           10 * time.Second,
			ReadTimeout:            10 * time.Second,
		},
	}, nil
}

func (p *KafkaPublisher) Publish(ctx context.Context, topic string, key []byte, value []byte) error {
	ctx, span := otel.Tracer("watch_together/kafka").Start(ctx, "kafka.publish."+topic)
	defer span.End()
	if p == nil || p.writer == nil {
		return errors.New("kafka publisher is disabled")
	}
	if strings.TrimSpace(topic) == "" {
		return errors.New("kafka topic is required")
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: value,
		Time:  time.Now(),
	})
}

func (p *KafkaPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
