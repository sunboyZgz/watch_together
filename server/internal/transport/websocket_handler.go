package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/sync/semaphore"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/realtime"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/timeline"
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
	roomMemberChecker   roomMembershipChecker
	controlDeduper      *controlRequestDeduper
	seekRateLimiter     *controlRateLimiter
	tokenVerifier       accessTokenVerifier
	deviceSwitches      *roomDeviceSwitchRegistry
	deviceSwitchTimeout time.Duration
	roomBroadcastBus    eventbus.RoomBroadcastBus
	roomControlBus      eventbus.RoomControlBus
	roomAuthority       roomAuthorityLookup
	authorityRecovery   roomAuthorityRecovery
	activeDevices       activeDeviceRegistry
	controlRequests     controlRequestRegistry
	presence            presenceRegistry
	presenceRefresh     time.Duration
	timelineRecorder    timeline.Recorder
	instanceID          string
	roomRuntimeMode     string
}

const (
	defaultHeartbeatInterval         = 5 * time.Second
	defaultHeartbeatTimeout          = 15 * time.Second
	defaultBroadcastConcurrencyLimit = int64(64)
	defaultBroadcastTimeout          = 5 * time.Second
	defaultBroadcastEnqueueTimeout   = 3 * time.Second
	defaultControlRequestDedupTTL    = 2 * time.Minute
	defaultControlIdempotencyTTL     = 10 * time.Minute
	defaultPresenceLeaseTTL          = 45 * time.Second
	defaultPresenceRefreshInterval   = 15 * time.Second
	defaultSeekMinInterval           = 250 * time.Millisecond
	roomRuntimeModeLocalProcess      = "local_process"
	roomRuntimeModeDistributed       = "distributed_authority"
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
	ControlIdempotencyTTL     time.Duration
	PresenceLeaseTTL          time.Duration
	PresenceRefreshInterval   time.Duration
	CrossInstanceBroadcast    bool
	EventBus                  string
}

type latestRoomStateWriter interface {
	SetRoomState(ctx context.Context, roomID string, state protocol.RoomStatePayload) error
}

type roomMembershipLeaver interface {
	LeaveRoomByCode(ctx context.Context, roomCode string, userID string) error
}

type roomMembershipChecker interface {
	IsActiveMemberByCode(ctx context.Context, roomCode string, userID string) (bool, error)
}

type roomAuthorityLookup interface {
	GetAuthority(ctx context.Context, roomID string) (cache.RoomAuthorityLease, bool, error)
}

type roomAuthorityRecovery interface {
	TryRecoverRoomAuthority(ctx context.Context, roomID string, reason string) (recovery.Result, error)
}

type activeDeviceRegistry interface {
	Acquire(ctx context.Context, roomID string, userID string, deviceID string, instanceID string, connectionID string) (cache.ActiveDeviceLease, bool, error)
	Get(ctx context.Context, roomID string, userID string) (cache.ActiveDeviceLease, bool, error)
	ReleaseIfMatch(ctx context.Context, roomID string, userID string, deviceID string, connectionID string) (bool, error)
}

type controlRequestRegistry interface {
	Reserve(ctx context.Context, roomID string, requestID string, authorityEpoch int64) (cache.ControlRequestRecord, bool, error)
	FinalizeAccepted(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, envelope []byte) (cache.ControlRequestRecord, bool, error)
	FinalizeRejected(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, message string) (cache.ControlRequestRecord, bool, error)
	Forget(ctx context.Context, roomID string, requestID string) error
}

type presenceRegistry interface {
	Upsert(ctx context.Context, roomID string, userID string, role string, deviceID string, instanceID string, connectionID string, isHost bool) (cache.PresenceMember, bool, error)
	ReleaseIfMatch(ctx context.Context, roomID string, userID string, deviceID string, connectionID string) (bool, error)
	Snapshot(ctx context.Context, roomID string) (cache.PresenceSnapshot, error)
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
	if checker, ok := roomLeaver.(roomMembershipChecker); ok {
		handler.roomMemberChecker = checker
	}
	return handler
}

func (h *WebSocketHandler) SetRoomBroadcastBus(instanceID string, bus eventbus.RoomBroadcastBus) {
	h.instanceID = instanceID
	h.roomBroadcastBus = bus
}

func (h *WebSocketHandler) SetDistributedAuthorityRuntime(
	instanceID string,
	authority roomAuthorityLookup,
	activeDevices activeDeviceRegistry,
	controlBus eventbus.RoomControlBus,
	recorder timeline.Recorder,
) {
	h.instanceID = instanceID
	h.roomRuntimeMode = roomRuntimeModeDistributed
	h.roomAuthority = authority
	h.activeDevices = activeDevices
	h.roomControlBus = controlBus
	if recorder != nil {
		h.timelineRecorder = recorder
	}
}

func (h *WebSocketHandler) SetRoomAuthorityRecovery(authorityRecovery roomAuthorityRecovery) {
	if h == nil {
		return
	}
	h.authorityRecovery = authorityRecovery
}

func (h *WebSocketHandler) SetDistributedControlHardening(
	controlRequests controlRequestRegistry,
	presence presenceRegistry,
	presenceRefreshInterval time.Duration,
) {
	if h == nil {
		return
	}
	h.controlRequests = controlRequests
	h.presence = presence
	if presenceRefreshInterval > 0 {
		h.presenceRefresh = presenceRefreshInterval
	}
}

func (h *WebSocketHandler) SubscribeRoomBroadcasts(ctx context.Context) error {
	if h == nil || h.roomBroadcastBus == nil {
		return nil
	}
	return h.roomBroadcastBus.SubscribeRoomBroadcasts(ctx, h.handleRemoteRoomBroadcast)
}

