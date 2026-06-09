package authority

import (
	"context"
	"encoding/json"

	"watch_together/server/internal/cache"
)

type LifecycleRegistry interface {
	GetAuthority(ctx context.Context, roomID string) (cache.RoomAuthorityLease, bool, error)
	ClaimAuthority(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error)
	RenewAuthorityEpoch(ctx context.Context, roomID string, instanceID string, epoch int64) (cache.RoomAuthorityLease, bool, error)
	BeginRecovery(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error)
	CompleteRecovery(ctx context.Context, roomID string, instanceID string, epoch int64) (cache.RoomAuthorityLease, bool, error)
}

type ControlApplier interface {
	ApplyRoomControl(ctx context.Context, request ApplyControlRequest) (ApplyControlResponse, error)
}

type ApplyControlRequest struct {
	SourceInstanceID       string
	RoomID                 string
	UserID                 string
	DeviceID               string
	ConnectionID           string
	Type                   string
	Payload                json.RawMessage
	RequestID              string
	Seq                    int64
	ExpectedAuthorityEpoch int64
	RequestedAtMs          int64
}

type ApplyControlResponse struct {
	Accepted       bool
	Type           string
	Payload        json.RawMessage
	Seq            int64
	AuthorityEpoch int64
	Error          string
}
