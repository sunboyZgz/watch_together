package room

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestManagerRemoveClientDeletesEmptyRoom(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	manager := newManagerWithClock(func() time.Time {
		return currentTime
	}, 2*time.Minute)
	room := manager.GetOrCreate("room_001")
	client := NewClientConnection(nil)
	client.SetIdentity("user_a", "room_001")

	room.Join(client)
	if got := manager.RoomCount(); got != 1 {
		t.Fatalf("expected 1 room, got %d", got)
	}
	if got := manager.ClientCount("room_001"); got != 1 {
		t.Fatalf("expected 1 client in room, got %d", got)
	}

	manager.RemoveClient(client)

	if got := manager.RoomCount(); got != 1 {
		t.Fatalf("expected room to remain during grace period, got %d rooms", got)
	}

	currentTime = currentTime.Add(2*time.Minute + time.Millisecond)
	manager.CleanupExpiredRooms()

	if got := manager.RoomCount(); got != 0 {
		t.Fatalf("expected room cleanup after grace period, got %d rooms", got)
	}
}

func TestManagerCreateRoomRegistersUniqueRoom(t *testing.T) {
	manager := NewManager()

	createdRoom, err := manager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	if len(createdRoom.ID()) != 6 {
		t.Fatalf("expected 6-char room id, got %q", createdRoom.ID())
	}
	if createdRoom.ID() != strings.ToUpper(createdRoom.ID()) {
		t.Fatalf("expected uppercase room id, got %q", createdRoom.ID())
	}

	state := createdRoom.StateSnapshot()
	if state.RoomID != createdRoom.ID() {
		t.Fatalf("expected state room id %q, got %q", createdRoom.ID(), state.RoomID)
	}
	if state.HostUserID != "user_a" {
		t.Fatalf("expected host user user_a, got %q", state.HostUserID)
	}
	if state.MediaID != "sample_001" {
		t.Fatalf("expected mediaId sample_001, got %q", state.MediaID)
	}
	if !state.Paused {
		t.Fatalf("expected new room paused=true")
	}
	if state.PositionMs != 0 {
		t.Fatalf("expected position 0, got %d", state.PositionMs)
	}
	if state.PlaybackRate != 1.0 {
		t.Fatalf("expected playbackRate 1.0, got %f", state.PlaybackRate)
	}
	if state.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", state.Seq)
	}

	storedRoom, ok := manager.Get(createdRoom.ID())
	if !ok {
		t.Fatalf("expected created room to be registered")
	}
	if storedRoom != createdRoom {
		t.Fatalf("expected stored room pointer to match created room")
	}
}

func TestRoomJoinReplacesPreviousConnectionForSameUser(t *testing.T) {
	room := NewCreatedRoom("ROOM01", "user_a", "sample_001")

	first := NewClientConnection(nil)
	first.SetIdentity("user_b", "ROOM01")
	firstJoin := room.Join(first)
	if firstJoin.ReplacedClient != nil {
		t.Fatalf("expected no replaced client on first join")
	}

	second := NewClientConnection(nil)
	second.SetIdentity("user_b", "ROOM01")
	secondJoin := room.Join(second)
	if secondJoin.ReplacedClient != first {
		t.Fatalf("expected first connection to be replaced")
	}
	if got := room.ClientCount(); got != 1 {
		t.Fatalf("expected one active connection after repeated join, got %d", got)
	}
	if secondJoin.State.HostUserID != "user_a" {
		t.Fatalf("expected host to stay user_a, got %s", secondJoin.State.HostUserID)
	}
}

func TestRoomStateSnapshotExtrapolatesCurrentPositionWhilePlaying(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	room := newWithClock("ROOM01", func() time.Time {
		return currentTime
	})
	room.state.MediaID = "sample_001"
	room.state.HostUserID = "user_a"

	if _, _, err := room.ApplyPlay("user_a", 5_000); err != nil {
		t.Fatalf("apply play: %v", err)
	}

	currentTime = currentTime.Add(3 * time.Second)
	state := room.StateSnapshot()
	if state.PositionMs != 8_000 {
		t.Fatalf("expected extrapolated position 8000, got %d", state.PositionMs)
	}
	if state.Seq != 2 {
		t.Fatalf("expected seq 2 after play, got %d", state.Seq)
	}
}

