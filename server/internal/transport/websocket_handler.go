package transport

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/semaphore"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/realtime"
	"watch_together/server/internal/room"
)

type WebSocketHandler struct {
	roomManager         *room.Manager
	debugSync           bool
	heartbeatInterval   time.Duration
	heartbeatTimeout    time.Duration
	clock               realtime.Clock
	broadcaster         roomBroadcaster
	clientOptions       room.ClientConnectionOptions
	connectionLimit     *semaphore.Weighted
	maxRoomClients      int
	roomStateWriter     latestRoomStateWriter
	roomLeaver          roomMembershipLeaver
	controlDeduper      *controlRequestDeduper
	seekRateLimiter     *controlRateLimiter
	tokenVerifier       accessTokenVerifier
	deviceSwitches      *roomDeviceSwitchRegistry
	deviceSwitchTimeout time.Duration
}

const (
	defaultHeartbeatInterval         = 5 * time.Second
	defaultHeartbeatTimeout          = 15 * time.Second
	defaultBroadcastConcurrencyLimit = int64(64)
	defaultBroadcastTimeout          = 5 * time.Second
	defaultBroadcastEnqueueTimeout   = 3 * time.Second
	defaultControlRequestDedupTTL    = 2 * time.Minute
	defaultSeekMinInterval           = 250 * time.Millisecond
)

type WebSocketRuntimeConfig struct {
	BroadcastConcurrencyLimit int64
	BroadcastTimeout          time.Duration
	BroadcastEnqueueTimeout   time.Duration
	ClientOutboxCapacity      int
	MaxConnections            int64
	MaxRoomClients            int
	RoomDeviceSwitchTimeout   time.Duration
	SeekMinInterval           time.Duration
}

type latestRoomStateWriter interface {
	SetRoomState(ctx context.Context, roomID string, state protocol.RoomStatePayload) error
}

type roomMembershipLeaver interface {
	LeaveRoomByCode(ctx context.Context, roomCode string, userID string) error
}

type protocolMessageError struct {
	roomID  string
	message string
}

func (e protocolMessageError) Error() string {
	return e.message
}

// NewWebSocketHandler builds the /ws entrypoint around the shared room manager.
func NewWebSocketHandler(roomManager *room.Manager, debugSync bool) *WebSocketHandler {
	return NewWebSocketHandlerWithConfig(roomManager, debugSync, WebSocketRuntimeConfig{})
}

func NewWebSocketHandlerWithConfig(
	roomManager *room.Manager,
	debugSync bool,
	config WebSocketRuntimeConfig,
) *WebSocketHandler {
	return newWebSocketHandler(roomManager, debugSync, defaultHeartbeatInterval, defaultHeartbeatTimeout, config)
}

func NewWebSocketHandlerWithConfigAndRoomStateWriter(
	roomManager *room.Manager,
	debugSync bool,
	config WebSocketRuntimeConfig,
	roomStateWriter latestRoomStateWriter,
) *WebSocketHandler {
	return NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifier(
		roomManager,
		debugSync,
		config,
		roomStateWriter,
		nil,
	)
}

func NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifier(
	roomManager *room.Manager,
	debugSync bool,
	config WebSocketRuntimeConfig,
	roomStateWriter latestRoomStateWriter,
	tokenVerifier accessTokenVerifier,
) *WebSocketHandler {
	return NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifierAndRoomLeaver(
		roomManager,
		debugSync,
		config,
		roomStateWriter,
		tokenVerifier,
		nil,
	)
}

func NewWebSocketHandlerWithConfigAndRoomStateWriterAndTokenVerifierAndRoomLeaver(
	roomManager *room.Manager,
	debugSync bool,
	config WebSocketRuntimeConfig,
	roomStateWriter latestRoomStateWriter,
	tokenVerifier accessTokenVerifier,
	roomLeaver roomMembershipLeaver,
) *WebSocketHandler {
	handler := newWebSocketHandler(roomManager, debugSync, defaultHeartbeatInterval, defaultHeartbeatTimeout, config)
	handler.roomStateWriter = roomStateWriter
	handler.tokenVerifier = tokenVerifier
	handler.roomLeaver = roomLeaver
	return handler
}

func newWebSocketHandler(
	roomManager *room.Manager,
	debugSync bool,
	heartbeatInterval time.Duration,
	heartbeatTimeout time.Duration,
	configs ...WebSocketRuntimeConfig,
) *WebSocketHandler {
	config := WebSocketRuntimeConfig{}
	if len(configs) > 0 {
		config = configs[0]
	}
	return newWebSocketHandlerWithClock(
		roomManager,
		debugSync,
		heartbeatInterval,
		heartbeatTimeout,
		realtime.SystemClock{},
		config,
	)
}

