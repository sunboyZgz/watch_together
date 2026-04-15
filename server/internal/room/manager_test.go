package room

import (
	"strings"
	"testing"
)

func TestManagerRemoveClientDeletesEmptyRoom(t *testing.T) {
	manager := NewManager()
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

	if got := manager.RoomCount(); got != 0 {
		t.Fatalf("expected room cleanup after disconnect, got %d rooms", got)
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
