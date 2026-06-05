package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestWebSocketCrossInstanceBroadcastDeliversRemoteControlEnvelope(t *testing.T) {
	bus := eventbus.NewMemoryRoomBroadcastBus()
	defer bus.Close()

	roomManagerA := room.NewManager()
	roomManagerB := room.NewManager()
	roomA, err := roomManagerA.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomManagerB.RegisterCreatedRoom(roomA.ID(), "user_a", "sample_001")

	serverA, _ := newCrossInstanceWebSocketServer(t, roomManagerA, "instance-a", bus)
	defer serverA.Close()
	serverB, _ := newCrossInstanceWebSocketServer(t, roomManagerB, "instance-b", bus)
	defer serverB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hostA := mustDialWebSocket(t, ctx, wsURL(serverA.URL), "user_a")
	defer hostA.Close(websocket.StatusNormalClosure, "test done")
	viewerB := mustDialWebSocket(t, ctx, wsURL(serverB.URL), "user_b")
	defer viewerB.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostA, roomA.ID(), "user_a")
	hostState := mustReadEnvelope(t, ctx, hostA)
	if hostState.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", hostState.Type)
	}
	mustJoinRoom(t, ctx, viewerB, roomA.ID(), "user_b")
	viewerState := mustReadEnvelope(t, ctx, viewerB)
	if viewerState.Type != protocol.TypeRoomState {
		t.Fatalf("expected viewer room_state, got %s", viewerState.Type)
	}

	mustSendEnvelope(t, ctx, hostA, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     roomA.ID(),
			UserID:     "user_a",
			RequestID:  "play-cross-instance",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, hostA, protocol.TypePlay, -1, 2)
	assertControlBroadcast(t, ctx, viewerB, protocol.TypePlay, -1, 2)
	stateA := roomA.StateSnapshot()
	if stateA.Seq != 2 || stateA.Paused {
		t.Fatalf("expected source authority state to keep accepted play, got seq=%d paused=%t", stateA.Seq, stateA.Paused)
	}
	stateB := roomManagerB.GetOrCreate(roomA.ID()).StateSnapshot()
	if stateB.Seq != 1 || !stateB.Paused {
		t.Fatalf("expected remote broadcast not to mutate local authority, got seq=%d paused=%t", stateB.Seq, stateB.Paused)
	}
	assertNoEnvelopeWithin(t, hostA, 100*time.Millisecond)
}

func TestWebSocketCrossInstanceBroadcastDropsOwnInstanceEvent(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	client := room.NewClientConnection(nil)
	client.SetIdentity("user_b", createdRoom.ID())
	createdRoom.Join(client)

	handler := NewWebSocketHandler(roomManager, true)
	recorder := &recordingRoomBroadcaster{}
	handler.broadcaster = recorder
	handler.SetRoomBroadcastBus("instance-a", eventbus.NewDisabledRoomBroadcastBus())

	handler.handleRemoteRoomBroadcast(context.Background(), eventbus.RoomBroadcastEvent{
		InstanceID: "instance-a",
		RoomID:     createdRoom.ID(),
		Type:       protocol.TypePlay,
		Payload:    mustJSONRaw(protocol.PlayPayload{RoomID: createdRoom.ID(), UserID: "user_a", Seq: 2}),
		Seq:        2,
	})

	if recorder.calls != 0 {
		t.Fatalf("expected own instance event to be dropped, got %d broadcasts", recorder.calls)
	}
}

func TestWebSocketCrossInstancePublishFailureDoesNotFailLocalBroadcast(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	failBus := &failingRoomBroadcastBus{err: errors.New("publish unavailable")}
	server, handler := newCrossInstanceWebSocketServer(t, roomManager, "instance-a", failBus)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	host := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_a")
	defer host.Close(websocket.StatusNormalClosure, "test done")
	viewer := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_b")
	defer viewer.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, host, createdRoom.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}
	mustJoinRoom(t, ctx, viewer, createdRoom.ID(), "user_b")
	if envelope := mustReadEnvelope(t, ctx, viewer); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected viewer room_state, got %s", envelope.Type)
	}
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomMembersChanged {
		t.Fatalf("expected host room_members_changed, got %s", envelope.Type)
	}

	mustSendEnvelope(t, ctx, host, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "play-publish-fails",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, host, protocol.TypePlay, -1, 2)
	assertControlBroadcast(t, ctx, viewer, protocol.TypePlay, -1, 2)
	if handler == nil || failBus.publishCount == 0 {
		t.Fatalf("expected handler to attempt bus publish")
	}
}