func newWebSocketHandlerWithClock(
	roomManager *room.Manager,
	debugSync bool,
	heartbeatInterval time.Duration,
	heartbeatTimeout time.Duration,
	clock realtime.Clock,
	configs ...WebSocketRuntimeConfig,
) *WebSocketHandler {
	if clock == nil {
		clock = realtime.SystemClock{}
	}
	config := WebSocketRuntimeConfig{}
	if len(configs) > 0 {
		config = configs[0]
	}
	config = normalizeWebSocketRuntimeConfig(config)
	var connectionLimit *semaphore.Weighted
	if config.MaxConnections > 0 {
		connectionLimit = semaphore.NewWeighted(config.MaxConnections)
	}
	return &WebSocketHandler{
		roomManager:       roomManager,
		debugSync:         debugSync,
		heartbeatInterval: heartbeatInterval,
		heartbeatTimeout:  heartbeatTimeout,
		clock:             clock,
		broadcaster:       newBoundedBroadcaster(broadcastConfigFromWebSocketConfig(config)),
		clientOptions: room.ClientConnectionOptions{
			OutboxCapacity: config.ClientOutboxCapacity,
		},
		connectionLimit: connectionLimit,
		maxRoomClients:  config.MaxRoomClients,
		controlDeduper: newControlRequestDeduper(
			defaultControlRequestDedupTTL,
			defaultControlRequestDedupMaxEntries,
			defaultControlRequestDedupShards,
		),
		seekRateLimiter:     newControlRateLimiter(config.SeekMinInterval, defaultControlRateLimitMaxEntries, defaultControlRateLimitShards),
		tokenVerifier:       defaultAccessTokenVerifier,
		deviceSwitches:      newRoomDeviceSwitchRegistry(),
		deviceSwitchTimeout: config.RoomDeviceSwitchTimeout,
	}
}

func normalizeWebSocketRuntimeConfig(config WebSocketRuntimeConfig) WebSocketRuntimeConfig {
	if config.BroadcastConcurrencyLimit <= 0 {
		config.BroadcastConcurrencyLimit = defaultBroadcastConcurrencyLimit
	}
	if config.BroadcastTimeout <= 0 {
		config.BroadcastTimeout = defaultBroadcastTimeout
	}
	if config.BroadcastEnqueueTimeout <= 0 {
		config.BroadcastEnqueueTimeout = defaultBroadcastEnqueueTimeout
	}
	if config.ClientOutboxCapacity <= 0 {
		config.ClientOutboxCapacity = room.DefaultClientOutboxCapacity()
	}
	if config.RoomDeviceSwitchTimeout <= 0 {
		config.RoomDeviceSwitchTimeout = defaultRoomDeviceSwitchTimeout
	}
	if config.SeekMinInterval == 0 {
		config.SeekMinInterval = defaultSeekMinInterval
	}
	return config
}

func broadcastConfigFromWebSocketConfig(config WebSocketRuntimeConfig) broadcastConfig {
	return broadcastConfig{
		ConcurrencyLimit:      config.BroadcastConcurrencyLimit,
		BroadcastTimeout:      config.BroadcastTimeout,
		EnqueueTimeout:        config.BroadcastEnqueueTimeout,
		CloseOnEnqueueTimeout: true,
	}
}

