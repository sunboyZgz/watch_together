package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestWebSocketJoinRoomFlow(t *testing.T) {
	roomManager := room.NewManager()
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandler(roomManager, true))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx := context.Background()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	joinData, err := json.Marshal(protocol.Envelope{
		Type: protocol.TypeJoinRoom,
		Payload: mustJSONRaw(protocol.JoinRoomPayload{
			RoomID: "room_001",
			UserID: "user_a",
		}),
	})
	if err != nil {
		t.Fatalf("marshal join message: %v", err)
	}

	if err := conn.Write(ctx, websocket.MessageText, joinData); err != nil {
		t.Fatalf("write join message: %v", err)
	}

	_, responseData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var envelope protocol.Envelope
	if err := json.Unmarshal(responseData, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state response, got %s", envelope.Type)
	}

	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}

	if payload.RoomID != "room_001" {
		t.Fatalf("expected room_001, got %s", payload.RoomID)
	}
	if payload.HostUserID != "user_a" {
		t.Fatalf("expected host user user_a, got %s", payload.HostUserID)
	}
	if !payload.Paused {
		t.Fatalf("expected initial room state paused=true")
	}
}
