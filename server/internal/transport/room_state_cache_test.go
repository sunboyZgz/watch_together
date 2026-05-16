package transport

import (
	"context"
	"testing"

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

	if len(stateCache.calls) != 1 {
		t.Fatalf("expected 1 cache write, got %d", len(stateCache.calls))
	}
	if stateCache.calls[0].roomID != "ROOM01" {
		t.Fatalf("expected room id ROOM01, got %s", stateCache.calls[0].roomID)
	}
	if stateCache.calls[0].state.Seq != 3 {
		t.Fatalf("expected cached seq 3, got %d", stateCache.calls[0].state.Seq)
	}
	if stateCache.calls[0].state.PositionMs != 12_000 {
		t.Fatalf("expected cached position 12000, got %d", stateCache.calls[0].state.PositionMs)
	}
}

type fakeRoomStateCache struct {
	calls []fakeRoomStateCacheCall
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
	c.calls = append(c.calls, fakeRoomStateCacheCall{
		roomID: roomID,
		state:  state,
	})
	return nil
}
