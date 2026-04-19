package transport

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

type WebSocketHandler struct {
	roomManager       *room.Manager
	debugSync         bool
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
}

const (
	defaultHeartbeatInterval = 5 * time.Second
	defaultHeartbeatTimeout  = 15 * time.Second
)

type protocolMessageError struct {
	roomID  string
	message string
}

func (e protocolMessageError) Error() string {
	return e.message
}

// NewWebSocketHandler builds the /ws entrypoint around the shared room manager.
func NewWebSocketHandler(roomManager *room.Manager, debugSync bool) *WebSocketHandler {
	return newWebSocketHandler(roomManager, debugSync, defaultHeartbeatInterval, defaultHeartbeatTimeout)
}

func newWebSocketHandler(
	roomManager *room.Manager,
	debugSync bool,
	heartbeatInterval time.Duration,
	heartbeatTimeout time.Duration,
) *WebSocketHandler {
	return &WebSocketHandler{
		roomManager:       roomManager,
		debugSync:         debugSync,
		heartbeatInterval: heartbeatInterval,
		heartbeatTimeout:  heartbeatTimeout,
	}
}

// ServeHTTP upgrades the request to WebSocket and keeps reading protocol messages
// until the client disconnects or a read error occurs.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}

	client := room.NewClientConnection(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		// Connection cleanup always flows through the room manager so empty rooms can be removed.
		removeResult := h.roomManager.RemoveClient(client)
		if removeResult.HostTransferred {
			h.broadcastRoomState(removeResult)
		}
		_ = client.Close(websocket.StatusNormalClosure, "connection closed")
	}()
	go h.runHeartbeatLoop(ctx, client)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if h.debugSync && !errors.Is(err, context.Canceled) {
				log.Printf("websocket read stopped: %v", err)
			}
			return
		}

		if err := h.handleMessage(ctx, client, data); err != nil {
			if h.debugSync {
				log.Printf("protocol handling failed: %v", err)
			}
			roomID := client.RoomID()
			message := err.Error()
			var protocolErr protocolMessageError
			if errors.As(err, &protocolErr) {
				roomID = protocolErr.roomID
				message = protocolErr.message
			}
			// Protocol errors are sent back as minimal server-side error events instead of
			// immediately dropping the connection.
			_ = client.WriteJSON(ctx, protocol.ErrorEnvelope{
				Type: protocol.TypeError,
				Payload: protocol.ErrorPayload{
					RoomID:  roomID,
					Message: message,
				},
			})
		}
	}
}

// handleMessage routes one decoded envelope to the matching protocol handler.
func (h *WebSocketHandler) handleMessage(
	ctx context.Context,
	client *room.ClientConnection,
	data []byte,
) error {
	envelope, err := protocol.DecodeEnvelope(data)
	if err != nil {
		return err
	}

	switch envelope.Type {
	case protocol.TypeJoinRoom:
		return h.handleJoinRoom(ctx, client, envelope)
	case protocol.TypePlay:
		return h.handlePlay(ctx, client, envelope)
	case protocol.TypePause:
		return h.handlePause(ctx, client, envelope)
	case protocol.TypeSeek:
		return h.handleSeek(ctx, client, envelope)
	case protocol.TypeSetPlaybackRate:
		return h.handleSetPlaybackRate(ctx, client, envelope)
	case protocol.TypeHeartbeatAck:
		return h.handleHeartbeatAck(client, envelope)
	default:
		return protocol.ErrUnsupportedMessageType
	}
}

// handleHeartbeatAck refreshes the last known healthy timestamp for one connection.
func (h *WebSocketHandler) handleHeartbeatAck(
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	_, err := protocol.DecodeHeartbeatAck(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	return nil
}

// handleJoinRoom attaches the client to an existing room and returns the current
// room_state snapshot expected by the protocol draft.
func (h *WebSocketHandler) handleJoinRoom(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeJoinRoom(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())

	existingRoom, ok := h.roomManager.Get(payload.RoomID)
	if !ok {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room not found",
		}
	}

	// We persist identity on the connection first so disconnect cleanup can find the room later.
	client.SetIdentity(payload.UserID, payload.RoomID)
	joinResult := existingRoom.Join(client)
	h.roomManager.MarkRoomActive(payload.RoomID)
	if joinResult.ReplacedClient != nil {
		// Repeated join for the same logical user should leave only one active connection.
		_ = joinResult.ReplacedClient.CloseNow()
	}

	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeRoomState,
		Payload: mustJSONRaw(protocol.RoomStatePayload{
			RoomID:       payload.RoomID,
			MediaID:      joinResult.State.MediaID,
			HostUserID:   joinResult.State.HostUserID,
			Paused:       joinResult.State.Paused,
			PositionMs:   joinResult.State.PositionMs,
			PlaybackRate: joinResult.State.PlaybackRate,
			Seq:          joinResult.State.Seq,
		}),
	})
}

