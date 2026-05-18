package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/realtime"
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
	conn, _, err := websocket.Dial(ctx, wsURL, websocketDialOptions("user_b"))
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
	if payload.Ended {
		t.Fatalf("expected initial room state ended=false")
	}
	if got := roomManager.ClientCount(createdRoom.ID()); got != 1 {
		t.Fatalf("expected 1 joined client in room, got %d", got)
	}
}

func TestWebSocketJoinBroadcastsRoomMembersChangedToExistingClients(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	hostState := mustReadEnvelope(t, ctx, hostConn)
	if hostState.Type != protocol.TypeRoomState {
		t.Fatalf("expected initial host room_state, got %s", hostState.Type)
	}

	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	viewerState := mustReadEnvelope(t, ctx, viewerConn)
	if viewerState.Type != protocol.TypeRoomState {
		t.Fatalf("expected viewer room_state, got %s", viewerState.Type)
	}

	membersChanged := mustReadEnvelope(t, ctx, hostConn)
	if membersChanged.Type != protocol.TypeRoomMembersChanged {
		t.Fatalf("expected room_members_changed on existing host, got %s", membersChanged.Type)
	}
	var payload protocol.RoomMembersChangedPayload
	if err := json.Unmarshal(membersChanged.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_members_changed payload: %v", err)
	}
	if payload.RoomID != createdRoom.ID() {
		t.Fatalf("expected roomId %s, got %s", createdRoom.ID(), payload.RoomID)
	}
	if payload.Reason != "join" {
		t.Fatalf("expected join reason, got %s", payload.Reason)
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
	conn, _, err := websocket.Dial(ctx, wsURL, websocketDialOptions("user_b"))
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

func TestWebSocketRejectsMissingAccessToken(t *testing.T) {
	roomManager := room.NewManager()
	mux := http.NewServeMux()
	mux.Handle("/ws", NewWebSocketHandler(roomManager, true))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx := context.Background()
	_, response, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatalf("expected websocket dial without token to fail")
	}
	if response == nil {
		t.Fatalf("expected HTTP response for rejected websocket")
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, response.StatusCode)
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
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
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

	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, -1, 2)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypePlay, -1, 2)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePause,
		Payload: mustJSONRaw(protocol.PausePayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 13_500,
			Seq:        2,
		}),
	})

	assertControlBroadcast(t, ctx, hostConn, protocol.TypePause, -1, 3)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypePause, -1, 3)

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

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeSetPlaybackRate,
		Payload: mustJSONRaw(protocol.SetPlaybackRatePayload{
			RoomID:       createdRoom.ID(),
			UserID:       "user_a",
			PositionMs:   42_000,
			PlaybackRate: 1.5,
			Seq:          4,
		}),
	})

	assertPlaybackRateBroadcast(t, ctx, hostConn, 42_000, 1.5, 5)
	assertPlaybackRateBroadcast(t, ctx, viewerConn, 42_000, 1.5, 5)

	state := createdRoom.StateSnapshot()
	if state.PositionMs != 42_000 {
		t.Fatalf("expected final room position 42000, got %d", state.PositionMs)
	}
	if !state.Paused {
		t.Fatalf("expected seek to preserve paused=true after pause and seek sequence")
	}
	if state.Seq != 5 {
		t.Fatalf("expected final seq 5, got %d", state.Seq)
	}
	if state.PlaybackRate != 1.5 {
		t.Fatalf("expected playbackRate 1.5, got %f", state.PlaybackRate)
	}
}

func TestWebSocketRoomStateRequestReturnsLatestSnapshot(t *testing.T) {
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

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	initial := mustReadEnvelope(t, ctx, hostConn)
	if initial.Type != protocol.TypeRoomState {
		t.Fatalf("expected initial room_state, got %s", initial.Type)
	}

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 12_000,
			Seq:        1,
		}),
	})
	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, -1, 2)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeRoomStateRequest,
		Payload: mustJSONRaw(protocol.RoomStateRequestPayload{
			RoomID: createdRoom.ID(),
			UserID: "user_a",
			Seq:    1,
		}),
	})

	response := mustReadEnvelope(t, ctx, hostConn)
	if response.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state response, got %s", response.Type)
	}
	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}
	if payload.Seq != 2 {
		t.Fatalf("expected latest seq 2, got %d", payload.Seq)
	}
	if payload.Paused {
		t.Fatalf("expected latest room_state to reflect playing state")
	}
}