// ServeHTTP upgrades the request to WebSocket and keeps reading protocol messages
// until the client disconnects or a read error occurs.
func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	authUserID, ok := h.authenticateRequest(r)
	if !ok {
		http.Error(w, "missing or invalid access token", http.StatusUnauthorized)
		return
	}

	if h.connectionLimit != nil {
		if !h.connectionLimit.TryAcquire(1) {
			http.Error(w, "websocket connection limit reached", http.StatusServiceUnavailable)
			return
		}
		defer h.connectionLimit.Release(1)
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}

	client := room.NewClientConnectionWithOptions(conn, h.clientOptions)
	client.SetIdentity(authUserID, "")
	ctx, cancel := context.WithCancel(context.Background())
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		if err := client.RunWriteLoop(ctx); err != nil && h.debugSync && !errors.Is(err, context.Canceled) {
			log.Printf("websocket write loop stopped: %v", err)
		}
	}()
	defer func() {
		cancel()
		select {
		case <-writerDone:
		case <-time.After(time.Second):
			_ = client.CloseNow()
		}
		// Connection cleanup always flows through the room manager so empty rooms can be removed.
		removeResult := h.roomManager.RemoveClient(client)
		if removeResult.HostUnavailable {
			h.broadcastRoomState(removeResult)
		}
		h.cancelRoomDeviceSwitchesForClient(client, "active_disconnected")
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

func (h *WebSocketHandler) authenticateRequest(r *http.Request) (string, bool) {
	token, ok := bearerTokenFromRequestHeader(r.Header.Get("Authorization"))
	if !ok {
		return "", false
	}
	verifier := h.tokenVerifier
	if verifier == nil {
		verifier = defaultAccessTokenVerifier
	}
	claims, err := verifier.VerifyAccessToken(token)
	if err != nil {
		return "", false
	}
	if claims.UserID == "" {
		return "", false
	}
	return claims.UserID, true
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
	case protocol.TypeLeaveRoom:
		return h.handleLeaveRoom(ctx, client, envelope)
	case protocol.TypeRoomStateRequest:
		return h.handleRoomStateRequest(ctx, client, envelope)
	case protocol.TypeRoomDeviceSwitchReply:
		return h.handleRoomDeviceSwitchReply(ctx, client, envelope)
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

// handleLeaveRoom handles an intentional room leave. It differs from disconnect cleanup:
// empty rooms are destroyed immediately, while transient disconnects keep the grace period.
func (h *WebSocketHandler) handleLeaveRoom(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeLeaveRoom(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(h.clock.Now())
	if client.RoomID() != payload.RoomID || client.UserID() == "" || client.UserID() != payload.UserID {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room identity mismatch",
		}
	}
	if existingRoom, ok := h.roomManager.Get(payload.RoomID); ok && !existingRoom.IsActiveClient(client) {
		h.cancelRoomDeviceSwitchesForClient(client, "left pending room device switch")
		client.SetIdentity("", "")
		return client.Close(websocket.StatusNormalClosure, "left pending room device switch")
	}

	h.persistRoomLeave(payload.RoomID, payload.UserID)
	leaveResult := h.roomManager.LeaveClient(client)
	client.SetIdentity("", "")
	if leaveResult.HostUnavailable {
		h.broadcastRoomState(leaveResult)
	}
	if !leaveResult.RoomRemoved {
		h.broadcastRoomMembersChangedToOthers(payload.RoomID, leaveResult.Remaining, client, "leave")
	}
	return client.Close(websocket.StatusNormalClosure, "left room")
}

func (h *WebSocketHandler) persistRoomLeave(roomID string, userID string) {
	if h.roomLeaver == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := h.roomLeaver.LeaveRoomByCode(ctx, roomID, userID); err != nil && h.debugSync {
		log.Printf("room leave persistence failed room=%s user=%s err=%v", roomID, userID, err)
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
	client.MarkHeartbeatAck(h.clock.Now())
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
	serverTimeMs := h.clock.NowUnixMilli()
	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeClockSyncPong,
		Payload: mustJSONRaw(protocol.ClockSyncPongPayload{
			ServerTimeMs:     serverTimeMs,
			ClientSendMonoMs: payload.ClientSendMonoMs,
		}),
	})
}

// handleRoomStateRequest lets joined clients explicitly recover the latest room snapshot
// without reconnecting. The snapshot is read from the authoritative in-process room state.
func (h *WebSocketHandler) handleRoomStateRequest(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeRoomStateRequest(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(h.clock.Now())
	if client.RoomID() != payload.RoomID || client.UserID() == "" || client.UserID() != payload.UserID {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room identity mismatch",
		}
	}

	existingRoom, ok := h.roomManager.Get(payload.RoomID)
	if !ok {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room not found",
		}
	}
	if !existingRoom.IsActiveClient(client) {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "active room device required",
		}
	}

	state := existingRoom.StateSnapshot()
	if h.debugSync {
		log.Printf(
			"sync room_state_request room=%s user=%s client_seq=%d server_seq=%d",
			payload.RoomID,
			payload.UserID,
			payload.Seq,
			state.Seq,
		)
	}
	h.cacheRoomState(state)
	return h.writeRoomState(ctx, client, state)
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
	client.MarkHeartbeatAck(h.clock.Now())
	authUserID := client.UserID()
	if authUserID == "" || payload.UserID != authUserID {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room identity mismatch",
		}
	}

	existingRoom, ok := h.roomManager.Get(payload.RoomID)
	if !ok {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room not found",
		}
	}

	// We persist identity on the connection first so disconnect cleanup can find the room later.
	client.SetIdentity(authUserID, payload.RoomID)
	if activeLookup, ok := h.roomManager.ActiveClientForUser(authUserID); ok && activeLookup.Client != nil && activeLookup.Client != client {
		switchRequest, created := h.deviceSwitches.create(
			authUserID,
			payload.RoomID,
			activeLookup.RoomID,
			activeLookup.Client,
			client,
			h.clock.Now().Add(h.deviceSwitchTimeout),
		)
		if !created {
			client.SetIdentity("", "")
			return protocolMessageError{
				roomID:  payload.RoomID,
				message: "device switch already pending",
			}
		}
		if err := h.writeRoomDeviceWaiting(ctx, client, switchRequest); err != nil {
			h.deviceSwitches.take(switchRequest.RequestID)
			client.SetIdentity("", "")
			return err
		}
		if err := h.notifyRoomDeviceSwitchRequest(ctx, activeLookup.Client, switchRequest); err != nil {
			h.deviceSwitches.take(switchRequest.RequestID)
			_ = h.writeRoomDeviceSwitchResult(context.Background(), client, switchRequest, false, "switch request delivery failed")
			client.SetIdentity("", "")
			_ = client.Close(websocket.StatusPolicyViolation, "device switch request delivery failed")
			return err
		}
		h.scheduleRoomDeviceSwitchTimeout(switchRequest)
		return nil
	}
	joinResult := existingRoom.JoinWithLimit(client, h.maxRoomClients)
	if joinResult.Err != nil {
		client.SetIdentity("", "")
		if errors.Is(joinResult.Err, room.ErrRoomFull) {
			return protocolMessageError{
				roomID:  payload.RoomID,
				message: "room is full",
			}
		}
		return joinResult.Err
	}
	h.roomManager.MarkRoomActive(payload.RoomID)
	if joinResult.ReplacedClient != nil {
		// Repeated join for the same logical user should leave only one active connection.
		_ = joinResult.ReplacedClient.CloseNow()
	}

	if err := h.writeRoomState(ctx, client, joinResult.State); err != nil {
		return err
	}
	h.cacheRoomState(joinResult.State)
	if joinResult.HostReclaimed {
		h.broadcastRoomState(room.RemoveClientResult{
			State:     joinResult.State,
			Remaining: clientsWithout(joinResult.Clients, client),
		})
	}
	if joinResult.MembershipChanged {
		h.broadcastRoomMembersChangedToOthers(payload.RoomID, joinResult.Clients, client, "join")
	}
	return nil
}

