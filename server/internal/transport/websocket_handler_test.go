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
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
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
			RoomID: createdRoom.ID(),
			UserID: "user_b",
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

	if payload.RoomID != createdRoom.ID() {
		t.Fatalf("expected %s, got %s", createdRoom.ID(), payload.RoomID)
	}
	if payload.HostUserID != "user_a" {
		t.Fatalf("expected host user user_a, got %s", payload.HostUserID)
	}
	if !payload.Paused {
		t.Fatalf("expected initial room state paused=true")
	}
	if got := roomManager.ClientCount(createdRoom.ID()); got != 1 {
		t.Fatalf("expected 1 joined client in room, got %d", got)
	}
}

func TestWebSocketJoinRoomMissingRoomReturnsError(t *testing.T) {
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
			RoomID: "ROOM01",
			UserID: "user_b",
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

	var envelope protocol.ErrorEnvelope
	if err := json.Unmarshal(responseData, &envelope); err != nil {
		t.Fatalf("unmarshal error envelope: %v", err)
	}
	if envelope.Type != protocol.TypeError {
		t.Fatalf("expected error response, got %s", envelope.Type)
	}
	if envelope.Payload.RoomID != "ROOM01" {
		t.Fatalf("expected error roomId ROOM01, got %s", envelope.Payload.RoomID)
	}
	if envelope.Payload.Message != "room not found" {
		t.Fatalf("expected room not found, got %s", envelope.Payload.Message)
	}
}

func TestWebSocketControlSyncFlow(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandler(roomManager, true))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx := context.Background()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn := mustDialWebSocket(t, ctx, wsURL)
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 12_000,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, 12_000, 2)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypePlay, 12_000, 2)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePause,
		Payload: mustJSONRaw(protocol.PausePayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 13_500,
			Seq:        2,
		}),
	})

	assertControlBroadcast(t, ctx, hostConn, protocol.TypePause, 13_500, 3)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypePause, 13_500, 3)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeSeek,
		Payload: mustJSONRaw(protocol.SeekPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 42_000,
			Seq:        3,
		}),
	})

	assertControlBroadcast(t, ctx, hostConn, protocol.TypeSeek, 42_000, 4)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypeSeek, 42_000, 4)

	state := createdRoom.StateSnapshot()
	if state.PositionMs != 42_000 {
		t.Fatalf("expected final room position 42000, got %d", state.PositionMs)
	}
	if !state.Paused {
		t.Fatalf("expected seek to preserve paused=true after pause and seek sequence")
	}
	if state.Seq != 4 {
		t.Fatalf("expected final seq 4, got %d", state.Seq)
	}
}

func TestWebSocketControlSyncRejectsNonHost(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandler(roomManager, true))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx := context.Background()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn := mustDialWebSocket(t, ctx, wsURL)
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	mustSendEnvelope(t, ctx, viewerConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_b",
			PositionMs: 5_000,
			Seq:        1,
		}),
	})

	var envelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, viewerConn, &envelope)
	if envelope.Type != protocol.TypeError {
		t.Fatalf("expected error response, got %s", envelope.Type)
	}
	if envelope.Payload.Message != "only host can control playback" {
		t.Fatalf("unexpected error message: %s", envelope.Payload.Message)
	}
	state := createdRoom.StateSnapshot()
	if state.Seq != 1 {
		t.Fatalf("expected seq to stay 1, got %d", state.Seq)
	}
}

func mustDialWebSocket(t *testing.T, ctx context.Context, wsURL string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func mustJoinRoom(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	roomID string,
	userID string,
) {
	t.Helper()
	mustSendEnvelope(t, ctx, conn, protocol.Envelope{
		Type: protocol.TypeJoinRoom,
		Payload: mustJSONRaw(protocol.JoinRoomPayload{
			RoomID: roomID,
			UserID: userID,
		}),
	})
}

func mustSendEnvelope(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	envelope protocol.Envelope,
) {
	t.Helper()
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
}

func mustReadEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn) protocol.Envelope {
	t.Helper()
	var envelope protocol.Envelope
	readMessageAs(t, ctx, conn, &envelope)
	return envelope
}

func readMessageAs(t *testing.T, ctx context.Context, conn *websocket.Conn, target any) {
	t.Helper()
	_, responseData, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if err := json.Unmarshal(responseData, target); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
}

func assertControlBroadcast(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	expectedType string,
	expectedPosition int64,
	expectedSeq int64,
) {
	t.Helper()
	envelope := mustReadEnvelope(t, ctx, conn)
	if envelope.Type != expectedType {
		t.Fatalf("expected %s, got %s", expectedType, envelope.Type)
	}

	switch expectedType {
	case protocol.TypePlay:
		var payload protocol.PlayPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal play payload: %v", err)
		}
		if payload.PositionMs != expectedPosition || payload.Seq != expectedSeq {
			t.Fatalf("unexpected play payload: %+v", payload)
		}
	case protocol.TypePause:
		var payload protocol.PausePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal pause payload: %v", err)
		}
		if payload.PositionMs != expectedPosition || payload.Seq != expectedSeq {
			t.Fatalf("unexpected pause payload: %+v", payload)
		}
	case protocol.TypeSeek:
		var payload protocol.SeekPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal seek payload: %v", err)
		}
		if payload.PositionMs != expectedPosition || payload.Seq != expectedSeq {
			t.Fatalf("unexpected seek payload: %+v", payload)
		}
	default:
		t.Fatalf("unsupported expected control type %s", expectedType)
	}
}
