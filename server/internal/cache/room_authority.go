package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultRoomAuthorityLeaseTTL = 30 * time.Second
)

type RoomAuthorityLease struct {
	InstanceID   string `json:"instanceId"`
	LeaseUntilMs int64  `json:"leaseUntilMs"`
}

type RoomAuthorityRegistry struct {
	client *redis.Client
	ttl    time.Duration
	now    func() time.Time
}

func NewRoomAuthorityRegistry(redisClient *RedisClient, ttl time.Duration) *RoomAuthorityRegistry {
	return NewRoomAuthorityRegistryWithClock(redisClient, ttl, time.Now)
}

func NewRoomAuthorityRegistryWithClock(
	redisClient *RedisClient,
	ttl time.Duration,
	now func() time.Time,
) *RoomAuthorityRegistry {
	if ttl <= 0 {
		ttl = defaultRoomAuthorityLeaseTTL
	}
	if now == nil {
		now = time.Now
	}
	return &RoomAuthorityRegistry{
		client: redisClient.Raw(),
		ttl:    ttl,
		now:    now,
	}
}

func (r *RoomAuthorityRegistry) ClaimAuthority(
	ctx context.Context,
	roomID string,
	instanceID string,
) (RoomAuthorityLease, bool, error) {
	if r == nil || r.client == nil {
		return RoomAuthorityLease{}, false, ErrRedisDisabled
	}
	if roomID == "" || instanceID == "" {
		return RoomAuthorityLease{}, false, errors.New("roomID and instanceID are required")
	}
	key := roomAuthorityKey(roomID)
	next := RoomAuthorityLease{
		InstanceID:   instanceID,
		LeaseUntilMs: r.now().Add(r.ttl).UnixMilli(),
	}
	data, err := json.Marshal(next)
	if err != nil {
		return RoomAuthorityLease{}, false, err
	}

	var current RoomAuthorityLease
	err = r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode room authority lease: %w", decodeErr)
			}
			if current.InstanceID != instanceID {
				return nil
			}
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, r.ttl)
			return nil
		})
		if pipeErr == nil {
			current = next
		}
		return pipeErr
	}, key)
	if err != nil {
		return RoomAuthorityLease{}, false, err
	}
	return current, current.InstanceID == instanceID, nil
}

func (r *RoomAuthorityRegistry) RefreshAuthority(
	ctx context.Context,
	roomID string,
	instanceID string,
) (RoomAuthorityLease, bool, error) {
	return r.ClaimAuthority(ctx, roomID, instanceID)
}

func (r *RoomAuthorityRegistry) GetAuthority(
	ctx context.Context,
	roomID string,
) (RoomAuthorityLease, bool, error) {
	if r == nil || r.client == nil {
		return RoomAuthorityLease{}, false, ErrRedisDisabled
	}
	if roomID == "" {
		return RoomAuthorityLease{}, false, errors.New("roomID is required")
	}
	value, err := r.client.Get(ctx, roomAuthorityKey(roomID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return RoomAuthorityLease{}, false, nil
		}
		return RoomAuthorityLease{}, false, err
	}
	var lease RoomAuthorityLease
	if err := json.Unmarshal(value, &lease); err != nil {
		return RoomAuthorityLease{}, false, fmt.Errorf("decode room authority lease: %w", err)
	}
	return lease, true, nil
}

func (r *RoomAuthorityRegistry) ReleaseAuthority(
	ctx context.Context,
	roomID string,
	instanceID string,
) (bool, error) {
	if r == nil || r.client == nil {
		return false, ErrRedisDisabled
	}
	if roomID == "" || instanceID == "" {
		return false, errors.New("roomID and instanceID are required")
	}
	key := roomAuthorityKey(roomID)
	released := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		var current RoomAuthorityLease
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode room authority lease: %w", err)
		}
		if current.InstanceID != instanceID {
			return nil
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		if err == nil {
			released = true
		}
		return err
	}, key)
	return released, err
}

func RoomAuthorityKey(roomID string) string {
	return roomAuthorityKey(roomID)
}

func roomAuthorityKey(roomID string) string {
	return "wt:room:authority:" + roomID + ":v1"
}
