package timeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	EventVersion = 1

	EventTypeControlAccepted = "room.control.accepted"
	EventTypeControlRejected = "room.control.rejected"
	EventTypeMemberJoined    = "room.member.joined"
	EventTypeMemberLeft      = "room.member.left"

	DefaultCanonicalTopic     = "wt.room.timeline.v1"
	DefaultControlResultTopic = "wt.room.control_result.v1"
	DefaultMembershipTopic    = "wt.room.membership.v1"
)

type Event struct {
	EventID      string          `json:"eventId"`
	EventVersion int             `json:"eventVersion"`
	EventType    string          `json:"eventType"`
	RoomID       string          `json:"roomId"`
	UserID       string          `json:"userId,omitempty"`
	DeviceID     string          `json:"deviceId,omitempty"`
	ConnectionID string          `json:"connectionId,omitempty"`
	InstanceID   string          `json:"instanceId,omitempty"`
	ControlType  string          `json:"controlType,omitempty"`
	Seq          int64           `json:"seq,omitempty"`
	OccurredAtMs int64           `json:"occurredAtMs"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type Recorder interface {
	RecordTimelineEvent(ctx context.Context, event Event) error
}

type NoopRecorder struct{}

func (NoopRecorder) RecordTimelineEvent(context.Context, Event) error {
	return nil
}

func NewEvent(eventType string, roomID string, now time.Time) Event {
	return Event{
		EventID:      NewEventID(),
		EventVersion: EventVersion,
		EventType:    eventType,
		RoomID:       roomID,
		OccurredAtMs: now.UnixMilli(),
	}
}

func MarshalEvent(event Event) ([]byte, error) {
	if event.EventID == "" {
		event.EventID = NewEventID()
	}
	if event.EventVersion == 0 {
		event.EventVersion = EventVersion
	}
	return json.Marshal(event)
}

func UnmarshalEvent(data []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func NewEventID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes[:])
}

type Topics struct {
	Canonical     string
	ControlResult string
	Membership    string
}

func (t Topics) Normalize() Topics {
	if t.Canonical == "" {
		t.Canonical = DefaultCanonicalTopic
	}
	if t.ControlResult == "" {
		t.ControlResult = DefaultControlResultTopic
	}
	if t.Membership == "" {
		t.Membership = DefaultMembershipTopic
	}
	return t
}

type Publication struct {
	Topic string
	Key   []byte
	Value []byte
}

func DerivePublications(event Event, topics Topics) ([]Publication, error) {
	topics = topics.Normalize()
	value, err := MarshalEvent(event)
	if err != nil {
		return nil, err
	}
	key := []byte(event.RoomID)
	switch event.EventType {
	case EventTypeControlAccepted, EventTypeControlRejected:
		return []Publication{{Topic: topics.ControlResult, Key: key, Value: value}}, nil
	case EventTypeMemberJoined, EventTypeMemberLeft:
		return []Publication{{Topic: topics.Membership, Key: key, Value: value}}, nil
	default:
		return nil, nil
	}
}
