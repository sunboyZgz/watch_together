package cache

import (
	"context"
	"time"

	"watch_together/server/internal/protocol"
)

const (
	// RoomStateRedisDB is the explicit Redis database for the latest room_state cache.
	RoomStateRedisDB    = 0
	defaultRoomStateTTL = 10 * time.Minute
)

type RoomStateCache struct {
	store JSONStore
	ttl   time.Duration
}

type JSONStore interface {
	GetJSON(ctx context.Context, key string, dest any) (bool, error)
	SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

func RoomStateRedisConfig(config RedisConfig) RedisConfig {
	config.DB = RoomStateRedisDB
	return config
}

func NewRoomStateCache(store JSONStore, ttl time.Duration) *RoomStateCache {
	if ttl <= 0 {
		ttl = defaultRoomStateTTL
	}
	return &RoomStateCache{
		store: store,
		ttl:   ttl,
	}
}

func (c *RoomStateCache) SetRoomState(
	ctx context.Context,
	roomID string,
	state protocol.RoomStatePayload,
) error {
	if c == nil || c.store == nil {
		return ErrRedisDisabled
	}
	return c.store.SetJSON(ctx, roomStateKey(roomID), state, c.ttl)
}

func (c *RoomStateCache) GetRoomState(
	ctx context.Context,
	roomID string,
) (protocol.RoomStatePayload, bool, error) {
	if c == nil || c.store == nil {
		return protocol.RoomStatePayload{}, false, ErrRedisDisabled
	}
	var state protocol.RoomStatePayload
	found, err := c.store.GetJSON(ctx, roomStateKey(roomID), &state)
	return state, found, err
}

func (c *RoomStateCache) DeleteRoomState(ctx context.Context, roomID string) error {
	if c == nil || c.store == nil {
		return ErrRedisDisabled
	}
	return c.store.Delete(ctx, roomStateKey(roomID))
}

func roomStateKey(roomID string) string {
	return "wt:room:state:" + roomID + ":v1"
}