func (h *WebSocketHandler) SubscribeRoomControls(ctx context.Context) error {
	if h == nil || h.roomControlBus == nil || h.instanceID == "" || !h.isDistributedAuthorityMode() {
		return nil
	}
	return h.roomControlBus.SubscribeRoomControls(ctx, h.instanceID, h.handleAuthorityRoomControlRequest)
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
		presenceRefresh:     config.PresenceRefreshInterval,
		timelineRecorder:    timeline.NoopRecorder{},
		roomRuntimeMode:     roomRuntimeModeLocalProcess,
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
	if config.ControlIdempotencyTTL <= 0 {
		config.ControlIdempotencyTTL = defaultControlIdempotencyTTL
	}
	if config.PresenceLeaseTTL <= 0 {
		config.PresenceLeaseTTL = defaultPresenceLeaseTTL
	}
	if config.PresenceRefreshInterval <= 0 {
		config.PresenceRefreshInterval = defaultPresenceRefreshInterval
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
		roomID := client.RoomID()
		removeResult := h.roomManager.RemoveClient(client)
		h.releasePresenceForClient(context.Background(), client)
		h.broadcastRoomPresenceForRoom(context.Background(), roomID, "disconnect", true)
		if removeResult.HostUnavailable {
			h.broadcastRoomState(removeResult)
		}
		h.releaseActiveDeviceForClient(context.Background(), client)
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
	h.releasePresenceForClient(ctx, client)
	h.releaseActiveDeviceForClient(ctx, client)
	h.recordMembershipTimeline(ctx, timeline.EventTypeMemberLeft, payload.RoomID, payload.UserID, client.DeviceID(), nil)
	client.SetIdentity("", "")
	if leaveResult.HostUnavailable {
		h.broadcastRoomState(leaveResult)
	}
	if !leaveResult.RoomRemoved {
		h.broadcastRoomMembersChangedToOthers(payload.RoomID, leaveResult.Remaining, client, "leave")
	}
	h.broadcastRoomPresenceForRoom(ctx, payload.RoomID, "leave", true)
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
	if h.isDistributedAuthorityMode() {
		if _, _, err := h.currentRoomAuthorityOrRecover(ctx, payload.RoomID, "room_state.request"); err != nil {
			return protocolMessageError{
				roomID:  payload.RoomID,
				message: authorityRecoveryErrorMessage(err),
			}
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
	if err := h.ensureActiveRoomMember(ctx, payload.RoomID, authUserID); err != nil {
		return err
	}
	if h.isDistributedAuthorityMode() {
		if _, _, err := h.currentRoomAuthorityOrRecover(ctx, payload.RoomID, "join_room"); err != nil {
			return protocolMessageError{
				roomID:  payload.RoomID,
				message: authorityRecoveryErrorMessage(err),
			}
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
	client.SetDeviceID(payload.DeviceID)
	if err := h.acquireActiveDevice(ctx, payload.RoomID, authUserID, payload.DeviceID, client); err != nil {
		client.SetIdentity("", "")
		return err
	}
	if !h.isDistributedAuthorityMode() {
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
	}
	joinResult := existingRoom.JoinWithLimit(client, h.maxRoomClients)
	if joinResult.Err != nil {
		h.releaseActiveDeviceForClient(ctx, client)
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
	if err := h.upsertPresenceForClient(ctx, client, joinResult.State); err != nil {
		h.releaseActiveDeviceForClient(ctx, client)
		_ = h.roomManager.RemoveClient(client)
		client.SetIdentity("", "")
		return err
	}
	h.broadcastRoomPresenceForRoom(ctx, payload.RoomID, "join", true)
	if joinResult.HostReclaimed {
		h.broadcastRoomState(room.RemoveClientResult{
			State:     joinResult.State,
			Remaining: clientsWithout(joinResult.Clients, client),
		})
	}
	if joinResult.MembershipChanged {
		h.broadcastRoomMembersChangedToOthers(payload.RoomID, joinResult.Clients, client, "join")
		h.recordMembershipTimeline(ctx, timeline.EventTypeMemberJoined, payload.RoomID, authUserID, payload.DeviceID, protocol.RoomMembersChangedPayload{
			RoomID: payload.RoomID,
			Reason: "join",
		})
	}
	return nil
}

func (h *WebSocketHandler) ensureActiveRoomMember(ctx context.Context, roomID string, userID string) error {
	if h.roomMemberChecker == nil {
		return nil
	}
	active, err := h.roomMemberChecker.IsActiveMemberByCode(ctx, roomID, userID)
	if err != nil {
		if errors.Is(err, roomapi.ErrInvalidInput) {
			return protocolMessageError{
				roomID:  roomID,
				message: "room membership is invalid",
			}
		}
		return err
	}
	if !active {
		return protocolMessageError{
			roomID:  roomID,
			message: "room membership required",
		}
	}
	return nil
}

func (h *WebSocketHandler) isDistributedAuthorityMode() bool {
	return h != nil && h.roomRuntimeMode == roomRuntimeModeDistributed
}

func (h *WebSocketHandler) acquireActiveDevice(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	client *room.ClientConnection,
) error {
	if !h.isDistributedAuthorityMode() || h.activeDevices == nil {
		return nil
	}
	lease, acquired, err := h.activeDevices.Acquire(ctx, roomID, userID, deviceID, h.instanceID, client.ConnectionID())
	if err != nil {
		return err
	}
	if !acquired {
		return protocolMessageError{
			roomID:  roomID,
			message: "active room device already exists",
		}
	}
	if h.debugSync {
		log.Printf(
			"active_device acquired room=%s user=%s device=%s connection=%s owner_instance=%s lease_until_ms=%d",
			roomID,
			userID,
			deviceID,
			client.ConnectionID(),
			lease.InstanceID,
			lease.LeaseUntilMs,
		)
	}
	return nil
}

func (h *WebSocketHandler) releaseActiveDeviceForClient(ctx context.Context, client *room.ClientConnection) {
	if !h.isDistributedAuthorityMode() || h.activeDevices == nil || client == nil {
		return
	}
	roomID := client.RoomID()
	userID := client.UserID()
	deviceID := client.DeviceID()
	connectionID := client.ConnectionID()
	if roomID == "" || userID == "" || deviceID == "" || connectionID == "" {
		return
	}
	released, err := h.activeDevices.ReleaseIfMatch(ctx, roomID, userID, deviceID, connectionID)
	if err != nil && h.debugSync {
		log.Printf("active_device release failed room=%s user=%s device=%s connection=%s err=%v", roomID, userID, deviceID, connectionID, err)
	}
	if released && h.debugSync {
		log.Printf("active_device released room=%s user=%s device=%s connection=%s", roomID, userID, deviceID, connectionID)
	}
}

func (h *WebSocketHandler) ensureActiveDeviceControl(
	ctx context.Context,
	roomID string,
	client *room.ClientConnection,
) error {
	if !h.isDistributedAuthorityMode() || h.activeDevices == nil {
		return nil
	}
	lease, found, err := h.activeDevices.Get(ctx, roomID, client.UserID())
	if err != nil {
		return err
	}
	if !found ||
		lease.DeviceID != client.DeviceID() ||
		lease.ConnectionID != client.ConnectionID() ||
		lease.InstanceID != h.instanceID {
		return protocolMessageError{
			roomID:  roomID,
			message: "active room device required",
		}
	}
	return h.refreshActiveDeviceLeaseForClient(ctx, client)
}

func (h *WebSocketHandler) refreshActiveDeviceLeaseForClient(
	ctx context.Context,
	client *room.ClientConnection,
) error {
	if !h.isDistributedAuthorityMode() || h.activeDevices == nil || client == nil {
		return nil
	}
	roomID := client.RoomID()
	userID := client.UserID()
	deviceID := client.DeviceID()
	connectionID := client.ConnectionID()
	if roomID == "" || userID == "" || deviceID == "" || connectionID == "" {
		return nil
	}
	_, acquired, err := h.activeDevices.Acquire(ctx, roomID, userID, deviceID, h.instanceID, connectionID)
	if err != nil {
		return err
	}
	if !acquired {
		return protocolMessageError{
			roomID:  roomID,
			message: "active room device required",
		}
	}
	return nil
}

func (h *WebSocketHandler) recordMembershipTimeline(
	ctx context.Context,
	eventType string,
	roomID string,
	userID string,
	deviceID string,
	payload any,
) {
	_ = h.recordTimelineEvent(ctx, eventType, roomID, userID, deviceID, "", 0, payload)
}

func (h *WebSocketHandler) recordControlAccepted(
	ctx context.Context,
	eventType string,
	roomID string,
	meta controlEventMeta,
	seq int64,
	envelope protocol.Envelope,
) error {
	return h.recordTimelineEvent(ctx, timeline.EventTypeControlAccepted, roomID, meta.UserID, meta.DeviceID, eventType, seq, envelope)
}

func (h *WebSocketHandler) recordControlRejected(
	ctx context.Context,
	eventType string,
	roomID string,
	meta controlEventMeta,
	reason string,
) {
	_ = h.recordTimelineEvent(ctx, timeline.EventTypeControlRejected, roomID, meta.UserID, meta.DeviceID, eventType, meta.ClientSeq, map[string]any{
		"type":      eventType,
		"requestId": meta.RequestID,
		"reason":    reason,
	})
}

func (h *WebSocketHandler) recordTimelineEvent(
	ctx context.Context,
	eventType string,
	roomID string,
	userID string,
	deviceID string,
	controlType string,
	seq int64,
	payload any,
) error {
	if h == nil || h.timelineRecorder == nil || roomID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		if h.debugSync {
			log.Printf("timeline event payload marshal failed room=%s type=%s err=%v", roomID, eventType, err)
		}
		return err
	}
	event := timeline.NewEvent(eventType, roomID, h.clock.Now())
	event.UserID = userID
	event.DeviceID = deviceID
	event.InstanceID = h.instanceID
	event.ControlType = controlType
	event.Seq = seq
	event.Payload = data
	recordCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if err := h.timelineRecorder.RecordTimelineEvent(recordCtx, event); err != nil {
		if h.debugSync {
			log.Printf("timeline outbox write failed room=%s type=%s err=%v", roomID, eventType, err)
		}
		return err
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
		UserID:       client.UserID(),
		DeviceID:     client.DeviceID(),
		ConnectionID: client.ConnectionID(),
		RequestID:    payload.RequestID,
		PositionMs:   payload.PositionMs,
		ClientSeq:    payload.Seq,
	}
	h.logControlReceived(protocol.TypePlay, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypePlay,
		payload.RoomID,
		meta,
		envelope,
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
		UserID:       client.UserID(),
		DeviceID:     client.DeviceID(),
		ConnectionID: client.ConnectionID(),
		RequestID:    payload.RequestID,
		PositionMs:   payload.PositionMs,
		ClientSeq:    payload.Seq,
	}
	h.logControlReceived(protocol.TypePause, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypePause,
		payload.RoomID,
		meta,
		envelope,
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
		UserID:       client.UserID(),
		DeviceID:     client.DeviceID(),
		ConnectionID: client.ConnectionID(),
		RequestID:    payload.RequestID,
		PositionMs:   payload.PositionMs,
		ClientSeq:    payload.Seq,
	}
	h.logControlReceived(protocol.TypeSeek, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeSeek,
		payload.RoomID,
		meta,
		envelope,
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
		UserID:       client.UserID(),
		DeviceID:     client.DeviceID(),
		ConnectionID: client.ConnectionID(),
		RequestID:    payload.RequestID,
		PositionMs:   payload.PositionMs,
		ClientSeq:    payload.Seq,
	}
	h.logControlReceived(protocol.TypeSetPlaybackRate, payload.RoomID, meta, payload.PlaybackRate)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeSetPlaybackRate,
		payload.RoomID,
		meta,
		envelope,
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
		UserID:       client.UserID(),
		DeviceID:     client.DeviceID(),
		ConnectionID: client.ConnectionID(),
		RequestID:    payload.RequestID,
		PositionMs:   payload.PositionMs,
		ClientSeq:    payload.Seq,
	}
	h.logControlReceived(protocol.TypeEnded, payload.RoomID, meta, 0)
	return h.handleControlEvent(
		ctx,
		client,
		protocol.TypeEnded,
		payload.RoomID,
		meta,
		envelope,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			return existingRoom.ApplyEndedIfSeq(meta.UserID, payload.PositionMs, meta.ClientSeq)
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypeEnded, state, payload.RequestID)
		},
	)
}

type controlEventMeta struct {
	UserID                 string
	DeviceID               string
	ConnectionID           string
	RequestID              string
	PositionMs             int64
	ClientSeq              int64
	AuthorityEpoch         int64
	IdempotencyPreReserved bool
}

func (h *WebSocketHandler) handleControlEvent(
	ctx context.Context,
	client *room.ClientConnection,
	eventType string,
	roomID string,
	meta controlEventMeta,
	sourceEnvelope protocol.Envelope,
	apply func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error),
	buildEnvelope func(state room.State) protocol.Envelope,
) error {
	if client.UserID() == "" || client.UserID() != meta.UserID || client.RoomID() != roomID {
		err := protocolMessageError{
			roomID:  roomID,
			message: "room identity mismatch",
		}
		h.recordControlRejected(ctx, eventType, roomID, meta, err.message)
		return err
	}
	if h.isDistributedAuthorityMode() {
		if err := h.ensureActiveDeviceControl(ctx, roomID, client); err != nil {
			h.recordControlRejected(ctx, eventType, roomID, meta, err.Error())
			return err
		}
		authority, found, err := h.currentRoomAuthorityOrRecover(ctx, roomID, "control")
		if err != nil {
			message := authorityRecoveryErrorMessage(err)
			h.recordControlRejected(ctx, eventType, roomID, meta, message)
			return protocolMessageError{roomID: roomID, message: message}
		}
		if !found || !authority.IsActive() {
			protocolErr := protocolMessageError{
				roomID:  roomID,
				message: "room authority unavailable",
			}
			h.recordControlRejected(ctx, eventType, roomID, meta, protocolErr.message)
			return protocolErr
		}
		if authority.InstanceID != h.instanceID {
			return h.forwardControlEvent(ctx, client, eventType, roomID, meta, sourceEnvelope, authority.InstanceID, authority.Epoch)
		}
		meta.AuthorityEpoch = authority.Epoch
	}
	existingRoom, ok := h.roomManager.Get(roomID)
	if !ok {
		err := protocolMessageError{
			roomID:  roomID,
			message: "room not found",
		}
		h.recordControlRejected(ctx, eventType, roomID, meta, err.message)
		return err
	}
	if !existingRoom.IsActiveClient(client) {
		err := protocolMessageError{
			roomID:  roomID,
			message: "active room device required",
		}
		h.recordControlRejected(ctx, eventType, roomID, meta, err.message)
		return err
	}

	response, accepted, err := h.applyLocalControlEvent(ctx, existingRoom, eventType, roomID, meta, apply, buildEnvelope)
	if err != nil {
		return err
	}
	if !accepted {
		return client.WriteJSON(ctx, response)
	}
	return nil
}

func (h *WebSocketHandler) applyLocalControlEvent(
	ctx context.Context,
	existingRoom *room.Room,
	eventType string,
	roomID string,
	meta controlEventMeta,
	apply func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error),
	buildEnvelope func(state room.State) protocol.Envelope,
) (protocol.Envelope, bool, error) {
	previous := existingRoom.StateSnapshot()
	if h.isDistributedAuthorityMode() {
		duplicate, handled, err := h.reserveDistributedControlRequest(ctx, roomID, meta)
		if err != nil {
			h.recordControlRejected(ctx, eventType, roomID, meta, controlErrorMessage(err))
			return protocol.Envelope{}, false, err
		}
		if handled {
			return duplicate, false, nil
		}
		if err := h.ensureLocalAuthorityEpoch(ctx, roomID, meta.AuthorityEpoch); err != nil {
			message := controlErrorMessage(err)
			h.finalizeDistributedControlRejected(ctx, roomID, meta, previous.Seq, message)
			h.recordControlRejected(ctx, eventType, roomID, meta, message)
			return protocol.Envelope{}, false, err
		}
	}
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
		h.finalizeDistributedControlRejected(ctx, roomID, meta, previous.Seq, "control seq does not match current room state")
		h.recordControlRejected(ctx, eventType, roomID, meta, "control seq does not match current room state")
		return protocol.Envelope{
			Type:    protocol.TypeRoomState,
			Payload: mustJSONRaw(roomStatePayload(previous)),
		}, false, nil
	}

	if !h.isDistributedAuthorityMode() && meta.RequestID != "" && !h.reserveControlRequest(roomID, meta.RequestID, h.clock.Now()) {
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
		return protocol.Envelope{
			Type:    protocol.TypeRoomState,
			Payload: mustJSONRaw(roomStatePayload(previous)),
		}, false, nil
	}

	var seekRateReservation time.Time
	if eventType == protocol.TypeSeek && h.seekRateLimiter != nil {
		now := h.clock.Now()
		seekRateReservation = now
		if !h.seekRateLimiter.Reserve(roomID, now) {
			h.forgetLocalControlRequest(roomID, meta.RequestID)
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
			h.finalizeDistributedControlRejected(ctx, roomID, meta, previous.Seq, "control rate limited")
			h.recordControlRejected(ctx, eventType, roomID, meta, "control rate limited")
			return protocol.Envelope{
				Type:    protocol.TypeRoomState,
				Payload: mustJSONRaw(roomStatePayload(previous)),
			}, false, nil
		}
	}

	state, clients, err := apply(existingRoom)
	if err != nil {
		h.forgetLocalControlRequest(roomID, meta.RequestID)
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
			h.finalizeDistributedControlRejected(ctx, roomID, meta, state.Seq, room.ErrSeqMismatch.Error())
			h.recordControlRejected(ctx, eventType, roomID, meta, room.ErrSeqMismatch.Error())
			return protocol.Envelope{
				Type:    protocol.TypeRoomState,
				Payload: mustJSONRaw(roomStatePayload(state)),
			}, false, nil
		}
		if errors.Is(err, room.ErrNotHost) {
			protocolErr := protocolMessageError{
				roomID:  roomID,
				message: "only host can control playback",
			}
			h.finalizeDistributedControlRejected(ctx, roomID, meta, meta.ClientSeq, protocolErr.message)
			h.recordControlRejected(ctx, eventType, roomID, meta, protocolErr.message)
			return protocol.Envelope{}, false, protocolErr
		}
		h.finalizeDistributedControlRejected(ctx, roomID, meta, meta.ClientSeq, err.Error())
		h.recordControlRejected(ctx, eventType, roomID, meta, err.Error())
		return protocol.Envelope{}, false, err
	}

	h.logTimelineTransition(eventType, roomID, meta, previous, state)
	envelope := buildEnvelope(state)
	if h.isDistributedAuthorityMode() {
		if err := h.ensureLocalAuthorityEpoch(ctx, roomID, meta.AuthorityEpoch); err != nil {
			message := controlErrorMessage(err)
			existingRoom.RestoreState(previous)
			h.cacheRoomState(previous)
			h.finalizeDistributedControlRejected(ctx, roomID, meta, previous.Seq, message)
			h.recordControlRejected(ctx, eventType, roomID, meta, message)
			return protocol.Envelope{}, false, err
		}
		if err := h.recordControlAccepted(ctx, eventType, roomID, meta, state.Seq, envelope); err != nil {
			existingRoom.RestoreState(previous)
			h.cacheRoomState(previous)
			message := "room timeline unavailable"
			if h.debugSync {
				log.Printf("strict timeline outbox write failed room=%s type=%s request_id=%q err=%v", roomID, eventType, meta.RequestID, err)
			}
			h.finalizeDistributedControlRejected(ctx, roomID, meta, previous.Seq, message)
			return protocol.Envelope{}, false, protocolMessageError{roomID: roomID, message: message}
		}
		if err := h.finalizeDistributedControlAccepted(ctx, roomID, meta, state.Seq, envelope); err != nil && h.debugSync {
			log.Printf("control idempotency accepted finalize failed room=%s request_id=%q epoch=%d err=%v", roomID, meta.RequestID, meta.AuthorityEpoch, err)
		}
	} else {
		_ = h.recordControlAccepted(ctx, eventType, roomID, meta, state.Seq, envelope)
	}
	h.cacheRoomState(state)
	stats, err := h.broadcaster.Broadcast(ctx, roomClientWriters(clients), envelope)
	h.logBroadcastStats(eventType, roomID, state.Seq, &state, stats, err)
	h.publishRoomBroadcast(ctx, roomID, state.Seq, envelope, meta.AuthorityEpoch)
	return envelope, true, err
}

