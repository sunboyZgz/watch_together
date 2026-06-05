package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestWebSocketHandlerRejectsJoinWhenRoomIsFull(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandlerWithConfig(roomManager, false, WebSocketRuntimeConfig{
		MaxRoomClients: 1,
	}))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	ctx := context.Background()
	first := dialTestWebSocket(t, ctx, httpServer.URL, "user_b")
	defer first.Close(websocket.StatusNormalClosure, "test done")
	writeJoinRoom(t, ctx, first, createdRoom.ID(), "user_b")
	assertNextEnvelopeType(t, ctx, first, protocol.TypeRoomState)

	second := dialTestWebSocket(t, ctx, httpServer.URL, "user_c")
	defer second.Close(websocket.StatusNormalClosure, "test done")
	writeJoinRoom(t, ctx, second, createdRoom.ID(), "user_c")
	envelope := assertNextEnvelopeType(t, ctx, second, protocol.TypeError)

	var payload protocol.ErrorPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Message != "room is full" {
		t.Fatalf("expected room full error, got %q", payload.Message)
	}
}

func TestWebSocketHandlerRejectsConnectionWhenProcessLimitIsReached(t *testing.T) {
	roomManager := room.NewManager()
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandlerWithConfig(roomManager, false, WebSocketRuntimeConfig{
		MaxConnections: 1,
	}))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	ctx := context.Background()
	first := dialTestWebSocket(t, ctx, httpServer.URL, "user_a")
	defer first.Close(websocket.StatusNormalClosure, "test done")

	_, response, err := websocket.Dial(ctx, wsURL(httpServer.URL), websocketDialOptions("user_b"))
	if err == nil {
		t.Fatalf("expected second websocket dial to fail")
	}
	if response == nil {
		t.Fatalf("expected HTTP response for rejected websocket")
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, response.StatusCode)
	}
}

func dialTestWebSocket(t *testing.T, ctx context.Context, serverURL string, userID string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.Dial(ctx, wsURL(serverURL), websocketDialOptions(userID))
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func wsURL(serverURL string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/ws"
}

func writeJoinRoom(t *testing.T, ctx context.Context, conn *websocket.Conn, roomID string, userID string) {
	t.Helper()
	data, err := json.Marshal(protocol.Envelope{
		Type: protocol.TypeJoinRoom,
		Payload: mustJSONRaw(protocol.JoinRoomPayload{
			RoomID:   roomID,
			UserID:   userID,
			DeviceID: userID + "-device",
		}),
	})
	if err != nil {
		t.Fatalf("marshal join room: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write join room: %v", err)
	}
}

func assertNextEnvelopeType(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	expectedType string,
) protocol.Envelope {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}

	var envelope protocol.Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if envelope.Type != expectedType {
		t.Fatalf("expected envelope type %s, got %s", expectedType, envelope.Type)
	}
	return envelope
}
