package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type ControlRateReservation struct {
	RoomID         string `json:"roomId"`
	ControlType    string `json:"controlType"`
	AuthorityEpoch int64  `json:"authorityEpoch"`
	Token          string `json:"token"`
	ReservedAtMs   int64  `json:"reservedAtMs"`
	LeaseUntilMs   int64  `json:"leaseUntilMs"`
}

type ControlRateRegistry struct {
	client *redis.Client
	now    func() time.Time
}

func NewControlRateRegistry(redisClient *RedisClient) *ControlRateRegistry {
	return NewControlRateRegistryWithClock(redisClient, time.Now)
}

func NewControlRateRegistryWithClock(redisClient *RedisClient, now func() time.Time) *ControlRateRegistry {
	if now == nil {
		now = time.Now
	}
	return &ControlRateRegistry{
		client: redisClient.Raw(),
		now:    now,
	}
}

func (r *ControlRateRegistry) Reserve(
	ctx context.Context,
	roomID string,
	controlType string,
	interval time.Duration,
	authorityEpoch int64,
) (ControlRateReservation, bool, error) {
	if interval <= 0 {
		return ControlRateReservation{}, true, nil
	}
	if r == nil || r.client == nil {
		return ControlRateReservation{}, false, ErrRedisDisabled
	}
	if roomID == "" || controlType == "" {
		return ControlRateReservation{}, false, errors.New("roomID and controlType are required")
	}
	key := ControlRateKey(roomID, controlType)
	now := r.now()
	reservation := ControlRateReservation{
		RoomID:         roomID,
		ControlType:    controlType,
		AuthorityEpoch: authorityEpoch,
		Token:          newControlRateToken(),
		ReservedAtMs:   now.UnixMilli(),
		LeaseUntilMs:   now.Add(interval).UnixMilli(),
	}
	data, err := json.Marshal(reservation)
	if err != nil {
		return ControlRateReservation{}, false, err
	}
	ok, err := r.client.SetNX(ctx, key, data, interval).Result()
	if err != nil {
		return ControlRateReservation{}, false, err
	}
	if ok {
		return reservation, true, nil
	}
	ttl, err := r.client.PTTL(ctx, key).Result()
	if err != nil {
		return ControlRateReservation{}, false, err
	}
	if ttl < 0 {
		ttl = interval
	}
	return ControlRateReservation{
		RoomID:       roomID,
		ControlType:  controlType,
		LeaseUntilMs: now.Add(ttl).UnixMilli(),
	}, false, nil
}

func (r *ControlRateRegistry) ReleaseIfMatch(
	ctx context.Context,
	reservation ControlRateReservation,
) (bool, error) {
	if reservation.RoomID == "" || reservation.ControlType == "" || reservation.Token == "" {
		return false, nil
	}
	if r == nil || r.client == nil {
		return false, ErrRedisDisabled
	}
	key := ControlRateKey(reservation.RoomID, reservation.ControlType)
	released := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		var current ControlRateReservation
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode control rate reservation: %w", err)
		}
		if current.Token != reservation.Token {
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

func ControlRateKey(roomID string, controlType string) string {
	return "wt:room:control_rate:" + roomID + ":" + controlType + ":v1"
}

func newControlRateToken() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