func (h *WebSocketHandler) currentRoomAuthority(
	ctx context.Context,
	roomID string,
) (cache.RoomAuthorityLease, bool, error) {
	if h.roomAuthority == nil {
		return cache.RoomAuthorityLease{}, false, nil
	}
	return h.roomAuthority.GetAuthority(ctx, roomID)
}

func (h *WebSocketHandler) currentRoomAuthorityOrRecover(
	ctx context.Context,
	roomID string,
	reason string,
) (cache.RoomAuthorityLease, bool, error) {
	authority, found, err := h.currentRoomAuthority(ctx, roomID)
	if err != nil {
		return cache.RoomAuthorityLease{}, false, err
	}
	if found && authority.IsActive() && !authority.ExpiredAt(h.clock.Now()) {
		return authority, true, nil
	}
	if found && authority.IsRecovering() && !authority.ExpiredAt(h.clock.Now()) {
		return authority, true, recovery.ErrAuthorityRecovering
	}
	if h.authorityRecovery == nil {
		return authority, found, nil
	}
	result, err := h.authorityRecovery.TryRecoverRoomAuthority(ctx, roomID, reason)
	if err != nil {
		if errors.Is(err, recovery.ErrAuthorityActive) && result.Lease.InstanceID != "" {
			return result.Lease, true, nil
		}
		return result.Lease, result.Lease.InstanceID != "", err
	}
	h.seedRecoveredControlRequests(ctx, roomID, result.Lease.Epoch, result.Requests, result.RequestIDs)
	return result.Lease, result.Lease.InstanceID != "", nil
}

