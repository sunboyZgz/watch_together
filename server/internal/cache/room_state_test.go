package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"watch_together/server/internal/protocol"
)

func TestRoomStateCacheDisabledWithoutRedis(t *testing.T) {
	cache := NewRoomStateCache(nil, 0)

	err := cache.SetRoomState(context.Background(), "ROOM01", protocolRoomStateForTest())
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected redis disabled, got %v", err)
	}
}

func TestRoomStateKey(t *testing.T) {
	if key := roomStateKey("ROOM01"); key != "wt:room:state:ROOM01:v1" {
		t.Fatalf("unexpected room state key %q", key)
	}
}

func TestRoomStateCacheSetUsesJSONStore(t *testing.T) {
	store := &fakeJSONStore{}
	cache := NewRoomStateCache(store, 0)
	state := protocolRoomStateForTest()

	if err := cache.SetRoomState(context.Background(), "ROOM01", state); err != nil {
		t.Fatalf("set room state: %v", err)
	}

	if store.setKey != "wt:room:state:ROOM01:v1" {
		t.Fatalf("unexpected set key %q", store.setKey)
	}
	if store.setTTL != defaultRoomStateTTL {
		t.Fatalf("expected default ttl %s, got %s", defaultRoomStateTTL, store.setTTL)
	}
	cached, ok := store.setValue.(protocol.RoomStatePayload)
	if !ok {
		t.Fatalf("expected protocol.RoomStatePayload value, got %T", store.setValue)
	}
	if cached.Seq != state.Seq {
		t.Fatalf("expected cached seq %d, got %d", state.Seq, cached.Seq)
	}
}

type fakeJSONStore struct {
	setKey   string
	setValue any
	setTTL   time.Duration
}

func (s *fakeJSONStore) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	return false, nil
}

func (s *fakeJSONStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	s.setKey = key
	s.setValue = value
	s.setTTL = ttl
	return nil
}

func (s *fakeJSONStore) Delete(ctx context.Context, keys ...string) error {
	return nil
}

func protocolRoomStateForTest() protocol.RoomStatePayload {
	return protocol.RoomStatePayload{
		RoomID: "ROOM01",
		Seq:    1,
	}
}
