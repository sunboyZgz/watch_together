package authority

import (
	"context"
	"sync"
)

type SerialControlApplier struct {
	next  ControlApplier
	locks sync.Map
}

func NewSerialControlApplier(next ControlApplier) *SerialControlApplier {
	if next == nil {
		return nil
	}
	return &SerialControlApplier{next: next}
}

func (a *SerialControlApplier) ApplyRoomControl(ctx context.Context, request ApplyControlRequest) (ApplyControlResponse, error) {
	if a == nil || a.next == nil {
		return ApplyControlResponse{Error: "room authority unavailable"}, nil
	}
	lock := a.lockForRoom(request.RoomID)
	lock.Lock()
	defer lock.Unlock()
	return a.next.ApplyRoomControl(ctx, request)
}

func (a *SerialControlApplier) lockForRoom(roomID string) *sync.Mutex {
	if roomID == "" {
		roomID = "__empty__"
	}
	value, _ := a.locks.LoadOrStore(roomID, &sync.Mutex{})
	return value.(*sync.Mutex)
}