func TestWebSocketControlRequestIDDeduplicatesAcceptedControl(t *testing.T) {
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

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)

	requestID := "req-play-1"
	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  requestID,
			PositionMs: 12_000,
			Seq:        1,
		}),
	})

	first := mustReadEnvelope(t, ctx, hostConn)
	if first.Type != protocol.TypePlay {
		t.Fatalf("expected first request to broadcast play, got %s", first.Type)
	}
	var playPayload protocol.PlayPayload
	if err := json.Unmarshal(first.Payload, &playPayload); err != nil {
		t.Fatalf("unmarshal play payload: %v", err)
	}
	if playPayload.Seq != 2 || playPayload.RequestID != requestID {
		t.Fatalf("unexpected play payload after first request: %+v", playPayload)
	}

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  requestID,
			PositionMs: 12_000,
			Seq:        1,
		}),
	})

	duplicate := mustReadEnvelope(t, ctx, hostConn)
	if duplicate.Type != protocol.TypeRoomState {
		t.Fatalf("expected duplicate request to return room_state, got %s", duplicate.Type)
	}
	var state protocol.RoomStatePayload
	if err := json.Unmarshal(duplicate.Payload, &state); err != nil {
		t.Fatalf("unmarshal duplicate room_state payload: %v", err)
	}
	if state.Seq != 2 {
		t.Fatalf("expected duplicate request to keep seq 2, got %d", state.Seq)
	}
}

func TestWebSocketControlRejectsSeqMismatchWithLatestRoomState(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "req-play-current",
			PositionMs: 12_000,
			Seq:        1,
		}),
	})
	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, -1, 2)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePause,
		Payload: mustJSONRaw(protocol.PausePayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "req-pause-stale",
			PositionMs: 13_500,
			Seq:        1,
		}),
	})

	response := mustReadEnvelope(t, ctx, hostConn)
	if response.Type != protocol.TypeRoomState {
		t.Fatalf("expected stale control to return room_state, got %s", response.Type)
	}
	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("unmarshal stale room_state payload: %v", err)
	}
	if payload.Seq != 2 {
		t.Fatalf("expected stale control to keep seq 2, got %d", payload.Seq)
	}
	if payload.Paused {
		t.Fatalf("expected stale pause not to overwrite accepted play state")
	}

	state := createdRoom.StateSnapshot()
	if state.Seq != 2 || state.Paused {
		t.Fatalf("expected room timeline to stay at accepted play state, got seq=%d paused=%t", state.Seq, state.Paused)
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
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
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

	mustSendEnvelope(t, ctx, viewerConn, protocol.Envelope{
		Type: protocol.TypeSetPlaybackRate,
		Payload: mustJSONRaw(protocol.SetPlaybackRatePayload{
			RoomID:       createdRoom.ID(),
			UserID:       "user_b",
			PositionMs:   5_000,
			PlaybackRate: 1.5,
			Seq:          1,
		}),
	})

	readMessageAs(t, ctx, viewerConn, &envelope)
	if envelope.Type != protocol.TypeError {
		t.Fatalf("expected error response, got %s", envelope.Type)
	}
	if envelope.Payload.Message != "only host can control playback" {
		t.Fatalf("unexpected playback rate error message: %s", envelope.Payload.Message)
	}
}

func TestWebSocketControlIgnoresSpoofedPayloadUserID(t *testing.T) {
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

	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	mustSendEnvelope(t, ctx, viewerConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
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
		t.Fatalf("expected only-host error, got %q", envelope.Payload.Message)
	}
}

