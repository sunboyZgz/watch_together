package room

import (
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
