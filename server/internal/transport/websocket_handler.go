package transport

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
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
	defaultHeartbeatInterval    = 5 * time.Second
	defaultHeartbeatTimeout     = 15 * time.Second
	defaultBroadcastWorkerLimit = 64
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
	case protocol.TypeEnded:
		return h.handleEnded(ctx, client, envelope)
	case protocol.TypeHeartbeatAck:
		return h.handleHeartbeatAck(client, envelope)
	case protocol.TypeClockSyncPing:
		return h.handleClockSyncPing(ctx, client, envelope)
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

// handleClockSyncPing replies with server wall time without touching room state or storage.
func (h *WebSocketHandler) handleClockSyncPing(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeClockSyncPing(envelope)
	if err != nil {
		return err
	}
	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeClockSyncPong,
		Payload: mustJSONRaw(protocol.ClockSyncPongPayload{
			ServerTimeMs:      time.Now().UnixMilli(),
			ClientSendMonoMs: payload.ClientSendMonoMs,
		}),
	})
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

	if err := client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeRoomState,
		Payload: mustJSONRaw(roomStatePayload(joinResult.State)),
	}); err != nil {
		return err
	}
	if joinResult.MembershipChanged {
		h.broadcastRoomMembersChangedToOthers(payload.RoomID, joinResult.Clients, client, "join")
	}
	return nil
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
	h.logControlReceived(protocol.TypePlay, payload.RoomID, payload.UserID, payload.PositionMs, payload.Seq, 0)
	return h.handleControlEvent(
		ctx,
		protocol.TypePlay,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlay(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypePlay, state)
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
	h.logControlReceived(protocol.TypePause, payload.RoomID, payload.UserID, payload.PositionMs, payload.Seq, 0)
	return h.handleControlEvent(
		ctx,
		protocol.TypePause,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPause(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypePause, state)
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
	h.logControlReceived(protocol.TypeSeek, payload.RoomID, payload.UserID, payload.PositionMs, payload.Seq, 0)
	return h.handleControlEvent(
		ctx,
		protocol.TypeSeek,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplySeek(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeSeek, state)
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
	h.logControlReceived(
		protocol.TypeSetPlaybackRate,
		payload.RoomID,
		payload.UserID,
		payload.PositionMs,
		payload.Seq,
		payload.PlaybackRate,
	)
	return h.handleControlEvent(
		ctx,
		protocol.TypeSetPlaybackRate,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlaybackRate(payload.UserID, payload.PlaybackRate)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeSetPlaybackRate, state)
		},
	)
}

// handleEnded validates one ended control event and broadcasts the authoritative completed state.
func (h *WebSocketHandler) handleEnded(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeEnded(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(time.Now())
	h.logControlReceived(protocol.TypeEnded, payload.RoomID, payload.UserID, payload.PositionMs, payload.Seq, 0)
	return h.handleControlEvent(
		ctx,
		protocol.TypeEnded,
		payload.RoomID,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyEnded(payload.UserID, payload.PositionMs)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeEnded, state)
		},
	)
}

func (h *WebSocketHandler) handleControlEvent(
	ctx context.Context,
	eventType string,
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

	envelope := buildEnvelope(state)
	stats, err := broadcastEnvelopeWithStats(ctx, clients, envelope)
	if h.debugSync {
		log.Printf(
			"sync broadcast type=%s room=%s seq=%d clients=%d pos=%d paused=%t rate=%.2f duration_us=%d slowest_user=%s slowest_us=%d err=%v",
			eventType,
			roomID,
			state.Seq,
			stats.Clients,
			state.PositionMs,
			state.Paused,
			state.PlaybackRate,
			stats.Duration.Microseconds(),
			stats.SlowestUserID,
			stats.SlowestDuration.Microseconds(),
			err,
		)
	}
	return err
}

func (h *WebSocketHandler) logControlReceived(
	eventType string,
	roomID string,
	userID string,
	positionMs int64,
	seq int64,
	playbackRate float64,
) {
	if !h.debugSync {
		return
	}
	if playbackRate > 0 {
		log.Printf(
			"sync control received type=%s room=%s user=%s pos=%d seq=%d rate=%.2f",
			eventType,
			roomID,
			userID,
			positionMs,
			seq,
			playbackRate,
		)
		return
	}
	log.Printf(
		"sync control received type=%s room=%s user=%s pos=%d seq=%d",
		eventType,
		roomID,
		userID,
		positionMs,
		seq,
	)
}

type broadcastStats struct {
	Clients         int
	Duration        time.Duration
	SlowestUserID   string
	SlowestDuration time.Duration
}