func TestWebSocketHostDisconnectPausesRoomWithoutTransfer(t *testing.T) {
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
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	if err := hostConn.Close(websocket.StatusNormalClosure, "host leaves"); err != nil {
		t.Fatalf("close host websocket: %v", err)
	}

	roomStateEnvelope := mustReadEnvelope(t, ctx, viewerConn)
	if roomStateEnvelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state broadcast, got %s", roomStateEnvelope.Type)
	}

	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(roomStateEnvelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}
	if payload.HostUserID != "" {
		t.Fatalf("expected no online host after host disconnect, got %s", payload.HostUserID)
	}
	if payload.Seq != 2 {
		t.Fatalf("expected seq 2 after host disconnect, got %d", payload.Seq)
	}
	if !payload.Paused || payload.Velocity != 0 {
		t.Fatalf("expected playback paused after host disconnect, paused=%t velocity=%f", payload.Paused, payload.Velocity)
	}
	if payload.Reason != "host_left" {
		t.Fatalf("expected host_left reason, got %s", payload.Reason)
	}

	mustSendEnvelope(t, ctx, viewerConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_b",
			PositionMs: 9_000,
			Seq:        payload.Seq,
		}),
	})

	var errorEnvelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, viewerConn, &errorEnvelope)
	if errorEnvelope.Type != protocol.TypeError {
		t.Fatalf("expected error for viewer control without host, got %s", errorEnvelope.Type)
	}
	if errorEnvelope.Payload.Message != "only host can control playback" {
		t.Fatalf("expected host control rejection, got %s", errorEnvelope.Payload.Message)
	}

	state := createdRoom.StateSnapshot()
	if state.HostUserID != "" {
		t.Fatalf("expected room to have no online host after disconnect, got %s", state.HostUserID)
	}
	if !state.Paused || state.Velocity != 0 {
		t.Fatalf("expected room playback paused after disconnect, paused=%t velocity=%f", state.Paused, state.Velocity)
	}
}

/*
*
它验证的是下面这条链路：
客户端连接 WebSocket
加入房间
服务端发送 heartbeat
客户端返回 heartbeat ack
服务端认为客户端仍然存活
连接继续保持
服务端还能发送下一次 heartbeat
*/
func TestWebSocketHeartbeatAckKeepsConnectionAlive(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", newWebSocketHandler(roomManager, true, 20*time.Millisecond, 80*time.Millisecond))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn := mustDialWebSocket(t, ctx, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, conn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, conn)

	firstHeartbeat := mustReadEnvelope(t, ctx, conn)
	if firstHeartbeat.Type != protocol.TypeHeartbeat {
		t.Fatalf("expected heartbeat, got %s", firstHeartbeat.Type)
	}
	var heartbeat protocol.HeartbeatPayload
	if err := json.Unmarshal(firstHeartbeat.Payload, &heartbeat); err != nil {
		t.Fatalf("unmarshal heartbeat payload: %v", err)
	}

	mustSendEnvelope(t, ctx, conn, protocol.Envelope{
		Type: protocol.TypeHeartbeatAck,
		Payload: mustJSONRaw(protocol.HeartbeatAckPayload{
			ServerTimeMs: heartbeat.ServerTimeMs,
			ClientTimeMs: heartbeat.ServerTimeMs + 1,
		}),
	})

	secondHeartbeat := mustReadEnvelope(t, ctx, conn)
	if secondHeartbeat.Type != protocol.TypeHeartbeat {
		t.Fatalf("expected second heartbeat, got %s", secondHeartbeat.Type)
	}
}

func TestWebSocketClockSyncPingReturnsServerTime(t *testing.T) {
	roomManager := room.NewManager()
	mux := http.NewServeMux()
	serverNow := time.UnixMilli(987_654_321)
	mux.Handle(
		"/ws",
		newWebSocketHandlerWithClock(
			roomManager,
			true,
			defaultHeartbeatInterval,
			defaultHeartbeatTimeout,
			realtime.ClockFunc(func() time.Time {
				return serverNow
			}),
		),
	)

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn := mustDialWebSocket(t, ctx, wsURL)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	clientSendMonoMs := int64(123_456)
	mustSendEnvelope(t, ctx, conn, protocol.Envelope{
		Type: protocol.TypeClockSyncPing,
		Payload: mustJSONRaw(protocol.ClockSyncPingPayload{
			ClientSendMonoMs: clientSendMonoMs,
		}),
	})

	envelope := mustReadEnvelope(t, ctx, conn)
	if envelope.Type != protocol.TypeClockSyncPong {
		t.Fatalf("expected clock_sync.pong, got %s", envelope.Type)
	}

	var payload protocol.ClockSyncPongPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal clock_sync.pong payload: %v", err)
	}
	if payload.ClientSendMonoMs != clientSendMonoMs {
		t.Fatalf("expected clientSendMonoMs %d, got %d", clientSendMonoMs, payload.ClientSendMonoMs)
	}
	if payload.ServerTimeMs != serverNow.UnixMilli() {
		t.Fatalf("expected serverTimeMs %d, got %d", serverNow.UnixMilli(), payload.ServerTimeMs)
	}
}

