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

	_, found, err := cache.GetRoomState(context.Background(), "ROOM01")
	if !errors.Is(err, ErrRedisDisabled) {
		t.Fatalf("expected redis disabled, got %v", err)
	}
	if found {
		t.Fatalf("expected disabled cache not to report a hit")
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

func TestRoomStateCacheGetMissUsesJSONStore(t *testing.T) {
	store := &fakeJSONStore{}
	cache := NewRoomStateCache(store, 0)

	state, found, err := cache.GetRoomState(context.Background(), "ROOM01")
	if err != nil {
		t.Fatalf("get room state: %v", err)
	}
	if found {
		t.Fatalf("expected cache miss")
	}
	if state.RoomID != "" || state.Seq != 0 {
		t.Fatalf("expected zero state on miss, got %+v", state)
	}
	if store.getKey != "wt:room:state:ROOM01:v1" {
		t.Fatalf("unexpected get key %q", store.getKey)
	}
}

func TestRoomStateCacheGetReturnsStoredPayload(t *testing.T) {
	stored := protocolRoomStateForTest()
	stored.MediaID = "media_001"
	stored.HostUserID = "user_a"
	stored.PositionMs = 12_000
	stored.PlaybackRate = 1.25
	stored.Seq = 9
	store := &fakeJSONStore{
		getFound: true,
		getValue: stored,
	}
	cache := NewRoomStateCache(store, 0)

	state, found, err := cache.GetRoomState(context.Background(), "ROOM01")
	if err != nil {
		t.Fatalf("get room state: %v", err)
	}
	if !found {
		t.Fatalf("expected cache hit")
	}
	if store.getKey != "wt:room:state:ROOM01:v1" {
		t.Fatalf("unexpected get key %q", store.getKey)
	}
	if state.RoomID != stored.RoomID ||
		state.MediaID != stored.MediaID ||
		state.HostUserID != stored.HostUserID ||
		state.PositionMs != stored.PositionMs ||
		state.PlaybackRate != stored.PlaybackRate ||
		state.Seq != stored.Seq {
		t.Fatalf("unexpected cached state: %+v", state)
	}
}

type fakeJSONStore struct {
	setKey   string
	setValue any
	setTTL   time.Duration
	getKey   string
	getFound bool
	getValue protocol.RoomStatePayload
}

func (s *fakeJSONStore) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	s.getKey = key
	if !s.getFound {
		return false, nil
	}
	state, ok := dest.(*protocol.RoomStatePayload)
	if !ok {
		return false, errors.New("unexpected room state cache destination")
	}
	*state = s.getValue
	return true, nil
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