func TestWebSocketCrossInstanceBroadcastPublishesMembershipEvent(t *testing.T) {
	bus := &recordingRoomBroadcastBus{}
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	server, _ := newCrossInstanceWebSocketServer(t, roomManager, "instance-a", bus)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	host := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_a")
	defer host.Close(websocket.StatusNormalClosure, "test done")
	viewer := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_b")
	defer viewer.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, host, createdRoom.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}
	mustJoinRoom(t, ctx, viewer, createdRoom.ID(), "user_b")
	if envelope := mustReadEnvelope(t, ctx, viewer); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected viewer room_state, got %s", envelope.Type)
	}
	_ = mustReadEnvelope(t, ctx, host)

	published := bus.events()
	if len(published) != 1 {
		t.Fatalf("expected one membership broadcast publish, got %d", len(published))
	}
	if published[0].InstanceID != "instance-a" ||
		published[0].RoomID != createdRoom.ID() ||
		published[0].Type != protocol.TypeRoomMembersChanged {
		t.Fatalf("unexpected published event: %+v", published[0])
	}
}

func newCrossInstanceWebSocketServer(
	t *testing.T,
	roomManager *room.Manager,
	instanceID string,
	bus eventbus.RoomBroadcastBus,
) (*httptest.Server, *WebSocketHandler) {
	t.Helper()
	handler := NewWebSocketHandler(roomManager, true)
	handler.SetRoomBroadcastBus(instanceID, bus)
	if err := handler.SubscribeRoomBroadcasts(context.Background()); err != nil {
		t.Fatalf("subscribe broadcasts: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", handler)
	server := httptest.NewServer(mux)
	return server, handler
}

func assertNoEnvelopeWithin(t *testing.T, conn *websocket.Conn, wait time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()

	_, data, err := conn.Read(ctx)
	if err == nil {
		var envelope protocol.Envelope
		if decodeErr := json.Unmarshal(data, &envelope); decodeErr == nil {
			t.Fatalf("expected no envelope within %s, got %s", wait, envelope.Type)
		}
		t.Fatalf("expected no envelope within %s, got raw message %q", wait, string(data))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline while waiting for no envelope, got %v", err)
	}
}

type recordingRoomBroadcaster struct {
	calls     int
	envelopes []protocol.Envelope
}

func (b *recordingRoomBroadcaster) Broadcast(ctx context.Context, clients []clientWriter, envelope protocol.Envelope) (broadcastStats, error) {
	b.calls++
	b.envelopes = append(b.envelopes, envelope)
	return broadcastStats{Clients: len(clients)}, nil
}

type failingRoomBroadcastBus struct {
	err          error
	publishCount int
}

func (b *failingRoomBroadcastBus) PublishRoomEnvelope(ctx context.Context, event eventbus.RoomBroadcastEvent) error {
	b.publishCount++
	return b.err
}

func (b *failingRoomBroadcastBus) SubscribeRoomBroadcasts(ctx context.Context, handler eventbus.RoomBroadcastHandler) error {
	return nil
}

func (b *failingRoomBroadcastBus) Close() error {
	return nil
}

type recordingRoomBroadcastBus struct {
	mu        sync.Mutex
	published []eventbus.RoomBroadcastEvent
	handler   eventbus.RoomBroadcastHandler
}

func (b *recordingRoomBroadcastBus) PublishRoomEnvelope(ctx context.Context, event eventbus.RoomBroadcastEvent) error {
	b.mu.Lock()
	b.published = append(b.published, event)
	handler := b.handler
	b.mu.Unlock()
	if handler != nil {
		handler(ctx, event)
	}
	return nil
}

func (b *recordingRoomBroadcastBus) SubscribeRoomBroadcasts(ctx context.Context, handler eventbus.RoomBroadcastHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handler = handler
	return nil
}

func (b *recordingRoomBroadcastBus) Close() error {
	return nil
}

func (b *recordingRoomBroadcastBus) events() []eventbus.RoomBroadcastEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]eventbus.RoomBroadcastEvent(nil), b.published...)
}
