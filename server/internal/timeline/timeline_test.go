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

	if err := dispatcher.DispatchMessage(context.Background(), Message{Value: value}); err != nil {
		t.Fatalf("dispatch message: %v", err)
	}
	if len(publisher.publications) != 1 || publisher.publications[0].topic != "control.topic" {
		t.Fatalf("unexpected publications: %+v", publisher.publications)
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