func (h *WebSocketHandler) handleRoomDeviceSwitchReply(
	ctx context.Context,
	client *room.ClientConnection,
	envelope protocol.Envelope,
) error {
	payload, err := protocol.DecodeRoomDeviceSwitchReply(envelope)
	if err != nil {
		return err
	}
	client.MarkHeartbeatAck(h.clock.Now())
	if client.RoomID() != payload.RoomID || client.UserID() == "" || client.UserID() != payload.UserID {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "room identity mismatch",
		}
	}

	pending, ok := h.deviceSwitches.get(payload.RequestID)
	if !ok || pending.ActiveRoomID != payload.RoomID || pending.UserID != payload.UserID {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "device switch not found",
		}
	}
	if pending.ActiveClient != client {
		return protocolMessageError{
			roomID:  payload.RoomID,
			message: "device switch reply from inactive client",
		}
	}
	if h.clock.Now().After(pending.ExpiresAt) {
		h.deviceSwitches.take(payload.RequestID)
		_ = h.writeRoomDeviceSwitchResult(ctx, pending.PendingClient, pending, false, "device switch expired")
		_ = pending.PendingClient.Close(websocket.StatusPolicyViolation, "device switch expired")
		return nil
	}
	if !payload.Approve {
		h.deviceSwitches.take(payload.RequestID)
		_ = h.writeRoomDeviceSwitchResult(ctx, pending.PendingClient, pending, false, "device switch rejected")
		_ = h.writeRoomDeviceSwitchResult(ctx, pending.ActiveClient, pending, false, "device switch rejected")
		_ = pending.PendingClient.Close(websocket.StatusPolicyViolation, "device switch rejected")
		return nil
	}

	switchResult := h.roomManager.SwitchActiveClient(pending.TargetRoomID, pending.ActiveClient, pending.PendingClient, h.maxRoomClients)
	if switchResult.Err != nil {
		h.deviceSwitches.take(payload.RequestID)
		_ = h.writeRoomDeviceSwitchResult(ctx, pending.PendingClient, pending, false, switchResult.Err.Error())
		_ = pending.PendingClient.Close(websocket.StatusPolicyViolation, switchResult.Err.Error())
		return switchResult.Err
	}
	h.deviceSwitches.take(payload.RequestID)
	h.cacheRoomState(switchResult.State)
	h.roomManager.MarkRoomActive(pending.TargetRoomID)
	_ = h.writeRoomDeviceSwitchResult(ctx, pending.PendingClient, pending, true, "")
	_ = h.writeRoomDeviceSwitchResult(ctx, pending.ActiveClient, pending, true, "")
	_ = h.writeRoomState(ctx, pending.PendingClient, switchResult.State)
	if switchResult.CrossRoom && switchResult.PreviousRoomID != "" {
		h.broadcastRoomState(room.RemoveClientResult{
			State:           switchResult.PreviousRoomState,
			Remaining:       switchResult.PreviousRoomRemaining,
			HostUnavailable: switchResult.PreviousHostUnavailable,
		})
		if len(switchResult.PreviousRoomRemaining) > 0 {
			h.broadcastRoomMembersChangedToOthers(
				switchResult.PreviousRoomID,
				switchResult.PreviousRoomRemaining,
				pending.ActiveClient,
				"leave",
			)
		}
	}
	if switchResult.MembershipChanged {
		h.broadcastRoomMembersChangedToOthers(
			pending.TargetRoomID,
			switchResult.Clients,
			pending.ActiveClient,
			"join",
		)
	}
	_ = pending.ActiveClient.Close(websocket.StatusNormalClosure, "device switched")
	return nil
}