func (h *WebSocketHandler) ensureLocalAuthorityEpoch(
	ctx context.Context,
	roomID string,
	authorityEpoch int64,
) error {
	if !h.isDistributedAuthorityMode() {
		return nil
	}
	authority, found, err := h.currentRoomAuthority(ctx, roomID)
	if err != nil {
		return err
	}
	if found && authority.IsRecovering() && !authority.ExpiredAt(h.clock.Now()) {
		return protocolMessageError{roomID: roomID, message: "room authority recovering"}
	}
	if !found ||
		authority.InstanceID != h.instanceID ||
		!authority.IsActive() ||
		authority.ExpiredAt(h.clock.Now()) ||
		(authorityEpoch > 0 && authority.Epoch != authorityEpoch) {
		return protocolMessageError{roomID: roomID, message: "room authority unavailable"}
	}
	return nil
}

func (h *WebSocketHandler) reserveDistributedControlRequest(
	ctx context.Context,
	roomID string,
	meta controlEventMeta,
) (protocol.Envelope, bool, error) {
	if !h.isDistributedAuthorityMode() || meta.RequestID == "" || meta.IdempotencyPreReserved {
		return protocol.Envelope{}, false, nil
	}
	if h.controlRequests == nil {
		return protocol.Envelope{}, false, protocolMessageError{
			roomID:  roomID,
			message: "control idempotency unavailable",
		}
	}
	record, reserved, err := h.controlRequests.Reserve(ctx, roomID, meta.RequestID, meta.AuthorityEpoch)
	if err != nil {
		return protocol.Envelope{}, false, protocolMessageError{
			roomID:  roomID,
			message: "control idempotency unavailable",
		}
	}
	if reserved {
		return protocol.Envelope{}, false, nil
	}
	switch record.Status {
	case cache.ControlRequestStatusAccepted:
		envelope, err := decodeControlRequestEnvelope(record.Envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		return envelope, true, nil
	case cache.ControlRequestStatusRejected:
		message := record.Error
		if message == "" {
			message = "control rejected"
		}
		return protocol.Envelope{}, false, protocolMessageError{roomID: roomID, message: message}
	case cache.ControlRequestStatusPending:
		return protocol.Envelope{}, false, protocolMessageError{roomID: roomID, message: "room authority processing"}
	default:
		return protocol.Envelope{}, false, protocolMessageError{roomID: roomID, message: "control idempotency unavailable"}
	}
}

func (h *WebSocketHandler) finalizeDistributedControlAccepted(
	ctx context.Context,
	roomID string,
	meta controlEventMeta,
	seq int64,
	envelope protocol.Envelope,
) error {
	if !h.isDistributedAuthorityMode() || h.controlRequests == nil || meta.RequestID == "" {
		return nil
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, finalized, err := h.controlRequests.FinalizeAccepted(ctx, roomID, meta.RequestID, meta.AuthorityEpoch, seq, data)
	if err != nil {
		return err
	}
	if !finalized {
		return errors.New("control idempotency finalize rejected stale epoch")
	}
	return nil
}

func (h *WebSocketHandler) finalizeDistributedControlRejected(
	ctx context.Context,
	roomID string,
	meta controlEventMeta,
	seq int64,
	message string,
) {
	if !h.isDistributedAuthorityMode() || h.controlRequests == nil || meta.RequestID == "" {
		return
	}
	if _, _, err := h.controlRequests.FinalizeRejected(ctx, roomID, meta.RequestID, meta.AuthorityEpoch, seq, message); err != nil && h.debugSync {
		log.Printf("control idempotency rejected finalize failed room=%s request_id=%q epoch=%d err=%v", roomID, meta.RequestID, meta.AuthorityEpoch, err)
	}
}

func (h *WebSocketHandler) forgetDistributedControlRequest(ctx context.Context, roomID string, requestID string) {
	if !h.isDistributedAuthorityMode() || h.controlRequests == nil || requestID == "" {
		return
	}
	if err := h.controlRequests.Forget(ctx, roomID, requestID); err != nil && h.debugSync {
		log.Printf("control idempotency forget failed room=%s request_id=%q err=%v", roomID, requestID, err)
	}
}

func (h *WebSocketHandler) forwardControlEvent(
	ctx context.Context,
	client *room.ClientConnection,
	eventType string,
	roomID string,
	meta controlEventMeta,
	envelope protocol.Envelope,
	authorityInstanceID string,
	authorityEpoch int64,
) error {
	if h.roomControlBus == nil {
		err := protocolMessageError{
			roomID:  roomID,
			message: "room authority unavailable",
		}
		h.recordControlRejected(ctx, eventType, roomID, meta, err.message)
		return err
	}
	meta.AuthorityEpoch = authorityEpoch
	duplicate, handled, err := h.reserveDistributedControlRequest(ctx, roomID, meta)
	if err != nil {
		h.recordControlRejected(ctx, eventType, roomID, meta, controlErrorMessage(err))
		return err
	}
	if handled {
		return client.WriteJSON(ctx, duplicate)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	response, err := h.roomControlBus.RequestRoomControl(requestCtx, authorityInstanceID, eventbus.RoomControlRequest{
		SourceInstanceID: h.instanceID,
		RoomID:           roomID,
		UserID:           meta.UserID,
		DeviceID:         client.DeviceID(),
		ConnectionID:     client.ConnectionID(),
		Type:             eventType,
		Payload:          envelope.Payload,
		RequestID:        meta.RequestID,
		Seq:              meta.ClientSeq,
		AuthorityEpoch:   authorityEpoch,
		RequestedAtMs:    h.clock.NowUnixMilli(),
	})
	if err != nil {
		protocolErr := protocolMessageError{
			roomID:  roomID,
			message: "room authority unavailable",
		}
		h.forgetDistributedControlRequest(ctx, roomID, meta.RequestID)
		h.recordControlRejected(ctx, eventType, roomID, meta, err.Error())
		return protocolErr
	}
	if authorityEpoch > 0 && response.AuthorityEpoch > 0 && response.AuthorityEpoch != authorityEpoch {
		h.forgetDistributedControlRequest(ctx, roomID, meta.RequestID)
		protocolErr := protocolMessageError{
			roomID:  roomID,
			message: "room authority unavailable",
		}
		h.recordControlRejected(ctx, eventType, roomID, meta, "stale authority response")
		return protocolErr
	}
	if response.Error != "" {
		h.finalizeDistributedControlRejected(ctx, roomID, meta, response.Seq, response.Error)
		h.recordControlRejected(ctx, eventType, roomID, meta, response.Error)
		return protocolMessageError{
			roomID:  roomID,
			message: response.Error,
		}
	}
	if response.Type == "" {
		return nil
	}
	return client.WriteJSON(ctx, protocol.Envelope{
		Type:    response.Type,
		Payload: response.Payload,
	})
}

func (h *WebSocketHandler) handleAuthorityRoomControlRequest(
	ctx context.Context,
	request eventbus.RoomControlRequest,
) eventbus.RoomControlResponse {
	if !h.isDistributedAuthorityMode() {
		return eventbus.RoomControlResponse{Error: "room authority unavailable"}
	}
	if request.SourceInstanceID == h.instanceID {
		return eventbus.RoomControlResponse{Error: "self control forwarding is not allowed"}
	}
	authority, found, err := h.currentRoomAuthority(ctx, request.RoomID)
	if err != nil {
		return eventbus.RoomControlResponse{Error: err.Error()}
	}
	if !found || authority.InstanceID != h.instanceID ||
		!authority.IsActive() ||
		(request.AuthorityEpoch > 0 && authority.Epoch != request.AuthorityEpoch) {
		return eventbus.RoomControlResponse{Error: "room authority unavailable"}
	}
	if err := h.ensureLocalAuthorityEpoch(ctx, request.RoomID, request.AuthorityEpoch); err != nil {
		return eventbus.RoomControlResponse{Error: controlErrorMessage(err)}
	}
	if err := h.ensureActiveDeviceControlRequest(ctx, request); err != nil {
		return eventbus.RoomControlResponse{Error: err.Error()}
	}
	existingRoom, ok := h.roomManager.Get(request.RoomID)
	if !ok {
		return eventbus.RoomControlResponse{Error: "room not found"}
	}
	response, accepted, err := h.applyForwardedControlRequest(ctx, existingRoom, request)
	if err != nil {
		return eventbus.RoomControlResponse{Error: controlErrorMessage(err)}
	}
	if err := h.ensureLocalAuthorityEpoch(ctx, request.RoomID, request.AuthorityEpoch); err != nil {
		return eventbus.RoomControlResponse{Error: controlErrorMessage(err)}
	}
	_ = accepted
	return eventbus.RoomControlResponse{
		Type:           response.Type,
		Payload:        response.Payload,
		Seq:            responseSeq(response),
		AuthorityEpoch: request.AuthorityEpoch,
	}
}

func (h *WebSocketHandler) ensureActiveDeviceControlRequest(
	ctx context.Context,
	request eventbus.RoomControlRequest,
) error {
	if h.activeDevices == nil {
		return nil
	}
	lease, found, err := h.activeDevices.Get(ctx, request.RoomID, request.UserID)
	if err != nil {
		return err
	}
	if !found ||
		lease.DeviceID != request.DeviceID ||
		lease.ConnectionID != request.ConnectionID ||
		lease.InstanceID != request.SourceInstanceID {
		return errors.New("active room device required")
	}
	return nil
}

func (h *WebSocketHandler) applyForwardedControlRequest(
	ctx context.Context,
	existingRoom *room.Room,
	request eventbus.RoomControlRequest,
) (protocol.Envelope, bool, error) {
	envelope := protocol.Envelope{Type: request.Type, Payload: request.Payload}
	switch request.Type {
	case protocol.TypePlay:
		payload, err := protocol.DecodePlay(envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		meta := forwardedControlMeta(request, payload.RequestID, payload.PositionMs, payload.Seq)
		return h.applyLocalControlEvent(
			ctx,
			existingRoom,
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
	case protocol.TypePause:
		payload, err := protocol.DecodePause(envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		meta := forwardedControlMeta(request, payload.RequestID, payload.PositionMs, payload.Seq)
		return h.applyLocalControlEvent(
			ctx,
			existingRoom,
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
	case protocol.TypeSeek:
		payload, err := protocol.DecodeSeek(envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		meta := forwardedControlMeta(request, payload.RequestID, payload.PositionMs, payload.Seq)
		return h.applyLocalControlEvent(
			ctx,
			existingRoom,
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
	case protocol.TypeSetPlaybackRate:
		payload, err := protocol.DecodeSetPlaybackRate(envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		meta := forwardedControlMeta(request, payload.RequestID, payload.PositionMs, payload.Seq)
		return h.applyLocalControlEvent(
			ctx,
			existingRoom,
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
	case protocol.TypeEnded:
		payload, err := protocol.DecodeEnded(envelope)
		if err != nil {
			return protocol.Envelope{}, false, err
		}
		meta := forwardedControlMeta(request, payload.RequestID, payload.PositionMs, payload.Seq)
		return h.applyLocalControlEvent(
			ctx,
			existingRoom,
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
	default:
		return protocol.Envelope{}, false, protocol.ErrUnsupportedMessageType
	}
}

func forwardedControlMeta(
	request eventbus.RoomControlRequest,
	requestID string,
	positionMs int64,
	seq int64,
) controlEventMeta {
	return controlEventMeta{
		UserID:                 request.UserID,
		DeviceID:               request.DeviceID,
		ConnectionID:           request.ConnectionID,
		RequestID:              requestID,
		PositionMs:             positionMs,
		ClientSeq:              seq,
		AuthorityEpoch:         request.AuthorityEpoch,
		IdempotencyPreReserved: request.RequestID != "",
	}
}

func controlErrorMessage(err error) string {
	var protocolErr protocolMessageError
	if errors.As(err, &protocolErr) {
		return protocolErr.message
	}
	return err.Error()
}

func responseSeq(envelope protocol.Envelope) int64 {
	switch envelope.Type {
	case protocol.TypeRoomState:
		var payload protocol.RoomStatePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	case protocol.TypePlay:
		var payload protocol.PlayPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	case protocol.TypePause:
		var payload protocol.PausePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	case protocol.TypeSeek:
		var payload protocol.SeekPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	case protocol.TypeSetPlaybackRate:
		var payload protocol.SetPlaybackRatePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	case protocol.TypeEnded:
		var payload protocol.EndedPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err == nil {
			return payload.Seq
		}
	}
	return 0
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

func (h *WebSocketHandler) broadcastEnvelopeAndPublish(
	ctx context.Context,
	roomID string,
	seq int64,
	clients []*room.ClientConnection,
	envelope protocol.Envelope,
) error {
	err := h.broadcastEnvelope(ctx, roomID, clients, envelope)
	h.publishRoomBroadcast(ctx, roomID, seq, envelope, 0)
	return err
}

func (h *WebSocketHandler) publishRoomBroadcast(
	ctx context.Context,
	roomID string,
	seq int64,
	envelope protocol.Envelope,
	authorityEpoch int64,
) {
	if h.roomBroadcastBus == nil || h.instanceID == "" || roomID == "" || envelope.Type == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	publishCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if h.isDistributedAuthorityMode() && roomBroadcastRequiresAuthority(envelope.Type) {
		authority, found, err := h.currentRoomAuthority(ctx, roomID)
		if err != nil || !found || authority.InstanceID != h.instanceID || !authority.IsActive() {
			if err != nil {
				log.Printf("room broadcast authority check failed room=%s type=%s err=%v", roomID, envelope.Type, err)
			}
			return
		}
		if authorityEpoch > 0 && authority.Epoch != authorityEpoch {
			return
		}
		authorityEpoch = authority.Epoch
	}
	event := eventbus.RoomBroadcastEvent{
		InstanceID:     h.instanceID,
		RoomID:         roomID,
		Type:           envelope.Type,
		Payload:        envelope.Payload,
		Seq:            seq,
		AuthorityEpoch: authorityEpoch,
		PublishedAtMs:  h.clock.NowUnixMilli(),
	}
	if err := h.roomBroadcastBus.PublishRoomEnvelope(publishCtx, event); err != nil {
		log.Printf("room broadcast publish failed room=%s type=%s seq=%d err=%v", roomID, envelope.Type, seq, err)
	}
}

func (h *WebSocketHandler) handleRemoteRoomBroadcast(ctx context.Context, event eventbus.RoomBroadcastEvent) {
	if h == nil || h.roomManager == nil {
		return
	}
	if event.InstanceID == "" || event.InstanceID == h.instanceID || event.RoomID == "" || event.Type == "" {
		return
	}
	if h.isDistributedAuthorityMode() && event.AuthorityEpoch > 0 {
		authority, found, err := h.currentRoomAuthority(ctx, event.RoomID)
		if err == nil && found && (authority.IsRecovering() || event.AuthorityEpoch != authority.Epoch) {
			return
		}
	}
	clients := h.roomManager.Clients(event.RoomID)
	if len(clients) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	broadcastCtx, cancel := context.WithTimeout(ctx, defaultBroadcastEnqueueTimeout)
	defer cancel()

	envelope := protocol.Envelope{
		Type:    event.Type,
		Payload: event.Payload,
	}
	if event.Type == protocol.TypeRoomPresence {
		h.broadcastRoomPresenceToClients(broadcastCtx, event.RoomID, clients, roomPresenceReason(event.Payload), false)
		return
	}
	if err := h.broadcastEnvelope(broadcastCtx, event.RoomID, clients, envelope); err != nil {
		log.Printf("remote room broadcast delivery failed room=%s type=%s seq=%d source_instance=%s err=%v", event.RoomID, event.Type, event.Seq, event.InstanceID, err)
	}
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
	_ = h.broadcastEnvelopeAndPublish(ctx, result.State.RoomID, result.State.Seq, result.Remaining, protocol.Envelope{
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

func (h *WebSocketHandler) shouldRefreshPresence(client *room.ClientConnection, now time.Time) bool {
	if !h.isDistributedAuthorityMode() || h.presence == nil || client == nil {
		return false
	}
	return client.PresenceRefreshDue(now, h.presenceRefresh)
}

func (h *WebSocketHandler) upsertPresenceForClient(
	ctx context.Context,
	client *room.ClientConnection,
	state room.State,
) error {
	if !h.isDistributedAuthorityMode() || h.presence == nil || client == nil {
		return nil
	}
	roomID := client.RoomID()
	userID := client.UserID()
	deviceID := client.DeviceID()
	connectionID := client.ConnectionID()
	if roomID == "" || userID == "" || deviceID == "" || connectionID == "" {
		return nil
	}
	_, acquired, err := h.presence.Upsert(
		ctx,
		roomID,
		userID,
		roleForRoomUser(state, userID),
		deviceID,
		h.instanceID,
		connectionID,
		state.HostUserID == userID,
	)
	if err != nil {
		return err
	}
	if !acquired {
		return protocolMessageError{
			roomID:  roomID,
			message: "active room device already exists",
		}
	}
	client.MarkPresenceRefreshed(h.clock.Now())
	return nil
}

func (h *WebSocketHandler) refreshPresenceForClient(
	ctx context.Context,
	client *room.ClientConnection,
	reason string,
) error {
	_ = reason
	if !h.isDistributedAuthorityMode() || h.presence == nil || client == nil {
		return nil
	}
	roomID := client.RoomID()
	if roomID == "" {
		return nil
	}
	existingRoom, ok := h.roomManager.Get(roomID)
	if !ok {
		return protocolMessageError{roomID: roomID, message: "room not found"}
	}
	return h.upsertPresenceForClient(ctx, client, existingRoom.StateSnapshot())
}

func (h *WebSocketHandler) releasePresenceForClient(ctx context.Context, client *room.ClientConnection) {
	if !h.isDistributedAuthorityMode() || h.presence == nil || client == nil {
		return
	}
	roomID := client.RoomID()
	userID := client.UserID()
	deviceID := client.DeviceID()
	connectionID := client.ConnectionID()
	if roomID == "" || userID == "" || deviceID == "" || connectionID == "" {
		return
	}
	released, err := h.presence.ReleaseIfMatch(ctx, roomID, userID, deviceID, connectionID)
	if err != nil && h.debugSync {
		log.Printf("presence release failed room=%s user=%s device=%s connection=%s err=%v", roomID, userID, deviceID, connectionID, err)
	}
	if released && h.debugSync {
		log.Printf("presence released room=%s user=%s device=%s connection=%s", roomID, userID, deviceID, connectionID)
	}
}

func (h *WebSocketHandler) broadcastRoomPresenceForRoom(
	ctx context.Context,
	roomID string,
	reason string,
	publish bool,
) {
	if roomID == "" {
		return
	}
	clients := h.roomManager.Clients(roomID)
	if len(clients) == 0 && !publish {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	broadcastCtx, cancel := context.WithTimeout(ctx, defaultBroadcastEnqueueTimeout)
	defer cancel()
	h.broadcastRoomPresenceToClients(broadcastCtx, roomID, clients, reason, publish)
}

func (h *WebSocketHandler) broadcastRoomPresenceToClients(
	ctx context.Context,
	roomID string,
	clients []*room.ClientConnection,
	reason string,
	publish bool,
) {
	if h == nil || roomID == "" || (len(clients) == 0 && !publish) {
		return
	}
	snapshot, err := h.roomPresenceSnapshot(ctx, roomID)
	if err != nil {
		if h.debugSync && !errors.Is(err, cache.ErrRedisDisabled) {
			log.Printf("room presence snapshot failed room=%s err=%v", roomID, err)
		}
		return
	}
	for _, client := range clients {
		if client == nil {
			continue
		}
		_, _ = client.EnqueueJSON(ctx, protocol.Envelope{
			Type:    protocol.TypeRoomPresence,
			Payload: mustJSONRaw(roomPresencePayload(snapshot, client.UserID(), reason)),
		})
	}
	if publish {
		h.publishRoomBroadcast(ctx, roomID, 0, protocol.Envelope{
			Type:    protocol.TypeRoomPresence,
			Payload: mustJSONRaw(roomPresencePayload(snapshot, "", reason)),
		}, 0)
	}
}

func (h *WebSocketHandler) roomPresenceSnapshot(ctx context.Context, roomID string) (cache.PresenceSnapshot, error) {
	if h.presence == nil {
		return cache.PresenceSnapshot{}, cache.ErrRedisDisabled
	}
	return h.presence.Snapshot(ctx, roomID)
}

func roomPresencePayload(snapshot cache.PresenceSnapshot, selfUserID string, reason string) protocol.RoomPresencePayload {
	members := make([]protocol.RoomPresenceMemberPayload, 0, len(snapshot.Members))
	for _, member := range snapshot.Members {
		members = append(members, protocol.RoomPresenceMemberPayload{
			UserID: member.UserID,
			Role:   member.Role,
			IsHost: member.IsHost,
			IsSelf: selfUserID != "" && selfUserID == member.UserID,
		})
	}
	return protocol.RoomPresencePayload{
		RoomID:       snapshot.RoomID,
		OnlineCount:  snapshot.OnlineCount,
		Members:      members,
		Reason:       reason,
		ServerTimeMs: time.Now().UnixMilli(),
	}
}

func roomPresenceReason(data []byte) string {
	var payload protocol.RoomPresencePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	return payload.Reason
}

func roleForRoomUser(state room.State, userID string) string {
	if state.HostUserID == userID {
		return "host"
	}
	return "member"
}

func roomBroadcastRequiresAuthority(eventType string) bool {
	switch eventType {
	case protocol.TypePlay, protocol.TypePause, protocol.TypeSeek, protocol.TypeSetPlaybackRate, protocol.TypeEnded, protocol.TypeRoomState:
		return true
	default:
		return false
	}
}

func decodeControlRequestEnvelope(data []byte) (protocol.Envelope, error) {
	var envelope protocol.Envelope
	if len(data) == 0 {
		return protocol.Envelope{}, errors.New("missing control envelope")
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return protocol.Envelope{}, err
	}
	return envelope, nil
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

func (h *WebSocketHandler) forgetLocalControlRequest(roomID string, requestID string) {
	if h.isDistributedAuthorityMode() {
		return
	}
	h.forgetControlRequest(roomID, requestID)
}

func (h *WebSocketHandler) seedRecoveredControlRequests(
	ctx context.Context,
	roomID string,
	authorityEpoch int64,
	requests []recovery.RecoveredRequest,
	requestIDs []string,
) {
	seen := make(map[string]struct{}, len(requests)+len(requestIDs))
	for _, request := range requests {
		if request.RequestID == "" {
			continue
		}
		seen[request.RequestID] = struct{}{}
		_ = h.reserveControlRequest(roomID, request.RequestID, h.clock.Now())
		h.seedRecoveredDistributedControlRequest(ctx, roomID, authorityEpoch, request)
	}
	for _, requestID := range requestIDs {
		if requestID == "" {
			continue
		}
		if _, ok := seen[requestID]; ok {
			continue
		}
		_ = h.reserveControlRequest(roomID, requestID, h.clock.Now())
	}
}

func (h *WebSocketHandler) seedRecoveredDistributedControlRequest(
	ctx context.Context,
	roomID string,
	authorityEpoch int64,
	request recovery.RecoveredRequest,
) {
	if !h.isDistributedAuthorityMode() ||
		h.controlRequests == nil ||
		authorityEpoch <= 0 ||
		request.RequestID == "" ||
		len(request.Envelope) == 0 {
		return
	}
	record, reserved, err := h.controlRequests.Reserve(ctx, roomID, request.RequestID, authorityEpoch)
	if err != nil {
		if h.debugSync {
			log.Printf("recovered control idempotency reserve failed room=%s request_id=%q epoch=%d err=%v", roomID, request.RequestID, authorityEpoch, err)
		}
		return
	}
	if !reserved {
		if record.Status == cache.ControlRequestStatusAccepted {
			return
		}
		if record.Status != cache.ControlRequestStatusPending || record.AuthorityEpoch != authorityEpoch {
			if err := h.controlRequests.Forget(ctx, roomID, request.RequestID); err != nil {
				if h.debugSync {
					log.Printf("recovered control idempotency forget failed room=%s request_id=%q epoch=%d err=%v", roomID, request.RequestID, authorityEpoch, err)
				}
				return
			}
			record, reserved, err = h.controlRequests.Reserve(ctx, roomID, request.RequestID, authorityEpoch)
			if err != nil {
				if h.debugSync {
					log.Printf("recovered control idempotency re-reserve failed room=%s request_id=%q epoch=%d err=%v", roomID, request.RequestID, authorityEpoch, err)
				}
				return
			}
			if !reserved && record.Status == cache.ControlRequestStatusAccepted {
				return
			}
		}
	}
	if _, finalized, err := h.controlRequests.FinalizeAccepted(ctx, roomID, request.RequestID, authorityEpoch, request.Seq, request.Envelope); err != nil {
		if h.debugSync {
			log.Printf("recovered control idempotency accepted finalize failed room=%s request_id=%q epoch=%d err=%v", roomID, request.RequestID, authorityEpoch, err)
		}
	} else if !finalized && h.debugSync {
		log.Printf("recovered control idempotency accepted finalize skipped room=%s request_id=%q epoch=%d", roomID, request.RequestID, authorityEpoch)
	}
}

func authorityRecoveryErrorMessage(err error) string {
	if errors.Is(err, recovery.ErrAuthorityRecovering) {
		return "room authority recovering"
	}
	if errors.Is(err, recovery.ErrAuthorityActive) {
		return "room authority unavailable"
	}
	return err.Error()
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

	_ = h.broadcastEnvelopeAndPublish(ctx, roomID, 0, targets, protocol.Envelope{
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
			if err := h.refreshActiveDeviceLeaseForClient(ctx, client); err != nil {
				if h.debugSync && !errors.Is(err, context.Canceled) {
					log.Printf("active_device heartbeat refresh failed: %v", err)
				}
				h.releasePresenceForClient(context.Background(), client)
				_ = client.Close(websocket.StatusPolicyViolation, "active device lease lost")
				return
			}
			if h.shouldRefreshPresence(client, now) {
				if err := h.refreshPresenceForClient(ctx, client, "heartbeat"); err != nil {
					if h.debugSync && !errors.Is(err, context.Canceled) {
						log.Printf("presence heartbeat refresh failed: %v", err)
					}
					h.releasePresenceForClient(context.Background(), client)
					h.broadcastRoomPresenceForRoom(context.Background(), client.RoomID(), "lease_lost", true)
					_ = client.Close(websocket.StatusPolicyViolation, "presence lease lost")
					return
				}
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