func broadcastEnvelopeWithStats(
	ctx context.Context,
	clients []*room.ClientConnection,
	envelope protocol.Envelope,
) (broadcastStats, error) {
	startedAt := time.Now()
	stats := broadcastStats{}
	targets := make([]*room.ClientConnection, 0, len(clients))
	for _, client := range clients {
		if client == nil {
			continue
		}
		targets = append(targets, client)
	}
	stats.Clients = len(targets)
	if len(targets) == 0 {
		stats.Duration = time.Since(startedAt)
		return stats, nil
	}

	workerCount := len(targets)
	if workerCount > defaultBroadcastWorkerLimit {
		workerCount = defaultBroadcastWorkerLimit
	}

	jobs := make(chan *room.ClientConnection)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for client := range jobs {
				clientStartedAt := time.Now()
				err := client.WriteJSON(ctx, envelope)
				clientDuration := time.Since(clientStartedAt)

				mu.Lock()
				if clientDuration > stats.SlowestDuration {
					stats.SlowestDuration = clientDuration
					stats.SlowestUserID = client.UserID()
				}
				if err != nil && firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}

	for _, client := range targets {
		jobs <- client
	}
	close(jobs)

	wg.Wait()
	stats.Duration = time.Since(startedAt)
	return stats, firstErr
}

func broadcastEnvelope(
	ctx context.Context,
	clients []*room.ClientConnection,
	envelope protocol.Envelope,
) error {
	_, err := broadcastEnvelopeWithStats(ctx, clients, envelope)
	return err
}

// broadcastRoomState pushes the latest authority snapshot after membership changes such
// as host transfer. Disconnect cleanup uses a fresh context because the request context
// may already be canceled when the websocket loop exits.
func (h *WebSocketHandler) broadcastRoomState(result room.RemoveClientResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = broadcastEnvelope(ctx, result.Remaining, protocol.Envelope{
		Type: protocol.TypeRoomState,
		Payload: mustJSONRaw(roomStatePayload(result.State)),
	})
}

func roomStatePayload(state room.State) protocol.RoomStatePayload {
	return protocol.RoomStatePayload{
		RoomID:       state.RoomID,
		MediaID:      state.MediaID,
		HostUserID:   state.HostUserID,
		Paused:       state.Paused,
		Ended:        state.Ended,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		PlaybackRate: state.PlaybackRate,
		Seq:          state.Seq,
	}
}

func controlEnvelopeFromState(eventType string, state room.State) protocol.Envelope {
	switch eventType {
	case protocol.TypePlay:
		return protocol.Envelope{
			Type:    protocol.TypePlay,
			Payload: mustJSONRaw(playPayloadFromState(state)),
		}
	case protocol.TypePause:
		return protocol.Envelope{
			Type:    protocol.TypePause,
			Payload: mustJSONRaw(pausePayloadFromState(state)),
		}
	case protocol.TypeSeek:
		return protocol.Envelope{
			Type:    protocol.TypeSeek,
			Payload: mustJSONRaw(seekPayloadFromState(state)),
		}
	case protocol.TypeSetPlaybackRate:
		return protocol.Envelope{
			Type:    protocol.TypeSetPlaybackRate,
			Payload: mustJSONRaw(setPlaybackRatePayloadFromState(state)),
		}
	case protocol.TypeEnded:
		return protocol.Envelope{
			Type:    protocol.TypeEnded,
			Payload: mustJSONRaw(endedPayloadFromState(state)),
		}
	default:
		return protocol.Envelope{
			Type:    protocol.TypeRoomState,
			Payload: mustJSONRaw(roomStatePayload(state)),
		}
	}
}

func playPayloadFromState(state room.State) protocol.PlayPayload {
	return protocol.PlayPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs:  state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func pausePayloadFromState(state room.State) protocol.PausePayload {
	return protocol.PausePayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs:  state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func seekPayloadFromState(state room.State) protocol.SeekPayload {
	return protocol.SeekPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs:  state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func setPlaybackRatePayloadFromState(state room.State) protocol.SetPlaybackRatePayload {
	return protocol.SetPlaybackRatePayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs:  state.ServerTimeMs,
		Reason:       state.Reason,
		PlaybackRate: state.PlaybackRate,
		Seq:          state.Seq,
	}
}

func endedPayloadFromState(state room.State) protocol.EndedPayload {
	return protocol.EndedPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs:  state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func (h *WebSocketHandler) broadcastRoomMembersChangedToOthers(
	roomID string,
	clients []*room.ClientConnection,
	exclude *room.ClientConnection,
	reason string,
) {
	if roomID == "" || len(clients) == 0 {
		return
	}
	targets := make([]*room.ClientConnection, 0, len(clients))
	for _, client := range clients {
		if client == exclude {
			continue
		}
		targets = append(targets, client)
	}
	if len(targets) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = broadcastEnvelope(ctx, targets, protocol.Envelope{
		Type: protocol.TypeRoomMembersChanged,
		Payload: mustJSONRaw(protocol.RoomMembersChangedPayload{
			RoomID: roomID,
			Reason: reason,
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
