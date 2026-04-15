package room

import "testing"

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
