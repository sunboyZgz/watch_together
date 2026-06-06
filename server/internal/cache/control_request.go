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
	defaultControlRequestTTL = 10 * time.Minute

	ControlRequestStatusPending  = "pending"
	ControlRequestStatusAccepted = "accepted"
	ControlRequestStatusRejected = "rejected"
)

type ControlRequestRecord struct {
	RoomID         string          `json:"roomId"`
	RequestID      string          `json:"requestId"`
	Status         string          `json:"status"`
	AuthorityEpoch int64           `json:"authorityEpoch"`
	Seq            int64           `json:"seq,omitempty"`
	Envelope       json.RawMessage `json:"envelope,omitempty"`
	Error          string          `json:"error,omitempty"`
	LeaseUntilMs   int64           `json:"leaseUntilMs"`
}

type ControlRequestRegistry struct {
	client *redis.Client
	ttl    time.Duration
	now    func() time.Time
}

func NewControlRequestRegistry(redisClient *RedisClient, ttl time.Duration) *ControlRequestRegistry {
	return NewControlRequestRegistryWithClock(redisClient, ttl, time.Now)
}

func NewControlRequestRegistryWithClock(
	redisClient *RedisClient,
	ttl time.Duration,
	now func() time.Time,
) *ControlRequestRegistry {
	if ttl <= 0 {
		ttl = defaultControlRequestTTL
	}
	if now == nil {
		now = time.Now
	}
	return &ControlRequestRegistry{
		client: redisClient.Raw(),
		ttl:    ttl,
		now:    now,
	}
}

func (r *ControlRequestRegistry) Reserve(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
) (ControlRequestRecord, bool, error) {
	if r == nil || r.client == nil {
		return ControlRequestRecord{}, false, ErrRedisDisabled
	}
	if roomID == "" || requestID == "" || authorityEpoch <= 0 {
		return ControlRequestRecord{}, false, errors.New("roomID, requestID, and authorityEpoch are required")
	}
	key := ControlRequestKey(roomID, requestID)
	next := ControlRequestRecord{
		RoomID:         roomID,
		RequestID:      requestID,
		Status:         ControlRequestStatusPending,
		AuthorityEpoch: authorityEpoch,
		LeaseUntilMs:   r.now().Add(r.ttl).UnixMilli(),
	}
	data, err := json.Marshal(next)
	if err != nil {
		return ControlRequestRecord{}, false, err
	}

	var current ControlRequestRecord
	reserved := false
	err = r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		if err == nil {
			if decodeErr := json.Unmarshal(existing, &current); decodeErr != nil {
				return fmt.Errorf("decode control request: %w", decodeErr)
			}
			return nil
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, data, r.ttl)
			return nil
		})
		if pipeErr == nil {
			current = next
			reserved = true
		}
		return pipeErr
	}, key)
	if err != nil {
		return ControlRequestRecord{}, false, err
	}
	return current, reserved, nil
}

func (r *ControlRequestRegistry) FinalizeAccepted(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
	seq int64,
	envelope []byte,
) (ControlRequestRecord, bool, error) {
	return r.finalize(ctx, roomID, requestID, authorityEpoch, ControlRequestStatusAccepted, seq, envelope, "")
}

func (r *ControlRequestRegistry) FinalizeRejected(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
	seq int64,
	message string,
) (ControlRequestRecord, bool, error) {
	return r.finalize(ctx, roomID, requestID, authorityEpoch, ControlRequestStatusRejected, seq, nil, message)
}

func (r *ControlRequestRegistry) finalize(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
	status string,
	seq int64,
	envelope []byte,
	message string,
) (ControlRequestRecord, bool, error) {
	if r == nil || r.client == nil {
		return ControlRequestRecord{}, false, ErrRedisDisabled
	}
	if roomID == "" || requestID == "" || authorityEpoch <= 0 {
		return ControlRequestRecord{}, false, errors.New("roomID, requestID, and authorityEpoch are required")
	}
	key := ControlRequestKey(roomID, requestID)
	var current ControlRequestRecord
	finalized := false
	err := r.client.Watch(ctx, func(tx *redis.Tx) error {
		existing, err := tx.Get(ctx, key).Bytes()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return nil
			}
			return err
		}
		if err := json.Unmarshal(existing, &current); err != nil {
			return fmt.Errorf("decode control request: %w", err)
		}
		if current.RoomID != roomID ||
			current.RequestID != requestID ||
			current.AuthorityEpoch != authorityEpoch ||
			current.Status != ControlRequestStatusPending {
			return nil
		}
		next := current
		next.Status = status
		next.Seq = seq
		next.Envelope = cloneRawMessage(envelope)
		next.Error = message
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
			finalized = true
		}
		return err
	}, key)
	if err != nil {
		return ControlRequestRecord{}, false, err
	}
	return current, finalized, nil
}

func (r *ControlRequestRegistry) Forget(ctx context.Context, roomID string, requestID string) error {
	if r == nil || r.client == nil {
		return ErrRedisDisabled
	}
	if roomID == "" || requestID == "" {
		return errors.New("roomID and requestID are required")
	}
	return r.client.Del(ctx, ControlRequestKey(roomID, requestID)).Err()
}

func ControlRequestKey(roomID string, requestID string) string {
	return "wt:room:control_request:" + roomID + ":" + requestID + ":v1"
}

func cloneRawMessage(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