func TestWebSocketHeartbeatTimeoutRemovesSilentClient(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", newWebSocketHandler(roomManager, true, 20*time.Millisecond, 60*time.Millisecond))

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	wsURL := "ws" + httpServer.URL[len("http"):] + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn := mustDialWebSocket(t, ctx, wsURL)
	mustJoinRoom(t, ctx, conn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, conn)

	readUntilClosed(t, ctx, conn)

	waitFor(t, time.Second, func() bool {
		return roomManager.ClientCount(createdRoom.ID()) == 0
	})
}

func TestWebSocketRepeatedJoinReplacesPreviousConnectionForSameUser(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	firstConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	secondConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer secondConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, firstConn, createdRoom.ID(), "user_b")
	firstRoomState := mustReadEnvelope(t, ctx, firstConn)
	if firstRoomState.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on first join, got %s", firstRoomState.Type)
	}

	mustJoinRoom(t, ctx, secondConn, createdRoom.ID(), "user_b")
	secondRoomState := mustReadEnvelope(t, ctx, secondConn)
	if secondRoomState.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on repeated join, got %s", secondRoomState.Type)
	}

	waitFor(t, time.Second, func() bool {
		return roomManager.ClientCount(createdRoom.ID()) == 1
	})

	readUntilClosed(t, ctx, firstConn)

	state := createdRoom.StateSnapshot()
	if state.HostUserID != "user_a" {
		t.Fatalf("expected host to stay user_a, got %s", state.HostUserID)
	}
}

func TestWebSocketRepeatedJoinKeepsHostIdentityStable(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn1 := mustDialWebSocket(t, ctx, wsURL)
	hostConn2 := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn2.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn1, createdRoom.ID(), "user_a")
	firstRoomState := mustReadEnvelope(t, ctx, hostConn1)
	if firstRoomState.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on first host join, got %s", firstRoomState.Type)
	}

	mustJoinRoom(t, ctx, hostConn2, createdRoom.ID(), "user_a")
	secondRoomState := mustReadEnvelope(t, ctx, hostConn2)
	if secondRoomState.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on repeated host join, got %s", secondRoomState.Type)
	}

	readUntilClosed(t, ctx, hostConn1)

	state := createdRoom.StateSnapshot()
	if state.HostUserID != "user_a" {
		t.Fatalf("expected host identity to stay user_a, got %s", state.HostUserID)
	}
	if state.Seq != 1 {
		t.Fatalf("expected repeated host join not to change seq, got %d", state.Seq)
	}
	if got := roomManager.ClientCount(createdRoom.ID()); got != 1 {
		t.Fatalf("expected one active host connection after repeated join, got %d", got)
	}
}

func TestWebSocketRepeatedJoinReturnsCurrentEffectiveRoomState(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn1 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	viewerConn2 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn2.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn1, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn1)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, hostConn, protocol.TypePlay, 0, 2)
	assertControlBroadcast(t, ctx, viewerConn1, protocol.TypePlay, 0, 2)

	time.Sleep(40 * time.Millisecond)

	mustJoinRoom(t, ctx, viewerConn2, createdRoom.ID(), "user_b")
	rejoinEnvelope := mustReadEnvelope(t, ctx, viewerConn2)
	if rejoinEnvelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on repeated join, got %s", rejoinEnvelope.Type)
	}

	readUntilClosed(t, ctx, viewerConn1)

	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(rejoinEnvelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}
	if payload.Paused {
		t.Fatalf("expected rejoin room_state to stay in playing state")
	}
	if payload.PositionMs <= 0 {
		t.Fatalf("expected repeated join to receive advanced effective position, got %d", payload.PositionMs)
	}
	if payload.Seq != 2 {
		t.Fatalf("expected repeated join room_state seq 2, got %d", payload.Seq)
	}
}