func (h *WebSocketHandler) writeRoomDeviceWaiting(
	ctx context.Context,
	client *room.ClientConnection,
	pending roomDeviceSwitch,
) error {
	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeRoomDeviceWaiting,
		Payload: mustJSONRaw(protocol.RoomDeviceSwitchRequestPayload{
			RoomID:       pending.TargetRoomID,
			TargetRoomID: pending.TargetRoomID,
			UserID:       pending.UserID,
			RequestID:    pending.RequestID,
			ExpiresAtMs:  pending.ExpiresAt.UnixMilli(),
		}),
	})
}

func (h *WebSocketHandler) notifyRoomDeviceSwitchRequest(
	ctx context.Context,
	client *room.ClientConnection,
	pending roomDeviceSwitch,
) error {
	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeRoomDeviceSwitchRequest,
		Payload: mustJSONRaw(protocol.RoomDeviceSwitchRequestPayload{
			RoomID:       pending.ActiveRoomID,
			TargetRoomID: pending.TargetRoomID,
			UserID:       pending.UserID,
			RequestID:    pending.RequestID,
			ExpiresAtMs:  pending.ExpiresAt.UnixMilli(),
		}),
	})
}

func (h *WebSocketHandler) writeRoomDeviceSwitchResult(
	ctx context.Context,
	client *room.ClientConnection,
	pending roomDeviceSwitch,
	approved bool,
	reason string,
) error {
	if client == nil {
		return nil
	}
	return client.WriteJSON(ctx, protocol.Envelope{
		Type: protocol.TypeRoomDeviceSwitchResult,
		Payload: mustJSONRaw(protocol.RoomDeviceSwitchResultPayload{
			RoomID:    pending.TargetRoomID,
			UserID:    pending.UserID,
			RequestID: pending.RequestID,
			Approved:  approved,
			Reason:    reason,
		}),
	})
}

func (h *WebSocketHandler) scheduleRoomDeviceSwitchTimeout(pending roomDeviceSwitch) {
	timeout := time.Until(pending.ExpiresAt)
	if timeout <= 0 {
		timeout = h.deviceSwitchTimeout
	}
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		<-timer.C

		expired, ok := h.deviceSwitches.take(pending.RequestID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = h.writeRoomDeviceSwitchResult(ctx, expired.PendingClient, expired, false, "device switch timeout")
		_ = expired.PendingClient.Close(websocket.StatusPolicyViolation, "device switch timeout")
	}()
}

