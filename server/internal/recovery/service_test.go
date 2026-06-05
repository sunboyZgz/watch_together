package recovery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/timeline"
)

func TestServiceRecoversStateFromKafkaAndPendingOutbox(t *testing.T) {
	ctx := context.Background()
	durationMs := int64(120_000)
	kafkaEvents := []timeline.Event{
		timelineControlEvent(t, "evt-1", timeline.EventTypeControlAccepted, protocol.TypePlay, protocol.PlayPayload{
			RoomID:       "ROOM01",
			UserID:       "user_a",
			RequestID:    "req-play",
			PositionMs:   10_000,
			Velocity:     1,
			ServerTimeMs: 1_000,
			Reason:       "play",
			Seq:          2,
		}),
		timelineControlEvent(t, "evt-2", timeline.EventTypeControlRejected, protocol.TypePause, map[string]any{
			"requestId": "req-rejected",
			"reason":    "stale",
		}),
		timelineControlEvent(t, "evt-3", timeline.EventTypeControlAccepted, protocol.TypePause, protocol.PausePayload{
			RoomID:       "ROOM01",
			UserID:       "user_a",
			RequestID:    "req-pause",
			PositionMs:   11_000,
			Velocity:     0,
			ServerTimeMs: 2_000,
			Reason:       "pause",
			Seq:          3,
		}),
	}
	pendingEvents := []timeline.Event{
		timelineControlEvent(t, "evt-3", timeline.EventTypeControlAccepted, protocol.TypePause, protocol.PausePayload{
			RoomID:       "ROOM01",
			UserID:       "user_a",
			RequestID:    "req-pause",
			PositionMs:   11_000,
			Velocity:     0,
			ServerTimeMs: 2_000,
			Reason:       "pause",
			Seq:          3,
		}),
		timelineControlEvent(t, "evt-4", timeline.EventTypeControlAccepted, protocol.TypeEnded, protocol.EndedPayload{
			RoomID:       "ROOM01",
			UserID:       "user_a",
			RequestID:    "req-ended",
			PositionMs:   120_000,
			Velocity:     0,
			ServerTimeMs: 3_000,
			Reason:       "media_end",
			Seq:          4,
		}),
	}

	roomManager := room.NewManager()
	writer := &recordingStateWriter{}
	service := NewService(
		Config{InstanceID: "instance-b", RecoveryTimeout: time.Second},
		&fakeRecoveryAuthority{},
		roomManager,
		&fakeRoomDetailStore{durationMs: &durationMs},
		fakeTimelineReader(kafkaEvents),
		fakePendingOutboxReader(pendingEvents),
		writer,
	)

	result, err := service.TryRecoverRoomAuthority(ctx, "ROOM01", "test")
	if err != nil {
		t.Fatalf("recover authority: %v", err)
	}
	if !result.Recovered || result.Lease.Status != cache.RoomAuthorityStatusActive || result.Lease.Epoch != 2 {
		t.Fatalf("unexpected recovery result: %+v", result)
	}
	if result.State.Seq != 4 || !result.State.Ended || result.State.PositionMs != 120_000 {
		t.Fatalf("unexpected recovered state: %+v", result.State)
	}
	if len(result.RequestIDs) != 3 {
		t.Fatalf("expected three accepted request IDs, got %+v", result.RequestIDs)
	}
	recoveredRoom, ok := roomManager.Get("ROOM01")
	if !ok {
		t.Fatalf("expected recovered room to be registered")
	}
	if state := recoveredRoom.StateSnapshot(); state.Seq != 4 || !state.Ended {
		t.Fatalf("unexpected manager recovered state: %+v", state)
	}
	if writer.last.Seq != 4 || writer.last.RoomID != "ROOM01" {
		t.Fatalf("expected room_state cache write, got %+v", writer.last)
	}
}

func timelineControlEvent(t *testing.T, id string, eventType string, controlType string, payload any) timeline.Event {
	t.Helper()
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	rawEnvelope, err := json.Marshal(protocol.Envelope{
		Type:    controlType,
		Payload: rawPayload,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return timeline.Event{
		EventID:      id,
		EventVersion: timeline.EventVersion,
		EventType:    eventType,
		RoomID:       "ROOM01",
		UserID:       "user_a",
		ControlType:  controlType,
		Seq:          responseSeqForTest(payload),
		OccurredAtMs: responseSeqForTest(payload) * 1_000,
		Payload:      rawEnvelope,
	}
}

func responseSeqForTest(payload any) int64 {
	switch value := payload.(type) {
	case protocol.PlayPayload:
		return value.Seq
	case protocol.PausePayload:
		return value.Seq
	case protocol.EndedPayload:
		return value.Seq
	}
	return 0
}

type fakeRecoveryAuthority struct{}

func (fakeRecoveryAuthority) BeginRecovery(context.Context, string, string) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{
		InstanceID:   "instance-b",
		Epoch:        2,
		Status:       cache.RoomAuthorityStatusRecovering,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}, true, nil
}

func (fakeRecoveryAuthority) CompleteRecovery(context.Context, string, string, int64) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{
		InstanceID:   "instance-b",
		Epoch:        2,
		Status:       cache.RoomAuthorityStatusActive,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}, true, nil
}

func (fakeRecoveryAuthority) GetAuthority(context.Context, string) (cache.RoomAuthorityLease, bool, error) {
	return cache.RoomAuthorityLease{}, false, nil
}

type fakeRoomDetailStore struct {
	durationMs *int64
}

func (s *fakeRoomDetailStore) GetRoomDetail(context.Context, string) (roomapi.DetailResult, error) {
	return roomapi.DetailResult{
		Room: roomapi.Room{
			RoomCode:    "ROOM01",
			HostUserID:  "user_a",
			MediaItemID: "sample_001",
			Status:      "active",
		},
		Media: roomapi.Media{
			ID:         "sample_001",
			DurationMs: s.durationMs,
		},
	}, nil
}

type fakeTimelineReader []timeline.Event

func (r fakeTimelineReader) ReadRoomEvents(context.Context, string) ([]timeline.Event, error) {
	return append([]timeline.Event(nil), r...), nil
}

type fakePendingOutboxReader []timeline.Event

func (r fakePendingOutboxReader) ReadRoomUnpublishedTimelineEvents(context.Context, string) ([]timeline.Event, error) {
	return append([]timeline.Event(nil), r...), nil
}

type recordingStateWriter struct {
	last protocol.RoomStatePayload
}

func (w *recordingStateWriter) SetRoomState(ctx context.Context, roomID string, state protocol.RoomStatePayload) error {
	w.last = state
	return nil
}