func TestWebSocketRepeatedJoinKeepsCurrentPlaybackRateInRoomState(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn1 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	viewerConn2 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn2.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn1, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn1)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeSetPlaybackRate,
		Payload: mustJSONRaw(protocol.SetPlaybackRatePayload{
			RoomID:       createdRoom.ID(),
			UserID:       "user_a",
			PositionMs:   0,
			PlaybackRate: 1.5,
			Seq:          1,
		}),
	})

	assertPlaybackRateBroadcast(t, ctx, hostConn, 0, 1.5, 2)
	assertPlaybackRateBroadcast(t, ctx, viewerConn1, 0, 1.5, 2)

	mustJoinRoom(t, ctx, viewerConn2, createdRoom.ID(), "user_b")
	rejoinEnvelope := mustReadEnvelope(t, ctx, viewerConn2)
	if rejoinEnvelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on repeated join, got %s", rejoinEnvelope.Type)
	}

	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(rejoinEnvelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}
	if payload.PlaybackRate != 1.5 {
		t.Fatalf("expected repeated join room_state playbackRate 1.5, got %f", payload.PlaybackRate)
	}
}

func TestWebSocketEndedBroadcastAndRepeatedJoinState(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	defer hostConn.Close(websocket.StatusNormalClosure, "test done")
	viewerConn1 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	viewerConn2 := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn2.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn1, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn1)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeEnded,
		Payload: mustJSONRaw(protocol.EndedPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 210_000,
			Seq:        1,
		}),
	})

	assertEndedBroadcast(t, ctx, hostConn, 210_000, 2)
	assertEndedBroadcast(t, ctx, viewerConn1, 210_000, 2)

	state := createdRoom.StateSnapshot()
	if !state.Ended {
		t.Fatalf("expected room ended=true after ended event")
	}
	if !state.Paused {
		t.Fatalf("expected room paused=true after ended event")
	}

	mustJoinRoom(t, ctx, viewerConn2, createdRoom.ID(), "user_b")
	rejoinEnvelope := mustReadEnvelope(t, ctx, viewerConn2)
	if rejoinEnvelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on repeated join, got %s", rejoinEnvelope.Type)
	}

	var payload protocol.RoomStatePayload
	if err := json.Unmarshal(rejoinEnvelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_state payload: %v", err)
	}
	if !payload.Ended {
		t.Fatalf("expected repeated join room_state ended=true")
	}
	if !payload.Paused {
		t.Fatalf("expected repeated join room_state paused=true")
	}
	if payload.PositionMs != 210_000 {
		t.Fatalf("expected repeated join room_state frozen position 210000, got %d", payload.PositionMs)
	}
}

func TestWebSocketEndedRejectsNonHost(t *testing.T) {
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
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	mustSendEnvelope(t, ctx, viewerConn, protocol.Envelope{
		Type: protocol.TypeEnded,
		Payload: mustJSONRaw(protocol.EndedPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_b",
			PositionMs: 210_000,
			Seq:        1,
		}),
	})

	var envelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, viewerConn, &envelope)
	if envelope.Type != protocol.TypeError {
		t.Fatalf("expected error response, got %s", envelope.Type)
	}
	if envelope.Payload.Message != "only host can control playback" {
		t.Fatalf("unexpected ended error message: %s", envelope.Payload.Message)
	}
}

func TestWebSocketSeekClearsEndedState(t *testing.T) {
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
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeEnded,
		Payload: mustJSONRaw(protocol.EndedPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 210_000,
			Seq:        1,
		}),
	})
	assertEndedBroadcast(t, ctx, hostConn, 210_000, 2)
	assertEndedBroadcast(t, ctx, viewerConn, 210_000, 2)

	mustSendEnvelope(t, ctx, hostConn, protocol.Envelope{
		Type: protocol.TypeSeek,
		Payload: mustJSONRaw(protocol.SeekPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 120_000,
			Seq:        2,
		}),
	})
	assertControlBroadcast(t, ctx, hostConn, protocol.TypeSeek, 120_000, 3)
	assertControlBroadcast(t, ctx, viewerConn, protocol.TypeSeek, 120_000, 3)

	state := createdRoom.StateSnapshot()
	if state.Ended {
		t.Fatalf("expected seek to clear ended state")
	}
}

