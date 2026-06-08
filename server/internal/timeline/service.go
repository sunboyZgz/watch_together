package timeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	MembershipResultJoined = "joined"
	MembershipResultLeft   = "left"
)

type EventWriter interface {
	RecordTimelineEvent(ctx context.Context, event Event) error
}

type UnpublishedReader interface {
	ReadRoomUnpublishedTimelineEvents(ctx context.Context, roomID string) ([]Event, error)
}

type Service struct {
	writer            EventWriter
	roomReader        RoomEventReader
	unpublishedReader UnpublishedReader
	now               func() time.Time
}

func NewService(
	writer EventWriter,
	roomReader RoomEventReader,
	unpublishedReader UnpublishedReader,
) *Service {
	return &Service{
		writer:            writer,
		roomReader:        roomReader,
		unpublishedReader: unpublishedReader,
		now:               time.Now,
	}
}

func (s *Service) RecordControlResult(ctx context.Context, result ControlResult) (Event, error) {
	if s == nil || s.writer == nil {
		return Event{}, ErrTimelineUnavailable
	}
	if strings.TrimSpace(result.RoomID) == "" {
		return Event{}, fmt.Errorf("%w: room_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(result.ControlType) == "" {
		return Event{}, fmt.Errorf("%w: control_type is required", ErrInvalidInput)
	}
	payload, err := marshalPayload(result.Payload)
	if err != nil {
		return Event{}, err
	}
	eventType := EventTypeControlRejected
	if result.Accepted {
		eventType = EventTypeControlAccepted
	}
	event := NewEvent(eventType, result.RoomID, s.currentTime())
	event.UserID = result.UserID
	event.DeviceID = result.DeviceID
	event.ConnectionID = result.ConnectionID
	event.InstanceID = result.InstanceID
	event.ControlType = result.ControlType
	event.Seq = result.Seq
	event.Payload = payload
	if err := s.writer.RecordTimelineEvent(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) RecordMembershipResult(ctx context.Context, result MembershipResult) (Event, error) {
	if s == nil || s.writer == nil {
		return Event{}, ErrTimelineUnavailable
	}
	if strings.TrimSpace(result.RoomID) == "" {
		return Event{}, fmt.Errorf("%w: room_id is required", ErrInvalidInput)
	}
	eventType, err := membershipEventType(result.MembershipType)
	if err != nil {
		return Event{}, err
	}
	payload, err := marshalPayload(result.Payload)
	if err != nil {
		return Event{}, err
	}
	event := NewEvent(eventType, result.RoomID, s.currentTime())
	event.UserID = result.UserID
	event.DeviceID = result.DeviceID
	event.ConnectionID = result.ConnectionID
	event.InstanceID = result.InstanceID
	event.Payload = payload
	if err := s.writer.RecordTimelineEvent(ctx, event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) ReadRoomRecoveryEvents(ctx context.Context, roomID string) ([]Event, error) {
	if s == nil || s.roomReader == nil {
		return nil, ErrTimelineUnavailable
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, fmt.Errorf("%w: room_id is required", ErrInvalidInput)
	}
	events, err := s.roomReader.ReadRoomEvents(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if s.unpublishedReader != nil {
		pending, err := s.unpublishedReader.ReadRoomUnpublishedTimelineEvents(ctx, roomID)
		if err != nil {
			return nil, err
		}
		events = append(events, pending...)
	}
	return MergeRecoveryEvents(events), nil
}

func MergeRecoveryEvents(events []Event) []Event {
	out := make([]Event, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.EventID != "" {
			if _, ok := seen[event.EventID]; ok {
				continue
			}
			seen[event.EventID] = struct{}{}
		}
		out = append(out, event)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OccurredAtMs != out[j].OccurredAtMs {
			return out[i].OccurredAtMs < out[j].OccurredAtMs
		}
		if out[i].Seq != out[j].Seq {
			return out[i].Seq < out[j].Seq
		}
		return out[i].EventID < out[j].EventID
	})
	return out
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func membershipEventType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case MembershipResultJoined, EventTypeMemberJoined:
		return EventTypeMemberJoined, nil
	case MembershipResultLeft, EventTypeMemberLeft:
		return EventTypeMemberLeft, nil
	default:
		return "", fmt.Errorf("%w: unsupported membership_type %q", ErrInvalidInput, value)
	}
}

func marshalPayload(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}
	switch value := payload.(type) {
	case json.RawMessage:
		return cloneBytes(value), nil
	case []byte:
		return cloneBytes(value), nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal timeline payload: %w", err)
		}
		return data, nil
	}
}