func TestManagerJoinDuringGracePeriodKeepsRoomAlive(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	manager := newManagerWithClock(func() time.Time {
		return currentTime
	}, 2*time.Minute)

	room := manager.GetOrCreate("room_001")
	firstClient := NewClientConnection(nil)
	firstClient.SetIdentity("user_a", "room_001")
	room.Join(firstClient)

	manager.RemoveClient(firstClient)
	if got := manager.RoomCount(); got != 1 {
		t.Fatalf("expected room to still exist during grace period, got %d rooms", got)
	}

	rejoinedRoom, ok := manager.Get("room_001")
	if !ok {
		t.Fatalf("expected room to be rejoinable during grace period")
	}
	secondClient := NewClientConnection(nil)
	secondClient.SetIdentity("user_b", "room_001")
	rejoinedRoom.Join(secondClient)
	manager.MarkRoomActive("room_001")

	currentTime = currentTime.Add(2*time.Minute + time.Millisecond)
	manager.CleanupExpiredRooms()

	if got := manager.RoomCount(); got != 1 {
		t.Fatalf("expected room to survive after rejoin during grace period, got %d rooms", got)
	}
}

func TestManagerLifecycleHooksTrackGracePeriodReactivationAndDestroy(t *testing.T) {
	currentTime := time.UnixMilli(1_000)
	manager := newManagerWithClock(func() time.Time {
		return currentTime
	}, 2*time.Minute)

	events := make([]string, 0, 3)
	manager.SetLifecycleHooks(LifecycleHooks{
		OnRoomBecameEmpty: func(roomID string, emptySince time.Time, destroyAfter time.Time) {
			events = append(
				events,
				fmt.Sprintf("empty:%s:%d:%d", roomID, emptySince.UnixMilli(), destroyAfter.UnixMilli()),
			)
		},
		OnRoomReactivated: func(roomID string) {
			events = append(events, "reactivated:"+roomID)
		},
		OnRoomDestroyed: func(roomID string) {
			events = append(events, "destroyed:"+roomID)
		},
	})

	room := manager.GetOrCreate("room_001")
	firstClient := NewClientConnection(nil)
	firstClient.SetIdentity("user_a", "room_001")
	room.Join(firstClient)

	manager.RemoveClient(firstClient)

	if len(events) != 1 {
		t.Fatalf("expected one empty hook event, got %v", events)
	}
	expectedEmpty := fmt.Sprintf(
		"empty:%s:%d:%d",
		"room_001",
		currentTime.UnixMilli(),
		currentTime.Add(2*time.Minute).UnixMilli(),
	)
	if events[0] != expectedEmpty {
		t.Fatalf("expected %q, got %q", expectedEmpty, events[0])
	}

	manager.MarkRoomActive("room_001")
	if len(events) != 2 || events[1] != "reactivated:room_001" {
		t.Fatalf("expected reactivated hook, got %v", events)
	}

	secondClient := NewClientConnection(nil)
	secondClient.SetIdentity("user_b", "room_001")
	room.Join(secondClient)
	manager.RemoveClient(secondClient)
	if len(events) != 3 {
		t.Fatalf("expected second empty event after room emptied again, got %v", events)
	}

	currentTime = currentTime.Add(2*time.Minute + time.Millisecond)
	manager.CleanupExpiredRooms()

	if got := manager.RoomCount(); got != 0 {
		t.Fatalf("expected room cleanup after grace period, got %d rooms", got)
	}
	if len(events) != 4 || events[3] != "destroyed:room_001" {
		t.Fatalf("expected destroyed hook, got %v", events)
	}
}

func TestRegisterCreatedRoomStartsGracePeriodUntilFirstJoin(t *testing.T) {
	currentTime := time.UnixMilli(5_000)
	manager := newManagerWithClock(func() time.Time {
		return currentTime
	}, 2*time.Minute)

	triggered := false
	var gotRoomID string
	var gotEmptySince time.Time
	var gotDestroyAfter time.Time
	manager.SetLifecycleHooks(LifecycleHooks{
		OnRoomBecameEmpty: func(roomID string, emptySince time.Time, destroyAfter time.Time) {
			triggered = true
			gotRoomID = roomID
			gotEmptySince = emptySince
			gotDestroyAfter = destroyAfter
		},
	})

	room := manager.RegisterCreatedRoom("A7K2M9", "user_a", "sample_001")

	if room == nil {
		t.Fatalf("expected room to be registered")
	}
	if !triggered {
		t.Fatalf("expected created room to start grace period immediately")
	}
	if gotRoomID != "A7K2M9" {
		t.Fatalf("expected roomID A7K2M9, got %s", gotRoomID)
	}
	if gotEmptySince.UnixMilli() != currentTime.UnixMilli() {
		t.Fatalf("expected emptySince %d, got %d", currentTime.UnixMilli(), gotEmptySince.UnixMilli())
	}
	if gotDestroyAfter.UnixMilli() != currentTime.Add(2*time.Minute).UnixMilli() {
		t.Fatalf(
			"expected destroyAfter %d, got %d",
			currentTime.Add(2*time.Minute).UnixMilli(),
			gotDestroyAfter.UnixMilli(),
		)
	}
}
