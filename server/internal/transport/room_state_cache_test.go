package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestWebSocketHandlerCachesRoomStateSnapshot(t *testing.T) {
	stateCache := &fakeRoomStateCache{}
	handler := NewWebSocketHandlerWithConfigAndRoomStateWriter(room.NewManager(), false, WebSocketRuntimeConfig{}, stateCache)
	state := room.State{
		RoomID:       "ROOM01",
		MediaID:      "sample_001",
		HostUserID:   "user_a",
		Paused:       true,
		PositionMs:   12_000,
		Velocity:     0,
		ServerTimeMs: 1_000,
		PlaybackRate: 1,
		Seq:          3,
		Reason:       "pause",
	}

	handler.cacheRoomState(state)

	if stateCache.callCount() != 1 {
		t.Fatalf("expected 1 cache write, got %d", stateCache.callCount())
	}
	firstCall := stateCache.callAt(t, 0)
	if firstCall.roomID != "ROOM01" {
		t.Fatalf("expected room id ROOM01, got %s", firstCall.roomID)
	}
	if firstCall.state.Seq != 3 {
		t.Fatalf("expected cached seq 3, got %d", firstCall.state.Seq)
	}
	if firstCall.state.PositionMs != 12_000 {
		t.Fatalf("expected cached position 12000, got %d", firstCall.state.PositionMs)
	}
}

func TestWebSocketHandlerCachesRoomStateAfterControlSuccess(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	stateCache := &fakeRoomStateCache{}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandlerWithConfigAndRoomStateWriter(roomManager, false, WebSocketRuntimeConfig{}, stateCache))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	ctx := context.Background()
	hostConn := mustDialWebSocket(t, ctx, wsURL(httpServer.URL))
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 12_000,
			Seq:        1,
		}),
	})
	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, -1, 2)

	cached := stateCache.lastCall(t)
	if cached.roomID != createdRoom.ID() {
		t.Fatalf("expected cached room %s, got %s", createdRoom.ID(), cached.roomID)
	}
	if cached.state.Seq != 2 {
		t.Fatalf("expected cached seq 2, got %d", cached.state.Seq)
	}
	if cached.state.Paused {
		t.Fatalf("expected cached state to reflect playing")
	}
}

func TestWebSocketHandlerCachesRoomStateRequestSnapshot(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	stateCache := &fakeRoomStateCache{}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandlerWithConfigAndRoomStateWriter(roomManager, false, WebSocketRuntimeConfig{}, stateCache))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	ctx := context.Background()
	hostConn := mustDialWebSocket(t, ctx, wsURL(httpServer.URL))
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeRoomStateRequest,
		Payload: mustJSONRaw(protocol.RoomStateRequestPayload{
			RoomID: createdRoom.ID(),
			UserID: "user_a",
			Seq:    1,
		}),
	})
	response := mustReadEnvelope(t, ctx, hostConn)
	if response.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state response, got %s", response.Type)
	}

	cached := stateCache.lastCall(t)
	if cached.roomID != createdRoom.ID() {
		t.Fatalf("expected cached room %s, got %s", createdRoom.ID(), cached.roomID)
	}
	if cached.state.Seq != 1 {
		t.Fatalf("expected cached seq 1, got %d", cached.state.Seq)
	}
}

func TestWebSocketHandlerContinuesWhenRoomStateCacheWriteFails(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	stateCache := &fakeRoomStateCache{err: errors.New("cache unavailable")}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandlerWithConfigAndRoomStateWriter(roomManager, false, WebSocketRuntimeConfig{}, stateCache))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	ctx := context.Background()
	hostConn := mustDialWebSocket(t, ctx, wsURL(httpServer.URL))
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	initial := mustReadEnvelope(t, ctx, hostConn)
	if initial.Type != protocol.TypeRoomState {
		t.Fatalf("expected initial room_state, got %s", initial.Type)
	}

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 12_000,
			Seq:        1,
		}),
	})
	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, -1, 2)
	if stateCache.callCount() == 0 {
		t.Fatalf("expected attempted cache writes")
	}
}

type fakeRoomStateCache struct {
	mu    sync.Mutex
	calls []fakeRoomStateCacheCall
	err   error
}

type fakeRoomStateCacheCall struct {
	roomID string
	state  protocol.RoomStatePayload
}

func (c *fakeRoomStateCache) SetRoomState(
	ctx context.Context,
	roomID string,
	state protocol.RoomStatePayload,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, fakeRoomStateCacheCall{
		roomID: roomID,
		state:  state,
	})
	return c.err
}

func (c *fakeRoomStateCache) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func (c *fakeRoomStateCache) callAt(t *testing.T, index int) fakeRoomStateCacheCall {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.calls) {
		t.Fatalf("expected cache call at index %d, got %d calls", index, len(c.calls))
	}
	return c.calls[index]
}

func (c *fakeRoomStateCache) lastCall(t *testing.T) fakeRoomStateCacheCall {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		t.Fatalf("expected at least one cache write")
	}
	return c.calls[len(c.calls)-1]
}
