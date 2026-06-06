package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultPresenceLeaseTTL = 45 * time.Second

type PresenceMember struct {
	RoomID       string `json:"roomId"`
	UserID       string `json:"userId"`
	Role         string `json:"role,omitempty"`
	DeviceID     string `json:"deviceId"`
	InstanceID   string `json:"instanceId"`
	ConnectionID string `json:"connectionId"`
	IsHost       bool   `json:"isHost"`
	LastSeenMs   int64  `json:"lastSeenMs"`
	LeaseUntilMs int64  `json:"leaseUntilMs"`
}

type PresenceSnapshot struct {
	RoomID      string
	OnlineCount int
	Members     []PresenceMember
}

type PresenceRegistry struct {
	client *redis.Client
	ttl    time.Duration
	now    func() time.Time
}

func NewPresenceRegistry(redisClient *RedisClient, ttl time.Duration) *PresenceRegistry {
	return NewPresenceRegistryWithClock(redisClient, ttl, time.Now)
}

func NewPresenceRegistryWithClock(
	redisClient *RedisClient,
	ttl time.Duration,
	now func() time.Time,
) *PresenceRegistry {
	if ttl <= 0 {
		ttl = defaultPresenceLeaseTTL
	}
	if now == nil {
		now = time.Now
	}
	return &PresenceRegistry{
		client: redisClient.Raw(),
		ttl:    ttl,
		now:    now,
	}
}

func (r *PresenceRegistry) Upsert(
	ctx context.Context,
	roomID string,
	userID string,
	role string,
	deviceID string,
	instanceID string,
	connectionID string,
	isHost bool,
) (PresenceMember, bool, error) {
	if r == nil || r.client == nil {
		return PresenceMember{}, false, ErrRedisDisabled
	}
	if roomID == "" || userID == "" || deviceID == "" || instanceID == "" || connectionID == "" {
		return PresenceMember{}, false, errors.New("roomID, userID, deviceID, instanceID, and connectionID are required")
	}
	key := PresenceKey(roomID)
	now := r.now()
	next := PresenceMember{
		RoomID:       roomID,
		UserID:       userID,
		Role:         role,
		DeviceID:     deviceID,
		InstanceID:   instanceID,
		ConnectionID: connectionID,
		IsHost:       isHost,
		LastSeenMs:   now.UnixMilli(),
		LeaseUntilMs: now.Add(r.ttl).UnixMilli(),
	}
	data, err := json.Marshal(next)
	if err != nil {
		return PresenceMember{}, false, err
	}

	var current PresenceMember
	acquired := false
	err = r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.HGet(ctx, key, userID).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode presence member: %w", decodeErr)
			}
			if current.DeviceID != deviceID && current.LeaseUntilMs > now.UnixMilli() {
				return nil
			}
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, key, userID, data)
			pipe.Expire(ctx, key, r.ttl)
			return nil
		})
		if pipeErr == nil {
			current = next
			acquired = true
		}
		return pipeErr
	}, key)
	if err != nil {
		return PresenceMember{}, false, err
	}
	return current, acquired, nil
}

func (r *PresenceRegistry) ReleaseIfMatch(
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
	key := PresenceKey(roomID)
	released := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.HGet(ctx, key, userID).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		var current PresenceMember
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode presence member: %w", err)
		}
		if current.DeviceID != deviceID || current.ConnectionID != connectionID {
			return nil
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HDel(ctx, key, userID)
			return nil
		})
		if err == nil {
			released = true
		}
		return err
	}, key)
	return released, err
}

func (r *PresenceRegistry) Snapshot(ctx context.Context, roomID string) (PresenceSnapshot, error) {
	if r == nil || r.client == nil {
		return PresenceSnapshot{}, ErrRedisDisabled
	}
	if roomID == "" {
		return PresenceSnapshot{}, errors.New("roomID is required")
	}
	key := PresenceKey(roomID)
	values, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return PresenceSnapshot{}, err
	}
	nowMs := r.now().UnixMilli()
	members := make([]PresenceMember, 0, len(values))
	expired := make([]string, 0)
	for userID, raw := range values {
		var member PresenceMember
		if err := json.Unmarshal([]byte(raw), &member); err != nil {
			expired = append(expired, userID)
			continue
		}
		if member.LeaseUntilMs <= nowMs {
			expired = append(expired, userID)
			continue
		}
		members = append(members, member)
	}
	if len(expired) > 0 {
		_ = r.client.HDel(ctx, key, expired...).Err()
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].IsHost != members[j].IsHost {
			return members[i].IsHost
		}
		return members[i].UserID < members[j].UserID
	})
	return PresenceSnapshot{
		RoomID:      roomID,
		OnlineCount: len(members),
		Members:     members,
	}, nil
}

func PresenceKey(roomID string) string {
	return "wt:room:presence:" + roomID + ":v1"
}