func TestWebSocketHostReconnectRestoresHostControl(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	hostConn := mustDialWebSocket(t, ctx, wsURL)
	viewerConn := mustDialWebSocket(t, ctx, wsURL, "user_b")
	defer viewerConn.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostConn, createdRoom.ID(), "user_a")
	mustReadEnvelope(t, ctx, hostConn)
	mustJoinRoom(t, ctx, viewerConn, createdRoom.ID(), "user_b")
	mustReadEnvelope(t, ctx, viewerConn)

	if err := hostConn.Close(websocket.StatusNormalClosure, "host leaves"); err != nil {
		t.Fatalf("close host websocket: %v", err)
	}

	hostUnavailable := mustReadEnvelope(t, ctx, viewerConn)
	if hostUnavailable.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state after host disconnect, got %s", hostUnavailable.Type)
	}
	var unavailableState protocol.RoomStatePayload
	if err := json.Unmarshal(hostUnavailable.Payload, &unavailableState); err != nil {
		t.Fatalf("unmarshal unavailable room_state: %v", err)
	}
	if unavailableState.HostUserID != "" {
		t.Fatalf("expected no online host, got %s", unavailableState.HostUserID)
	}

	reconnectedHost := mustDialWebSocket(t, ctx, wsURL)
	defer reconnectedHost.Close(websocket.StatusNormalClosure, "test done")
	mustJoinRoom(t, ctx, reconnectedHost, createdRoom.ID(), "user_a")

	rejoinStateEnvelope := mustReadEnvelope(t, ctx, reconnectedHost)
	if rejoinStateEnvelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on host reconnect, got %s", rejoinStateEnvelope.Type)
	}
	var rejoinState protocol.RoomStatePayload
	if err := json.Unmarshal(rejoinStateEnvelope.Payload, &rejoinState); err != nil {
		t.Fatalf("unmarshal host rejoin room_state: %v", err)
	}
	if rejoinState.HostUserID != "user_a" {
		t.Fatalf("expected original host to regain control, got %s", rejoinState.HostUserID)
	}

	rejoinBroadcast := mustReadEnvelope(t, ctx, viewerConn)
	if rejoinBroadcast.Type != protocol.TypeRoomState {
		t.Fatalf("expected room_state on viewer after host reconnect, got %s", rejoinBroadcast.Type)
	}
	var viewerRejoinState protocol.RoomStatePayload
	if err := json.Unmarshal(rejoinBroadcast.Payload, &viewerRejoinState); err != nil {
		t.Fatalf("unmarshal viewer rejoin room_state: %v", err)
	}
	if viewerRejoinState.HostUserID != "user_a" {
		t.Fatalf("expected viewer to see original host restored, got %s", viewerRejoinState.HostUserID)
	}
	membersChanged := mustReadEnvelope(t, ctx, viewerConn)
	if membersChanged.Type != protocol.TypeRoomMembersChanged {
		t.Fatalf("expected room_members_changed after host reconnect, got %s", membersChanged.Type)
	}

	mustSendEnvelope(t, ctx, reconnectedHost, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			PositionMs: 5_000,
			Seq:        rejoinState.Seq,
		}),
	})

	assertControlBroadcast(t, ctx, viewerConn, protocol.TypePlay, -1, rejoinState.Seq+1)
	assertControlBroadcast(t, ctx, reconnectedHost, protocol.TypePlay, -1, rejoinState.Seq+1)

	state := createdRoom.StateSnapshot()
	if state.HostUserID != "user_a" {
		t.Fatalf("expected original host to remain host, got %s", state.HostUserID)
	}
}

