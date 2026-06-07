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