func (h *WebSocketHandler) cancelRoomDeviceSwitchesForClient(client *room.ClientConnection, reason string) {
	for _, pending := range h.deviceSwitches.cancelForClient(client) {
		if pending.ActiveClient == client {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = h.writeRoomDeviceSwitchResult(ctx, pending.PendingClient, pending, false, reason)
			cancel()
			_ = pending.PendingClient.Close(websocket.StatusPolicyViolation, reason)
		}
	}
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
	client.MarkHeartbeatAck(h.clock.Now())
	meta := controlEventMeta{
		UserID:     client.UserID(),
		RequestID:  payload.RequestID,
		PositionMs: payload.PositionMs,
		ClientSeq:  payload.Seq,
	}
	h.logControlReceived(protocol.TypePlay, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypePlay,
		payload.RoomID,
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlayIfSeq(meta.UserID, payload.PositionMs, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypePlay, state, payload.RequestID)
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
	client.MarkHeartbeatAck(h.clock.Now())
	meta := controlEventMeta{
		UserID:     client.UserID(),
		RequestID:  payload.RequestID,
		PositionMs: payload.PositionMs,
		ClientSeq:  payload.Seq,
	}
	h.logControlReceived(protocol.TypePause, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypePause,
		payload.RoomID,
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPauseIfSeq(meta.UserID, payload.PositionMs, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypePause, state, payload.RequestID)
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
	client.MarkHeartbeatAck(h.clock.Now())
	meta := controlEventMeta{
		UserID:     client.UserID(),
		RequestID:  payload.RequestID,
		PositionMs: payload.PositionMs,
		ClientSeq:  payload.Seq,
	}
	h.logControlReceived(protocol.TypeSeek, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeSeek,
		payload.RoomID,
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplySeekIfSeq(meta.UserID, payload.PositionMs, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeSeek, state, payload.RequestID)
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
	client.MarkHeartbeatAck(h.clock.Now())
	meta := controlEventMeta{
		UserID:     client.UserID(),
		RequestID:  payload.RequestID,
		PositionMs: payload.PositionMs,
		ClientSeq:  payload.Seq,
	}
	h.logControlReceived(protocol.TypeSetPlaybackRate, payload.RoomID, meta, payload.PlaybackRate)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeSetPlaybackRate,
		payload.RoomID,
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyPlaybackRateIfSeq(meta.UserID, payload.PlaybackRate, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeSetPlaybackRate, state, payload.RequestID)
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
	client.MarkHeartbeatAck(h.clock.Now())
	meta := controlEventMeta{
		UserID:     client.UserID(),
		RequestID:  payload.RequestID,
		PositionMs: payload.PositionMs,
		ClientSeq:  payload.Seq,
	}
	h.logControlReceived(protocol.TypeEnded, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeEnded,
		payload.RoomID,
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyEndedIfSeq(meta.UserID, payload.PositionMs, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeEnded, state, payload.RequestID)
		},
	)
}

type controlEventMeta struct {
	UserID     string
	RequestID  string
	PositionMs int64
	ClientSeq  int64
}

func (h *WebSocketHandler) handleControlEvent(
	ctx context.Context,
	client *room.ClientConnection,
	eventType string,
	roomID string,
	meta controlEventMeta,
	apply func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error),
	buildEnvelope func(state room.State) protocol.Envelope,
) error {
	if client.UserID() == "" || client.UserID() != meta.UserID || client.RoomID() != roomID {
		return protocolMessageError{
			roomID:  roomID,
			message: "room identity mismatch",
		}
	}
	existingRoom, ok := h.roomManager.Get(roomID)
	if !ok {
		return protocolMessageError{
			roomID:  roomID,
			message: "room not found",
		}
	}
	if !existingRoom.IsActiveClient(client) {
		return protocolMessageError{
			roomID:  roomID,
			message: "active room device required",
		}
	}

	previous := existingRoom.StateSnapshot()
	if meta.ClientSeq != previous.Seq {
		if h.debugSync {
			log.Printf(
				"sync control_seq_mismatch room=%s type=%s user=%s request_id=%q client_seq=%d server_seq=%d",
				roomID,
				eventType,
				meta.UserID,
				meta.RequestID,
				meta.ClientSeq,
				previous.Seq,
			)
		}
		h.cacheRoomState(previous)
		return h.writeRoomState(ctx, client, previous)
	}

	if meta.RequestID != "" && !h.reserveControlRequest(roomID, meta.RequestID, h.clock.Now()) {
		if h.debugSync {
			log.Printf(
				"sync control_duplicate room=%s type=%s user=%s request_id=%q client_seq=%d server_seq=%d",
				roomID,
				eventType,
				meta.UserID,
				meta.RequestID,
				meta.ClientSeq,
				previous.Seq,
			)
		}
		return h.writeRoomState(ctx, client, previous)
	}

	var seekRateReservation time.Time
	if eventType == protocol.TypeSeek && h.seekRateLimiter != nil {
		now := h.clock.Now()
		seekRateReservation = now
		if !h.seekRateLimiter.Reserve(roomID, now) {
			h.forgetControlRequest(roomID, meta.RequestID)
			if h.debugSync {
				log.Printf(
					"sync control_rate_limited room=%s type=%s user=%s request_id=%q client_seq=%d server_seq=%d min_interval_ms=%d",
					roomID,
					eventType,
					meta.UserID,
					meta.RequestID,
					meta.ClientSeq,
					previous.Seq,
					h.seekRateLimiter.interval.Milliseconds(),
				)
			}
			h.cacheRoomState(previous)
			return h.writeRoomState(ctx, client, previous)
		}
	}

	state, clients, err := apply(existingRoom)
	if err != nil {
		h.forgetControlRequest(roomID, meta.RequestID)
		if eventType == protocol.TypeSeek && !seekRateReservation.IsZero() {
			h.seekRateLimiter.ForgetReservation(roomID, seekRateReservation)
		}
		if errors.Is(err, room.ErrSeqMismatch) {
			if h.debugSync {
				log.Printf(
					"sync control_seq_mismatch room=%s type=%s user=%s request_id=%q client_seq=%d server_seq=%d",
					roomID,
					eventType,
					meta.UserID,
					meta.RequestID,
					meta.ClientSeq,
					state.Seq,
				)
			}
			h.cacheRoomState(state)
			return h.writeRoomState(ctx, client, state)
		}
		if errors.Is(err, room.ErrNotHost) {
			return protocolMessageError{
				roomID:  roomID,
				message: "only host can control playback",
			}
		}
		return err
	}

	h.logTimelineTransition(eventType, roomID, meta, previous, state)
	envelope := buildEnvelope(state)
	h.cacheRoomState(state)
	stats, err := h.broadcaster.Broadcast(ctx, roomClientWriters(clients), envelope)
	h.logBroadcastStats(eventType, roomID, state.Seq, &state, stats, err)
	return err
}

func (h *WebSocketHandler) logControlReceived(
	eventType string,
	roomID string,
	meta controlEventMeta,
	playbackRate float64,
) {
	if !h.debugSync {
		return
	}
	if playbackRate > 0 {
		log.Printf(
			"sync control received type=%s room=%s user=%s pos=%d seq=%d rate=%.2f request_id=%q",
			eventType,
			roomID,
			meta.UserID,
			meta.PositionMs,
			meta.ClientSeq,
			playbackRate,
			meta.RequestID,
		)
		return
	}
	log.Printf(
		"sync control received type=%s room=%s user=%s pos=%d seq=%d request_id=%q",
		eventType,
		roomID,
		meta.UserID,
		meta.PositionMs,
		meta.ClientSeq,
		meta.RequestID,
	)
}

func (h *WebSocketHandler) logTimelineTransition(
	eventType string,
	roomID string,
	meta controlEventMeta,
	previous room.State,
	next room.State,
) {
	if !h.debugSync {
		return
	}
	log.Printf(
		"sync timeline_transition room=%s media=%s type=%s user=%s request_id=%q client_seq=%d previous_seq=%d new_seq=%d client_pos=%d previous_pos=%d new_pos=%d previous_velocity=%.2f new_velocity=%.2f previous_rate=%.2f new_rate=%.2f previous_paused=%t new_paused=%t reason=%s server_time_ms=%d",
		roomID,
		next.MediaID,
		eventType,
		meta.UserID,
		meta.RequestID,
		meta.ClientSeq,
		previous.Seq,
		next.Seq,
		meta.PositionMs,
		previous.PositionMs,
		next.PositionMs,
		previous.Velocity,
		next.Velocity,
		previous.PlaybackRate,
		next.PlaybackRate,
		previous.Paused,
		next.Paused,
		next.Reason,
		next.ServerTimeMs,
	)
}

func (h *WebSocketHandler) logBroadcastStats(
	eventType string,
	roomID string,
	seq int64,
	state *room.State,
	stats broadcastStats,
	err error,
) {
	if !h.debugSync {
		return
	}
	if state != nil {
		log.Printf(
			"sync broadcast type=%s room=%s seq=%d clients=%d failed=%d timed_out=%d closed=%d coalesced=%d queue_pressure=%d max_queue_depth=%d pos=%d paused=%t rate=%.2f duration_us=%d slowest_user=%s slowest_us=%d err=%v",
			eventType,
			roomID,
			seq,
			stats.Clients,
			stats.FailedClients,
			stats.TimedOutClients,
			stats.ClosedClients,
			stats.CoalescedClients,
			stats.QueuePressureClients,
			stats.MaxQueueDepth,
			state.PositionMs,
			state.Paused,
			state.PlaybackRate,
			stats.Duration.Microseconds(),
			stats.SlowestUserID,
			stats.SlowestDuration.Microseconds(),
			err,
		)
		return
	}
	log.Printf(
		"sync broadcast type=%s room=%s seq=%d clients=%d failed=%d timed_out=%d closed=%d coalesced=%d queue_pressure=%d max_queue_depth=%d duration_us=%d slowest_user=%s slowest_us=%d err=%v",
		eventType,
		roomID,
		seq,
		stats.Clients,
		stats.FailedClients,
		stats.TimedOutClients,
		stats.ClosedClients,
		stats.CoalescedClients,
		stats.QueuePressureClients,
		stats.MaxQueueDepth,
		stats.Duration.Microseconds(),
		stats.SlowestUserID,
		stats.SlowestDuration.Microseconds(),
		err,
	)
}

func (h *WebSocketHandler) broadcastEnvelope(
	ctx context.Context,
	roomID string,
	clients []*room.ClientConnection,
	envelope protocol.Envelope,
) error {
	stats, err := h.broadcaster.Broadcast(ctx, roomClientWriters(clients), envelope)
	h.logBroadcastStats(envelope.Type, roomID, 0, nil, stats, err)
	return err
}

func (h *WebSocketHandler) writeRoomState(
	ctx context.Context,
	client *room.ClientConnection,
	state room.State,
) error {
	return client.WriteJSON(ctx, protocol.Envelope{
		Type:    protocol.TypeRoomState,
		Payload: mustJSONRaw(roomStatePayload(state)),
	})
}

// broadcastRoomState pushes the latest authority snapshot after membership changes.
// Disconnect cleanup uses a fresh context because the request context may already be canceled.
func (h *WebSocketHandler) broadcastRoomState(result room.RemoveClientResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	h.cacheRoomState(result.State)
	_ = h.broadcastEnvelope(ctx, result.State.RoomID, result.Remaining, protocol.Envelope{
		Type:    protocol.TypeRoomState,
		Payload: mustJSONRaw(roomStatePayload(result.State)),
	})
}

func (h *WebSocketHandler) cacheRoomState(state room.State) {
	if h.roomStateWriter == nil || state.RoomID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	payload := roomStatePayload(state)
	if err := h.roomStateWriter.SetRoomState(ctx, state.RoomID, payload); err != nil && h.debugSync {
		log.Printf("room_state cache write failed room=%s seq=%d err=%v", state.RoomID, state.Seq, err)
	}
}

func clientsWithout(clients []*room.ClientConnection, excluded *room.ClientConnection) []*room.ClientConnection {
	filtered := make([]*room.ClientConnection, 0, len(clients))
	for _, client := range clients {
		if client == nil || client == excluded {
			continue
		}
		filtered = append(filtered, client)
	}
	return filtered
}

// Attempt to register a control request. If this request has been processed recently, return false; if it is a new request, register it and return true.
func (h *WebSocketHandler) reserveControlRequest(roomID string, requestID string, now time.Time) bool {
	return h.controlDeduper.Reserve(roomID, requestID, now)
}

// If a RequestID indicates that a request has failed to complete the entire process of a service
func (h *WebSocketHandler) forgetControlRequest(roomID string, requestID string) {
	h.controlDeduper.Forget(roomID, requestID)
}

func roomStatePayload(state room.State) protocol.RoomStatePayload {
	view := newRoomSyncView(state)
	return protocol.RoomStatePayload{
		RoomID:          state.RoomID,
		MediaID:         state.MediaID,
		MediaDurationMs: view.MediaDurationMs,
		HostUserID:      state.HostUserID,
		Paused:          view.Paused,
		Ended:           view.Ended,
		PositionMs:      view.PositionMs,
		Velocity:        view.Velocity,
		ServerTimeMs:    view.ServerTimeMs,
		Reason:          view.Reason,
		PlaybackRate:    view.PlaybackRate,
		Seq:             view.Seq,
	}
}

func controlEnvelopeFromState(eventType string, state room.State, requestID string) protocol.Envelope {
	switch eventType {
	case protocol.TypePlay:
		return protocol.Envelope{
			Type:    protocol.TypePlay,
			Payload: mustJSONRaw(playPayloadFromState(state, requestID)),
		}
	case protocol.TypePause:
		return protocol.Envelope{
			Type:    protocol.TypePause,
			Payload: mustJSONRaw(pausePayloadFromState(state, requestID)),
		}
	case protocol.TypeSeek:
		return protocol.Envelope{
			Type:    protocol.TypeSeek,
			Payload: mustJSONRaw(seekPayloadFromState(state, requestID)),
		}
	case protocol.TypeSetPlaybackRate:
		return protocol.Envelope{
			Type:    protocol.TypeSetPlaybackRate,
			Payload: mustJSONRaw(setPlaybackRatePayloadFromState(state, requestID)),
		}
	case protocol.TypeEnded:
		return protocol.Envelope{
			Type:    protocol.TypeEnded,
			Payload: mustJSONRaw(endedPayloadFromState(state, requestID)),
		}
	default:
		return protocol.Envelope{
			Type:    protocol.TypeRoomState,
			Payload: mustJSONRaw(roomStatePayload(state)),
		}
	}
}

func playPayloadFromState(state room.State, requestID string) protocol.PlayPayload {
	return protocol.PlayPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		RequestID:    requestID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func pausePayloadFromState(state room.State, requestID string) protocol.PausePayload {
	return protocol.PausePayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		RequestID:    requestID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func seekPayloadFromState(state room.State, requestID string) protocol.SeekPayload {
	return protocol.SeekPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		RequestID:    requestID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		Seq:          state.Seq,
	}
}

func setPlaybackRatePayloadFromState(state room.State, requestID string) protocol.SetPlaybackRatePayload {
	return protocol.SetPlaybackRatePayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		RequestID:    requestID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		PlaybackRate: state.PlaybackRate,
		Seq:          state.Seq,
	}
}

func endedPayloadFromState(state room.State, requestID string) protocol.EndedPayload {
	return protocol.EndedPayload{
		RoomID:       state.RoomID,
		UserID:       state.HostUserID,
		RequestID:    requestID,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
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

	_ = h.broadcastEnvelope(ctx, roomID, targets, protocol.Envelope{
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
			now := h.clock.Now()
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