// handlePlay validates one play control event, applies it to the room authority state,
// and broadcasts the authoritative play payload to all joined clients.
func (h *WebSocketHandler) handlePlay(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodePlay(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	return h.handleControlEvent(
		ctx,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlay(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return protocol.Envelope{
				Type: protocol.TypePlay,
				Payload: mustJSONRaw(protocol.PlayPayload{
					RoomID:     state.RoomID,
					UserID:     state.HostUserID,
					PositionMs: state.PositionMs,
					Seq:        state.Seq,
				}),
			}
		},
	)
}

// handlePause validates one pause control event and broadcasts the authoritative pause.
func (h *WebSocketHandler) handlePause(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodePause(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	return h.handleControlEvent(
		ctx,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPause(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return protocol.Envelope{
				Type: protocol.TypePause,
				Payload: mustJSONRaw(protocol.PausePayload{
					RoomID:     state.RoomID,
					UserID:     state.HostUserID,
					PositionMs: state.PositionMs,
					Seq:        state.Seq,
				}),
			}
		},
	)
}

// handleSeek validates one seek control event and broadcasts the authoritative seek.
func (h *WebSocketHandler) handleSeek(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeSeek(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	return h.handleControlEvent(
		ctx,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplySeek(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return protocol.Envelope{
				Type: protocol.TypeSeek,
				Payload: mustJSONRaw(protocol.SeekPayload{
					RoomID:     state.RoomID,
					UserID:     state.HostUserID,
					PositionMs: state.PositionMs,
					Seq:        state.Seq,
				}),
			}
		},
	)
}

// handleSetPlaybackRate validates one playback-rate control event and broadcasts the authoritative rate.
func (h *WebSocketHandler) handleSetPlaybackRate(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeSetPlaybackRate(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	return h.handleControlEvent(
		ctx,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlaybackRate(payload.UserID, payload.PlaybackRate)
		},
		func(state room.State) protocol.Envelope {
			return protocol.Envelope{
				Type: protocol.TypeSetPlaybackRate,
				Payload: mustJSONRaw(protocol.SetPlaybackRatePayload{
					RoomID:       state.RoomID,
					UserID:       state.HostUserID,
					PositionMs:   state.PositionMs,
					PlaybackRate: state.PlaybackRate,
					Seq:          state.Seq,
				}),
			}
		},
	)
}

func (h *WebSocketHandler) handleControlEvent(
	ctx context.Context,
	roomID string,
	apply func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error),
	buildEnvelope func(state room.State) protocol.Envelope,
) error {
	existingRoom, ok := h.roomManager.Get(roomID)
	if !ok {
		return protocolMessageError{
			roomID:  roomID,
			message: "room not found",
		}
	}

	state, clients, err := apply(existingRoom)
	if err != nil {
		if errors.Is(err, room.ErrNotHost) {
			return protocolMessageError{
				roomID:  roomID,
				message: "only host can control playback",
			}
		}
		return err
	}

	return broadcastEnvelope(ctx, clients, buildEnvelope(state))
}

func broadcastEnvelope(
	ctx context.Context,
	clients []*room.ClientConnection,
	envelope protocol.Envelope,
) error {
	for _, client := range clients {
		if err := client.WriteJSON(ctx, envelope); err != nil {
			return err
		}
	}
	return nil
}

// broadcastRoomState pushes the latest authority snapshot after membership changes such
// as host transfer. Disconnect cleanup uses a fresh context because the request context
// may already be canceled when the websocket loop exits.
func (h *WebSocketHandler) broadcastRoomState(result room.RemoveClientResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = broadcastEnvelope(ctx, result.Remaining, protocol.Envelope{
		Type: protocol.TypeRoomState,
		Payload: mustJSONRaw(protocol.RoomStatePayload{
			RoomID:       result.State.RoomID,
			MediaID:      result.State.MediaID,
			HostUserID:   result.State.HostUserID,
			Paused:       result.State.Paused,
			PositionMs:   result.State.PositionMs,
			PlaybackRate: result.State.PlaybackRate,
			Seq:          result.State.Seq,
		}),
	})
}

// runHeartbeatLoop keeps every websocket connection on a simple liveness contract:
// the server emits heartbeat events and closes the socket if the client stops acking.
func (h *WebSocketHandler) runHeartbeatLoop(ctx context.Context, client *room.ClientConnection) {
	ticker := time.NewTicker(h.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			if client.HeartbeatTimedOut(now, h.heartbeatTimeout) {
				_ = client.Close(websocket.StatusPolicyViolation, "heartbeat timeout")
				return
			}

			client.MarkHeartbeatSent(now)
			if err := client.WriteJSON(ctx, protocol.Envelope{
				Type: protocol.TypeHeartbeat,
				Payload: mustJSONRaw(protocol.HeartbeatPayload{
					ServerTimeMs: now.UnixMilli(),
				}),
			}); err != nil {
				if h.debugSync && !errors.Is(err, context.Canceled) {
					log.Printf("heartbeat write stopped: %v", err)
				}
				_ = client.Close(websocket.StatusPolicyViolation, "heartbeat write failed")
				return
			}
		}
	}
}

// mustJSONRaw is used for small internal protocol responses that should never fail to marshal.
func mustJSONRaw(value any) []byte {
	data, err := protocolMarshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
