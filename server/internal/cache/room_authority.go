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

	RoomAuthorityStatusActive     = "active"
	RoomAuthorityStatusRecovering = "recovering"
)

type RoomAuthorityLease struct {
	InstanceID   string `json:"instanceId"`
	Epoch        int64  `json:"epoch"`
	Status       string `json:"status"`
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
	var current RoomAuthorityLease
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode room authority lease: %w", decodeErr)
			}
			current = normalizeRoomAuthorityLease(current)
			if current.InstanceID != instanceID {
				return nil
			}
		}
		next := current
		if next.InstanceID == "" {
			next = RoomAuthorityLease{
				InstanceID: instanceID,
				Epoch:      1,
				Status:     RoomAuthorityStatusActive,
			}
		}
		next.InstanceID = instanceID
		next = normalizeRoomAuthorityLease(next)
		next.LeaseUntilMs = r.now().Add(r.ttl).UnixMilli()
		data, marshalErr := json.Marshal(next)
		if marshalErr != nil {
			return marshalErr
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
	return r.RenewAuthority(ctx, roomID, instanceID)
}

func (r *RoomAuthorityRegistry) RenewAuthority(
	ctx context.Context,
	roomID string,
	instanceID string,
) (RoomAuthorityLease, bool, error) {
	return r.RenewAuthorityEpoch(ctx, roomID, instanceID, 0)
}

func (r *RoomAuthorityRegistry) RenewAuthorityEpoch(
	ctx context.Context,
	roomID string,
	instanceID string,
	epoch int64,
) (RoomAuthorityLease, bool, error) {
	if r == nil || r.client == nil {
		return RoomAuthorityLease{}, false, ErrRedisDisabled
	}
	if roomID == "" || instanceID == "" {
		return RoomAuthorityLease{}, false, errors.New("roomID and instanceID are required")
	}
	key := roomAuthorityKey(roomID)
	var current RoomAuthorityLease
	renewed := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode room authority lease: %w", err)
		}
		current = normalizeRoomAuthorityLease(current)
		if current.InstanceID != instanceID ||
			current.Status != RoomAuthorityStatusActive ||
			(epoch > 0 && current.Epoch != epoch) {
			return nil
		}
		next := current
		next.LeaseUntilMs = r.now().Add(r.ttl).UnixMilli()
		data, err := json.Marshal(next)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, r.ttl)
			return nil
		})
		if err == nil {
			current = next
			renewed = true
		}
		return err
	}, key)
	if err != nil {
		return RoomAuthorityLease{}, false, err
	}
	return current, renewed, nil
}

func (r *RoomAuthorityRegistry) BeginRecovery(
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
	now := r.now()
	var current RoomAuthorityLease
	started := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode room authority lease: %w", decodeErr)
			}
			current = normalizeRoomAuthorityLease(current)
			if !current.ExpiredAt(now) {
				return nil
			}
		}
		next := RoomAuthorityLease{
			InstanceID:   instanceID,
			Epoch:        normalizeRoomAuthorityLease(current).Epoch + 1,
			Status:       RoomAuthorityStatusRecovering,
			LeaseUntilMs: now.Add(r.ttl).UnixMilli(),
		}
		if current.InstanceID == "" {
			next.Epoch = 1
		}
		data, marshalErr := json.Marshal(next)
		if marshalErr != nil {
			return marshalErr
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, r.ttl)
			return nil
		})
		if pipeErr == nil {
			current = next
			started = true
		}
		return pipeErr
	}, key)
	if err != nil {
		return RoomAuthorityLease{}, false, err
	}
	return current, started, nil
}

func (r *RoomAuthorityRegistry) CompleteRecovery(
	ctx context.Context,
	roomID string,
	instanceID string,
	epoch int64,
) (RoomAuthorityLease, bool, error) {
	if r == nil || r.client == nil {
		return RoomAuthorityLease{}, false, ErrRedisDisabled
	}
	if roomID == "" || instanceID == "" || epoch <= 0 {
		return RoomAuthorityLease{}, false, errors.New("roomID, instanceID, and epoch are required")
	}
	key := roomAuthorityKey(roomID)
	var current RoomAuthorityLease
	completed := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode room authority lease: %w", err)
		}
		current = normalizeRoomAuthorityLease(current)
		if current.InstanceID != instanceID ||
			current.Epoch != epoch ||
			current.Status != RoomAuthorityStatusRecovering {
			return nil
		}
		next := current
		next.Status = RoomAuthorityStatusActive
		next.LeaseUntilMs = r.now().Add(r.ttl).UnixMilli()
		data, err := json.Marshal(next)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, r.ttl)
			return nil
		})
		if err == nil {
			current = next
			completed = true
		}
		return err
	}, key)
	if err != nil {
		return RoomAuthorityLease{}, false, err
	}
	return current, completed, nil
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
	return normalizeRoomAuthorityLease(lease), true, nil
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
		current = normalizeRoomAuthorityLease(current)
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

func (l RoomAuthorityLease) ExpiredAt(now time.Time) bool {
	if l.LeaseUntilMs <= 0 {
		return true
	}
	return l.LeaseUntilMs <= now.UnixMilli()
}

func (l RoomAuthorityLease) IsActive() bool {
	return normalizeRoomAuthorityLease(l).Status == RoomAuthorityStatusActive
}

func (l RoomAuthorityLease) IsRecovering() bool {
	return normalizeRoomAuthorityLease(l).Status == RoomAuthorityStatusRecovering
}

func normalizeRoomAuthorityLease(lease RoomAuthorityLease) RoomAuthorityLease {
	if lease.Epoch <= 0 {
		lease.Epoch = 1
	}
	if lease.Status == "" {
		lease.Status = RoomAuthorityStatusActive
	}
	return lease
}

func roomAuthorityKey(roomID string) string {
	return "wt:room:authority:" + roomID + ":v1"
}
