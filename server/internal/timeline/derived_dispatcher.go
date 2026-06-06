package timeline

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
)

type MessageReader interface {
	ReadMessage(ctx context.Context) (Message, error)
	Close() error
}

type Message struct {
	Key   []byte
	Value []byte
}

type KafkaMessageReader struct {
	reader *kafka.Reader
}

func NewKafkaTimelineReader(brokers []string, topic string, groupID string) (*KafkaMessageReader, error) {
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
	if strings.TrimSpace(groupID) == "" {
		groupID = "watch-together-derived-workers"
	}
	return &KafkaMessageReader{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        cleaned,
			Topic:          topic,
			GroupID:        groupID,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: time.Second,
		}),
	}, nil
}

func (r *KafkaMessageReader) ReadMessage(ctx context.Context) (Message, error) {
	if r == nil || r.reader == nil {
		return Message{}, errors.New("kafka reader is disabled")
	}
	message, err := r.reader.ReadMessage(ctx)
	if err != nil {
		return Message{}, err
	}
	return Message{Key: message.Key, Value: message.Value}, nil
}

func (r *KafkaMessageReader) Close() error {
	if r == nil || r.reader == nil {
		return nil
	}
	return r.reader.Close()
}

type DerivedDispatcher struct {
	reader    MessageReader
	publisher Publisher
	topics    Topics
	observer  WorkerObserver
}

func NewDerivedDispatcher(reader MessageReader, publisher Publisher, topics Topics) *DerivedDispatcher {
	return &DerivedDispatcher{
		reader:    reader,
		publisher: publisher,
		topics:    topics.Normalize(),
	}
}

func (d *DerivedDispatcher) SetObserver(observer WorkerObserver) {
	if d == nil {
		return
	}
	d.observer = observer
}

func (d *DerivedDispatcher) Run(ctx context.Context) error {
	if d == nil || d.reader == nil || d.publisher == nil {
		return nil
	}
	for {
		message, err := d.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("timeline derived read failed: %v", err)
			continue
		}
		if err := d.DispatchMessage(ctx, message); err != nil {
			log.Printf("timeline derived dispatch failed: %v", err)
		}
	}
}

func (d *DerivedDispatcher) DispatchMessage(ctx context.Context, message Message) error {
	event, err := UnmarshalEvent(message.Value)
	if err != nil {
		return err
	}
	publications, err := DerivePublications(event, d.topics)
	if err != nil {
		return err
	}
	for _, publication := range publications {
		if err := d.publisher.Publish(ctx, publication.Topic, publication.Key, publication.Value); err != nil {
			d.recordWorkerEvent("derivedworker", "publish_failed")
			return err
		}
		d.recordWorkerEvent("derivedworker", "published")
	}
	return nil
}

func (d *DerivedDispatcher) recordWorkerEvent(worker string, result string) {
	if d != nil && d.observer != nil {
		d.observer.RecordWorkerEvent(worker, result)
	}
}