func mustDialWebSocket(t *testing.T, ctx context.Context, wsURL string, userIDs ...string) *websocket.Conn {
	t.Helper()
	userID := "user_a"
	if len(userIDs) > 0 && userIDs[0] != "" {
		userID = userIDs[0]
	}
	conn, _, err := websocket.Dial(ctx, wsURL, websocketDialOptions(userID))
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func websocketDialOptions(userID string) *websocket.DialOptions {
	return &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {testAuthorizationHeader(userID)},
		},
	}
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

func mustReadEnvelopeSkippingMembershipChanged(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
) protocol.Envelope {
	t.Helper()
	for {
		envelope := mustReadEnvelope(t, ctx, conn)
		if envelope.Type == protocol.TypeRoomMembersChanged {
			continue
		}
		return envelope
	}
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
	envelope := mustReadEnvelopeSkippingMembershipChanged(t, ctx, conn)
	if envelope.Type != expectedType {
		t.Fatalf("expected %s, got %s", expectedType, envelope.Type)
	}

	switch expectedType {
	case protocol.TypePlay:
		var payload protocol.PlayPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal play payload: %v", err)
		}
		if payload.ServerTimeMs == 0 || payload.Reason == "" {
			t.Fatalf("expected authoritative timeline fields in play payload: %+v", payload)
		}
		if (expectedPosition >= 0 && payload.PositionMs != expectedPosition) || payload.Seq != expectedSeq {
			t.Fatalf("unexpected play payload: %+v", payload)
		}
	case protocol.TypePause:
		var payload protocol.PausePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal pause payload: %v", err)
		}
		if payload.ServerTimeMs == 0 || payload.Reason == "" {
			t.Fatalf("expected authoritative timeline fields in pause payload: %+v", payload)
		}
		if (expectedPosition >= 0 && payload.PositionMs != expectedPosition) || payload.Seq != expectedSeq {
			t.Fatalf("unexpected pause payload: %+v", payload)
		}
	case protocol.TypeSeek:
		var payload protocol.SeekPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal seek payload: %v", err)
		}
		if payload.ServerTimeMs == 0 || payload.Reason == "" {
			t.Fatalf("expected authoritative timeline fields in seek payload: %+v", payload)
		}
		if (expectedPosition >= 0 && payload.PositionMs != expectedPosition) || payload.Seq != expectedSeq {
			t.Fatalf("unexpected seek payload: %+v", payload)
		}
	default:
		t.Fatalf("unsupported expected control type %s", expectedType)
	}
}

func assertPlaybackRateBroadcast(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	expectedPosition int64,
	expectedRate float64,
	expectedSeq int64,
) {
	t.Helper()
	envelope := mustReadEnvelopeSkippingMembershipChanged(t, ctx, conn)
	if envelope.Type != protocol.TypeSetPlaybackRate {
		t.Fatalf("expected %s, got %s", protocol.TypeSetPlaybackRate, envelope.Type)
	}

	var payload protocol.SetPlaybackRatePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal set_playback_rate payload: %v", err)
	}
	if payload.ServerTimeMs == 0 || payload.Reason == "" {
		t.Fatalf("expected authoritative timeline fields in set_playback_rate payload: %+v", payload)
	}
	if (expectedPosition >= 0 && payload.PositionMs != expectedPosition) ||
		payload.Seq != expectedSeq ||
		payload.PlaybackRate != expectedRate {
		t.Fatalf("unexpected set_playback_rate payload: %+v", payload)
	}
}

func assertEndedBroadcast(
	t *testing.T,
	ctx context.Context,
	conn *websocket.Conn,
	expectedPosition int64,
	expectedSeq int64,
) {
	t.Helper()
	envelope := mustReadEnvelopeSkippingMembershipChanged(t, ctx, conn)
	if envelope.Type != protocol.TypeEnded {
		t.Fatalf("expected %s, got %s", protocol.TypeEnded, envelope.Type)
	}

	var payload protocol.EndedPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal ended payload: %v", err)
	}
	if payload.ServerTimeMs == 0 || payload.Reason == "" {
		t.Fatalf("expected authoritative timeline fields in ended payload: %+v", payload)
	}
	if payload.PositionMs != expectedPosition || payload.Seq != expectedSeq {
		t.Fatalf("unexpected ended payload: %+v", payload)
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("condition not satisfied within %s", timeout)
}

func readUntilClosed(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()

	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}
