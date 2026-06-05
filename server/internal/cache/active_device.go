package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultActiveDeviceLeaseTTL = 45 * time.Second

type ActiveDeviceLease struct {
	DeviceID     string `json:"deviceId"`
	InstanceID   string `json:"instanceId"`
	ConnectionID string `json:"connectionId"`
	LeaseUntilMs int64  `json:"leaseUntilMs"`
}

type ActiveDeviceRegistry struct {
	client *redis.Client
	ttl    time.Duration
	now    func() time.Time
}

func NewActiveDeviceRegistry(redisClient *RedisClient, ttl time.Duration) *ActiveDeviceRegistry {
	return NewActiveDeviceRegistryWithClock(redisClient, ttl, time.Now)
}

func NewActiveDeviceRegistryWithClock(
	redisClient *RedisClient,
	ttl time.Duration,
	now func() time.Time,
) *ActiveDeviceRegistry {
	if ttl <= 0 {
		ttl = defaultActiveDeviceLeaseTTL
	}
	if now == nil {
		now = time.Now
	}
	return &ActiveDeviceRegistry{
		client: redisClient.Raw(),
		ttl:    ttl,
		now:    now,
	}
}

func (r *ActiveDeviceRegistry) Acquire(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	instanceID string,
	connectionID string,
) (ActiveDeviceLease, bool, error) {
	if r == nil || r.client == nil {
		return ActiveDeviceLease{}, false, ErrRedisDisabled
	}
	if roomID == "" || userID == "" || deviceID == "" || instanceID == "" || connectionID == "" {
		return ActiveDeviceLease{}, false, errors.New("roomID, userID, deviceID, instanceID, and connectionID are required")
	}
	key := activeDeviceKey(roomID, userID)
	next := ActiveDeviceLease{
		DeviceID:     deviceID,
		InstanceID:   instanceID,
		ConnectionID: connectionID,
		LeaseUntilMs: r.now().Add(r.ttl).UnixMilli(),
	}
	data, err := json.Marshal(next)
	if err != nil {
		return ActiveDeviceLease{}, false, err
	}

	var current ActiveDeviceLease
	err = r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode active device lease: %w", decodeErr)
			}
			if current.DeviceID != deviceID {
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
		return ActiveDeviceLease{}, false, err
	}
	return current, current.DeviceID == deviceID, nil
}

func (r *ActiveDeviceRegistry) Get(
	ctx context.Context,
	roomID string,
	userID string,
) (ActiveDeviceLease, bool, error) {
	if r == nil || r.client == nil {
		return ActiveDeviceLease{}, false, ErrRedisDisabled
	}
	if roomID == "" || userID == "" {
		return ActiveDeviceLease{}, false, errors.New("roomID and userID are required")
	}
	value, err := r.client.Get(ctx, activeDeviceKey(roomID, userID)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ActiveDeviceLease{}, false, nil
		}
		return ActiveDeviceLease{}, false, err
	}
	var lease ActiveDeviceLease
	if err := json.Unmarshal(value, &lease); err != nil {
		return ActiveDeviceLease{}, false, fmt.Errorf("decode active device lease: %w", err)
	}
	return lease, true, nil
}

func (r *ActiveDeviceRegistry) ReleaseIfMatch(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	connectionID string,
) (bool, error) {
	if r == nil || r.client == nil {
		return false, ErrRedisDisabled
	}
	if roomID == "" || userID == "" || deviceID == "" || connectionID == "" {
		return false, errors.New("roomID, userID, deviceID, and connectionID are required")
	}
	key := activeDeviceKey(roomID, userID)
	released := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		var current ActiveDeviceLease
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode active device lease: %w", err)
		}
		if current.DeviceID != deviceID || current.ConnectionID != connectionID {
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

func ActiveDeviceKey(roomID string, userID string) string {
	return activeDeviceKey(roomID, userID)
}

func activeDeviceKey(roomID string, userID string) string {
	return "wt:room:active_device:" + roomID + ":" + userID + ":v1"
}
