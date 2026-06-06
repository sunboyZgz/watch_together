package timeline

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDerivePublicationsRoutesControlAndMembershipEvents(t *testing.T) {
	topics := Topics{
		ControlResult: "control.topic",
		Membership:    "membership.topic",
	}

	control := NewEvent(EventTypeControlAccepted, "ROOM01", time.UnixMilli(1000))
	control.Payload = json.RawMessage(`{"type":"play"}`)
	controlPublications, err := DerivePublications(control, topics)
	if err != nil {
		t.Fatalf("derive control: %v", err)
	}
	if len(controlPublications) != 1 || controlPublications[0].Topic != "control.topic" {
		t.Fatalf("unexpected control publications: %+v", controlPublications)
	}

	membership := NewEvent(EventTypeMemberJoined, "ROOM01", time.UnixMilli(1000))
	membershipPublications, err := DerivePublications(membership, topics)
	if err != nil {
		t.Fatalf("derive membership: %v", err)
	}
	if len(membershipPublications) != 1 || membershipPublications[0].Topic != "membership.topic" {
		t.Fatalf("unexpected membership publications: %+v", membershipPublications)
	}
}

func TestDerivedDispatcherPublishesDerivedEvent(t *testing.T) {
	event := NewEvent(EventTypeControlRejected, "ROOM01", time.UnixMilli(1000))
	value, err := MarshalEvent(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	publisher := &recordingPublisher{}
	dispatcher := NewDerivedDispatcher(nil, publisher, Topics{ControlResult: "control.topic"})
	observer := &recordingWorkerObserver{}
	dispatcher.SetObserver(observer)

	if err := dispatcher.DispatchMessage(context.Background(), Message{Value: value}); err != nil {
		t.Fatalf("dispatch message: %v", err)
	}
	if len(publisher.publications) != 1 || publisher.publications[0].topic != "control.topic" {
		t.Fatalf("unexpected publications: %+v", publisher.publications)
	}
	if observer.count("derivedworker", "published") != 1 {
		t.Fatalf("expected derivedworker published metric, got %+v", observer.events)
	}
}

func TestOutboxDispatcherRecordsWorkerEvents(t *testing.T) {
	store := &recordingOutboxStore{
		rows: []OutboxRow{{
			ID:      "outbox-1",
			Topic:   "timeline.topic",
			RoomID:  "ROOM01",
			Payload: []byte(`{"eventId":"evt-1"}`),
		}},
	}
	publisher := &recordingPublisher{}
	dispatcher := NewOutboxDispatcher(store, publisher, 10, time.Second)
	observer := &recordingWorkerObserver{}
	dispatcher.SetObserver(observer)

	if err := dispatcher.DispatchOnce(context.Background()); err != nil {
		t.Fatalf("dispatch outbox: %v", err)
	}
	if len(store.published) != 1 || store.published[0] != "outbox-1" {
		t.Fatalf("expected outbox row published, got %+v", store.published)
	}
	if observer.count("outboxworker", "published") != 1 {
		t.Fatalf("expected outboxworker published metric, got %+v", observer.events)
	}
}

type recordingPublisher struct {
	publications []recordedPublication
}

type recordedPublication struct {
	topic string
	key   []byte
	value []byte
}

func (p *recordingPublisher) Publish(ctx context.Context, topic string, key []byte, value []byte) error {
	p.publications = append(p.publications, recordedPublication{
		topic: topic,
		key:   append([]byte(nil), key...),
		value: append([]byte(nil), value...),
	})
	return nil
}

func (p *recordingPublisher) Close() error {
	return nil
}

type recordingOutboxStore struct {
	rows      []OutboxRow
	published []string
}

func (s *recordingOutboxStore) ClaimPending(ctx context.Context, batchSize int, now time.Time) ([]OutboxRow, error) {
	rows := s.rows
	s.rows = nil
	return rows, nil
}

func (s *recordingOutboxStore) MarkPublished(ctx context.Context, id string) error {
	s.published = append(s.published, id)
	return nil
}

func (s *recordingOutboxStore) MarkPublishFailed(ctx context.Context, id string, attempts int, lastError string, nextAttemptAt time.Time) error {
	return nil
}

type recordingWorkerObserver struct {
	events []recordedWorkerEvent
}

type recordedWorkerEvent struct {
	worker string
	result string
}

func (o *recordingWorkerObserver) RecordWorkerEvent(worker string, result string) {
	o.events = append(o.events, recordedWorkerEvent{worker: worker, result: result})
}

func (o *recordingWorkerObserver) count(worker string, result string) int {
	count := 0
	for _, event := range o.events {
		if event.worker == worker && event.result == result {
			count++
		}
	}
	return count
}
