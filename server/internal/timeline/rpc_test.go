package timeline

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
)

func TestRPCClientMatchesTimelineInterfaces(t *testing.T) {
	store := &fakeRPCTimelineStore{
		roomEvents:  []Event{{EventID: "evt-1", EventType: EventTypeControlAccepted, RoomID: "ROOM01"}},
		unpublished: []Event{{EventID: "evt-2", EventType: EventTypeControlAccepted, RoomID: "ROOM01"}},
	}
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", store, store, store)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})
	if err := client.RecordTimelineEvent(context.Background(), Event{EventID: "evt-record", RoomID: "ROOM01"}); err != nil {
		t.Fatalf("record through rpc: %v", err)
	}
	if store.recorded.EventID != "evt-record" {
		t.Fatalf("expected recorded event, got %+v", store.recorded)
	}
	controlEvent, err := client.RecordControlResult(context.Background(), ControlResult{
		RoomID:      "ROOM01",
		UserID:      "user-a",
		DeviceID:    "device-a",
		InstanceID:  "instance-a",
		ControlType: "play",
		Seq:         2,
		Accepted:    true,
		Payload:     map[string]any{"type": "play"},
	})
	if err != nil {
		t.Fatalf("record control result through rpc: %v", err)
	}
	if controlEvent.EventID == "" ||
		controlEvent.EventType != EventTypeControlAccepted ||
		controlEvent.RoomID != "ROOM01" ||
		controlEvent.ControlType != "play" ||
		controlEvent.Seq != 2 {
		t.Fatalf("unexpected control event: %+v", controlEvent)
	}
	membershipEvent, err := client.RecordMembershipResult(context.Background(), MembershipResult{
		RoomID:         "ROOM01",
		UserID:         "user-a",
		DeviceID:       "device-a",
		InstanceID:     "instance-a",
		MembershipType: MembershipResultJoined,
		Payload:        map[string]any{"reason": "join"},
	})
	if err != nil {
		t.Fatalf("record membership result through rpc: %v", err)
	}
	if membershipEvent.EventID == "" || membershipEvent.EventType != EventTypeMemberJoined || membershipEvent.RoomID != "ROOM01" {
		t.Fatalf("unexpected membership event: %+v", membershipEvent)
	}
	events, err := client.ReadRoomEvents(context.Background(), "ROOM01")
	if err != nil {
		t.Fatalf("read events through rpc: %v", err)
	}
	if len(events) != 1 || events[0].EventID != "evt-1" {
		t.Fatalf("unexpected room events: %+v", events)
	}
	unpublished, err := client.ReadRoomUnpublishedTimelineEvents(context.Background(), "ROOM01")
	if err != nil {
		t.Fatalf("read unpublished through rpc: %v", err)
	}
	if len(unpublished) != 1 || unpublished[0].EventID != "evt-2" {
		t.Fatalf("unexpected unpublished events: %+v", unpublished)
	}
	recoveryEvents, err := client.ReadRoomRecoveryEvents(context.Background(), "ROOM01")
	if err != nil {
		t.Fatalf("read recovery events through rpc: %v", err)
	}
	if len(recoveryEvents) != 2 || recoveryEvents[0].EventID != "evt-1" || recoveryEvents[1].EventID != "evt-2" {
		t.Fatalf("unexpected recovery events: %+v", recoveryEvents)
	}
}

func TestRPCClientMapsUnavailableReader(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "", nil, nil, nil)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
	_, err := client.ReadRoomEvents(context.Background(), "ROOM01")
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnavailable {
		t.Fatalf("expected unavailable connect error, got %v", err)
	}
}

func TestRPCClientMapsTypedTimelineErrors(t *testing.T) {
	t.Run("invalid argument", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterInternalRPC(mux, "", "", &fakeRPCTimelineStore{}, nil, nil)
		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
		_, err := client.RecordControlResult(context.Background(), ControlResult{ControlType: "play", Accepted: true})
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeInvalidArgument {
			t.Fatalf("expected invalid argument connect error, got %v", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterInternalRPC(mux, "", "secret", &fakeRPCTimelineStore{}, nil, nil)
		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second, AuthToken: "wrong"})
		_, err := client.RecordControlResult(context.Background(), ControlResult{
			RoomID:      "ROOM01",
			ControlType: "play",
			Accepted:    true,
		})
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
			t.Fatalf("expected unauthenticated connect error, got %v", err)
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterInternalRPC(mux, "", "", nil, nil, nil)
		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
		_, err := client.RecordControlResult(context.Background(), ControlResult{
			RoomID:      "ROOM01",
			ControlType: "play",
			Accepted:    true,
		})
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnavailable {
			t.Fatalf("expected unavailable connect error, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mux := http.NewServeMux()
		RegisterInternalRPC(mux, "", "", blockingRPCTimelineStore{}, nil, nil)
		server := httptest.NewServer(mux)
		defer server.Close()

		client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Nanosecond})
		_, err := client.RecordControlResult(context.Background(), ControlResult{
			RoomID:      "ROOM01",
			ControlType: "play",
			Accepted:    true,
		})
		var connectErr *connect.Error
		if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeDeadlineExceeded {
			t.Fatalf("expected deadline exceeded connect error, got %v", err)
		}
	})
}

type fakeRPCTimelineStore struct {
	recorded    Event
	roomEvents  []Event
	unpublished []Event
}

func (s *fakeRPCTimelineStore) RecordTimelineEvent(ctx context.Context, event Event) error {
	_ = ctx
	s.recorded = event
	return nil
}

func (s *fakeRPCTimelineStore) ReadRoomEvents(ctx context.Context, roomID string) ([]Event, error) {
	_ = ctx
	_ = roomID
	return s.roomEvents, nil
}

func (s *fakeRPCTimelineStore) ReadRoomUnpublishedTimelineEvents(ctx context.Context, roomID string) ([]Event, error) {
	_ = ctx
	_ = roomID
	return s.unpublished, nil
}

type blockingRPCTimelineStore struct{}

func (blockingRPCTimelineStore) RecordTimelineEvent(ctx context.Context, event Event) error {
	_ = event
	<-ctx.Done()
	return ctx.Err()
}
