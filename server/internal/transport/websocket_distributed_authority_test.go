package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/timeline"
)

func TestWebSocketDistributedAuthorityForwardsControlToAuthority(t *testing.T) {
	broadcastBus := eventbus.NewMemoryRoomBroadcastBus()
	defer broadcastBus.Close()
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManagerA := room.NewManager()
	roomA, err := roomManagerA.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomManagerB := room.NewManager()
	roomManagerB.RegisterCreatedRoom(roomA.ID(), "user_a", "sample_001")

	authority := newFakeAuthorityRegistry(roomA.ID(), "instance-a")
	activeDevices := newFakeActiveDeviceRegistry()
	serverA := newDistributedWebSocketServer(t, roomManagerA, "instance-a", authority, activeDevices, broadcastBus, controlBus)
	defer serverA.Close()
	serverB := newDistributedWebSocketServer(t, roomManagerB, "instance-b", authority, activeDevices, broadcastBus, controlBus)
	defer serverB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hostB := mustDialWebSocket(t, ctx, wsURL(serverB.URL), "user_a")
	defer hostB.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostB, roomA.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, hostB); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}

	mustSendEnvelope(t, ctx, hostB, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     roomA.ID(),
			UserID:     "user_a",
			RequestID:  "distributed-play",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, hostB, protocol.TypePlay, -1, 2)
	stateA := roomA.StateSnapshot()
	if stateA.Seq != 2 || stateA.Paused {
		t.Fatalf("expected authority room to apply play, got seq=%d paused=%t", stateA.Seq, stateA.Paused)
	}
	stateB := roomManagerB.GetOrCreate(roomA.ID()).StateSnapshot()
	if stateB.Seq != 1 || !stateB.Paused {
		t.Fatalf("expected non-authority room state not to mutate, got seq=%d paused=%t", stateB.Seq, stateB.Paused)
	}
}

func TestWebSocketDistributedAuthorityRejectsNonActiveDeviceControl(t *testing.T) {
	broadcastBus := eventbus.NewMemoryRoomBroadcastBus()
	defer broadcastBus.Close()
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManagerA := room.NewManager()
	roomA, err := roomManagerA.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomManagerB := room.NewManager()
	roomManagerB.RegisterCreatedRoom(roomA.ID(), "user_a", "sample_001")

	authority := newFakeAuthorityRegistry(roomA.ID(), "instance-a")
	activeDevices := newFakeActiveDeviceRegistry()
	serverA := newDistributedWebSocketServer(t, roomManagerA, "instance-a", authority, activeDevices, broadcastBus, controlBus)
	defer serverA.Close()
	serverB := newDistributedWebSocketServer(t, roomManagerB, "instance-b", authority, activeDevices, broadcastBus, controlBus)
	defer serverB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hostB := mustDialWebSocket(t, ctx, wsURL(serverB.URL), "user_a")
	defer hostB.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostB, roomA.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, hostB); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}
	activeDevices.forceLease(roomA.ID(), "user_a", cache.ActiveDeviceLease{
		DeviceID:     "other-device",
		InstanceID:   "instance-b",
		ConnectionID: "other-connection",
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	})

	mustSendEnvelope(t, ctx, hostB, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     roomA.ID(),
			UserID:     "user_a",
			RequestID:  "inactive-device-play",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	var envelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, hostB, &envelope)
	if envelope.Type != protocol.TypeError || envelope.Payload.Message != "active room device required" {
		t.Fatalf("unexpected inactive-device error: %+v", envelope)
	}
	if state := roomA.StateSnapshot(); state.Seq != 1 {
		t.Fatalf("expected authority state to stay unchanged, got seq=%d", state.Seq)
	}
}

func TestWebSocketDistributedAuthorityRejectsMissingAuthority(t *testing.T) {
	broadcastBus := eventbus.NewMemoryRoomBroadcastBus()
	defer broadcastBus.Close()
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	authority := &fakeAuthorityRegistry{}
	activeDevices := newFakeActiveDeviceRegistry()
	server := newDistributedWebSocketServer(t, roomManager, "instance-b", authority, activeDevices, broadcastBus, controlBus)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	host := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_a")
	defer host.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, host, createdRoom.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}

	mustSendEnvelope(t, ctx, host, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "missing-authority-play",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	var envelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, host, &envelope)
	if envelope.Type != protocol.TypeError || envelope.Payload.Message != "room authority unavailable" {
		t.Fatalf("unexpected missing-authority error: %+v", envelope)
	}
}

func TestWebSocketDistributedAuthorityRecoversMissingAuthorityOnControl(t *testing.T) {
	broadcastBus := eventbus.NewMemoryRoomBroadcastBus()
	defer broadcastBus.Close()
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	authority := &fakeAuthorityRegistry{leases: make(map[string]cache.RoomAuthorityLease)}
	activeDevices := newFakeActiveDeviceRegistry()
	recoverer := &fakeAuthorityRecoverer{
		authority:  authority,
		instanceID: "instance-b",
		epoch:      2,
	}
	server := newDistributedWebSocketServerWithRecovery(
		t,
		roomManager,
		"instance-b",
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		recoverer,
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	host := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_a")
	defer host.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, host, createdRoom.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}

	mustSendEnvelope(t, ctx, host, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "recover-play",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	assertControlBroadcast(t, ctx, host, protocol.TypePlay, -1, 2)
	if recoverer.calls == 0 {
		t.Fatalf("expected authority recovery to be attempted")
	}
	state := createdRoom.StateSnapshot()
	if state.Seq != 2 || state.Paused {
		t.Fatalf("expected recovered local authority to apply play, got %+v", state)
	}
}

func TestWebSocketDistributedAuthorityOutboxFailureRejectsAcceptedControl(t *testing.T) {
	broadcastBus := &recordingRoomBroadcastBus{}
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	authority := newFakeAuthorityRegistry(createdRoom.ID(), "instance-a")
	activeDevices := newFakeActiveDeviceRegistry()
	server := newDistributedWebSocketServerWithHardening(
		t,
		roomManager,
		"instance-a",
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		nil,
		newFakeControlRequestRegistry(),
		nil,
		failingTimelineRecorder{},
	)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	host := mustDialWebSocket(t, ctx, wsURL(server.URL), "user_a")
	defer host.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, host, createdRoom.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, host); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}

	mustSendEnvelope(t, ctx, host, protocol.Envelope{
		Type: protocol.TypePlay,
		Payload: mustJSONRaw(protocol.PlayPayload{
			RoomID:     createdRoom.ID(),
			UserID:     "user_a",
			RequestID:  "outbox-fails-play",
			PositionMs: 0,
			Seq:        1,
		}),
	})

	var envelope protocol.ErrorEnvelope
	readMessageAs(t, ctx, host, &envelope)
	if envelope.Type != protocol.TypeError || envelope.Payload.Message != "room timeline unavailable" {
		t.Fatalf("unexpected outbox failure response: %+v", envelope)
	}
	state := createdRoom.StateSnapshot()
	if state.Seq != 1 || !state.Paused {
		t.Fatalf("expected failed accepted control to roll back, got seq=%d paused=%t", state.Seq, state.Paused)
	}
	for _, event := range broadcastBus.events() {
		if event.Type == protocol.TypePlay {
			t.Fatalf("expected no accepted play broadcast when outbox fails, got %+v", event)
		}
	}
}

func TestWebSocketDistributedAuthorityFinalEpochFenceRejectsStaleApply(t *testing.T) {
	roomManager := room.NewManager()
	createdRoom, err := roomManager.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	client := room.NewClientConnection(nil)
	client.SetIdentity("user_a", createdRoom.ID())
	client.SetDeviceID("user_a-device")
	createdRoom.Join(client)

	authority := newFakeAuthorityRegistry(createdRoom.ID(), "instance-a")
	handler := NewWebSocketHandler(roomManager, true)
	handler.SetRoomBroadcastBus("instance-a", eventbus.NewDisabledRoomBroadcastBus())
	handler.SetDistributedAuthorityRuntime("instance-a", authority, newFakeActiveDeviceRegistry(), eventbus.NewDisabledRoomControlBus(), timeline.NoopRecorder{})
	handler.SetDistributedControlHardening(newFakeControlRequestRegistry(), nil, 0)

	meta := controlEventMeta{
		UserID:         "user_a",
		DeviceID:       "user_a-device",
		ConnectionID:   client.ConnectionID(),
		RequestID:      "stale-epoch-play",
		ClientSeq:      1,
		AuthorityEpoch: 1,
	}
	_, accepted, err := handler.applyLocalControlEvent(
		context.Background(),
		createdRoom,
		protocol.TypePlay,
		createdRoom.ID(),
		meta,
		func(existingRoom *room.Room) (room.State, []*room.ClientConnection, error) {
			state, clients, err := existingRoom.ApplyPlayIfSeq("user_a", 0, 1)
			authority.forceLease(createdRoom.ID(), cache.RoomAuthorityLease{
				InstanceID:   "instance-b",
				Epoch:        2,
				Status:       cache.RoomAuthorityStatusActive,
				LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
			})
			return state, clients, err
		},
		func(state room.State) protocol.Envelope {
			return controlEnvelopeFromState(protocol.TypePlay, state, "stale-epoch-play")
		},
	)
	if err == nil || accepted {
		t.Fatalf("expected stale epoch apply to be rejected, accepted=%t err=%v", accepted, err)
	}
	state := createdRoom.StateSnapshot()
	if state.Seq != 1 || !state.Paused {
		t.Fatalf("expected stale epoch apply to roll back, got seq=%d paused=%t", state.Seq, state.Paused)
	}
}

func TestWebSocketDistributedAuthorityBroadcastsUserLevelPresenceAcrossInstances(t *testing.T) {
	broadcastBus := eventbus.NewMemoryRoomBroadcastBus()
	defer broadcastBus.Close()
	controlBus := eventbus.NewMemoryRoomControlBus()
	defer controlBus.Close()

	roomManagerA := room.NewManager()
	roomA, err := roomManagerA.CreateRoom("user_a", "sample_001")
	if err != nil {
		t.Fatalf("create room A: %v", err)
	}
	roomManagerB := room.NewManager()
	roomManagerB.RegisterCreatedRoom(roomA.ID(), "user_a", "sample_001")

	authority := newFakeAuthorityRegistry(roomA.ID(), "instance-a")
	activeDevices := newFakeActiveDeviceRegistry()
	controlRequests := newFakeControlRequestRegistry()
	presence := newFakePresenceRegistry()
	serverA := newDistributedWebSocketServerWithHardening(
		t,
		roomManagerA,
		"instance-a",
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		nil,
		controlRequests,
		presence,
		timeline.NoopRecorder{},
	)
	defer serverA.Close()
	serverB := newDistributedWebSocketServerWithHardening(
		t,
		roomManagerB,
		"instance-b",
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		nil,
		controlRequests,
		presence,
		timeline.NoopRecorder{},
	)
	defer serverB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	hostA := mustDialWebSocket(t, ctx, wsURL(serverA.URL), "user_a")
	defer hostA.Close(websocket.StatusNormalClosure, "test done")
	viewerB := mustDialWebSocket(t, ctx, wsURL(serverB.URL), "user_b")
	defer viewerB.Close(websocket.StatusNormalClosure, "test done")

	mustJoinRoom(t, ctx, hostA, roomA.ID(), "user_a")
	if envelope := mustReadEnvelope(t, ctx, hostA); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected host room_state, got %s", envelope.Type)
	}
	hostPresence := mustReadEnvelope(t, ctx, hostA)
	assertRoomPresence(t, hostPresence, roomA.ID(), "user_a", 1)

	mustJoinRoom(t, ctx, viewerB, roomA.ID(), "user_b")
	if envelope := mustReadEnvelope(t, ctx, viewerB); envelope.Type != protocol.TypeRoomState {
		t.Fatalf("expected viewer room_state, got %s", envelope.Type)
	}
	viewerPresence := mustReadEnvelope(t, ctx, viewerB)
	assertRoomPresence(t, viewerPresence, roomA.ID(), "user_b", 2)

	hostUpdate := mustReadEnvelope(t, ctx, hostA)
	assertRoomPresence(t, hostUpdate, roomA.ID(), "user_a", 2)
}

func TestSeedRecoveredControlRequestsBackfillsAcceptedRegistry(t *testing.T) {
	handler := NewWebSocketHandler(room.NewManager(), true)
	handler.roomRuntimeMode = roomRuntimeModeDistributed
	controlRequests := newFakeControlRequestRegistry()
	handler.SetDistributedControlHardening(controlRequests, nil, 0)

	payload := mustJSONRaw(protocol.PlayPayload{
		RoomID:       "ROOM01",
		UserID:       "user_a",
		RequestID:    "req-recovered",
		PositionMs:   1000,
		Velocity:     1,
		ServerTimeMs: 2000,
		Reason:       "play",
		Seq:          7,
	})
	envelope := mustJSONRaw(protocol.Envelope{
		Type:    protocol.TypePlay,
		Payload: payload,
	})

	handler.seedRecoveredControlRequests(context.Background(), "ROOM01", 3, []recovery.RecoveredRequest{
		{
			RequestID: "req-recovered",
			Seq:       7,
			Envelope:  envelope,
		},
	}, nil)

	controlRequests.mu.Lock()
	record := controlRequests.records[roomMembershipKey("ROOM01", "req-recovered")]
	controlRequests.mu.Unlock()
	if record.Status != cache.ControlRequestStatusAccepted ||
		record.AuthorityEpoch != 3 ||
		record.Seq != 7 ||
		len(record.Envelope) == 0 {
		t.Fatalf("expected recovered request to be finalized as accepted, got %+v", record)
	}
	duplicate, reserved, err := controlRequests.Reserve(context.Background(), "ROOM01", "req-recovered", 3)
	if err != nil {
		t.Fatalf("reserve duplicate: %v", err)
	}
	if reserved || duplicate.Status != cache.ControlRequestStatusAccepted {
		t.Fatalf("expected duplicate reserve to return accepted record, reserved=%t record=%+v", reserved, duplicate)
	}
}

func newDistributedWebSocketServer(
	t *testing.T,
	roomManager *room.Manager,
	instanceID string,
	authority *fakeAuthorityRegistry,
	activeDevices *fakeActiveDeviceRegistry,
	broadcastBus eventbus.RoomBroadcastBus,
	controlBus eventbus.RoomControlBus,
) *httptest.Server {
	return newDistributedWebSocketServerWithRecovery(
		t,
		roomManager,
		instanceID,
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		nil,
	)
}

func newDistributedWebSocketServerWithRecovery(
	t *testing.T,
	roomManager *room.Manager,
	instanceID string,
	authority *fakeAuthorityRegistry,
	activeDevices *fakeActiveDeviceRegistry,
	broadcastBus eventbus.RoomBroadcastBus,
	controlBus eventbus.RoomControlBus,
	recoverer roomAuthorityRecovery,
) *httptest.Server {
	return newDistributedWebSocketServerWithHardening(
		t,
		roomManager,
		instanceID,
		authority,
		activeDevices,
		broadcastBus,
		controlBus,
		recoverer,
		newFakeControlRequestRegistry(),
		nil,
		timeline.NoopRecorder{},
	)
}

func newDistributedWebSocketServerWithHardening(
	t *testing.T,
	roomManager *room.Manager,
	instanceID string,
	authority *fakeAuthorityRegistry,
	activeDevices *fakeActiveDeviceRegistry,
	broadcastBus eventbus.RoomBroadcastBus,
	controlBus eventbus.RoomControlBus,
	recoverer roomAuthorityRecovery,
	controlRequests controlRequestRegistry,
	presence presenceRegistry,
	recorder timeline.Recorder,
) *httptest.Server {
	t.Helper()
	handler := NewWebSocketHandler(roomManager, true)
	handler.SetRoomBroadcastBus(instanceID, broadcastBus)
	handler.SetDistributedAuthorityRuntime(instanceID, authority, activeDevices, controlBus, recorder)
	handler.SetDistributedControlHardening(controlRequests, presence, 0)
	handler.SetRoomAuthorityRecovery(recoverer)
	if err := handler.SubscribeRoomBroadcasts(context.Background()); err != nil {
		t.Fatalf("subscribe broadcasts: %v", err)
	}
	if err := handler.SubscribeRoomControls(context.Background()); err != nil {
		t.Fatalf("subscribe controls: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", handler)
	return httptest.NewServer(mux)
}

type fakeAuthorityRegistry struct {
	mu     sync.Mutex
	leases map[string]cache.RoomAuthorityLease
}

func newFakeAuthorityRegistry(roomID string, instanceID string) *fakeAuthorityRegistry {
	registry := &fakeAuthorityRegistry{leases: make(map[string]cache.RoomAuthorityLease)}
	registry.leases[roomID] = cache.RoomAuthorityLease{
		InstanceID:   instanceID,
		Epoch:        1,
		Status:       cache.RoomAuthorityStatusActive,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	return registry
}

func (r *fakeAuthorityRegistry) GetAuthority(ctx context.Context, roomID string) (cache.RoomAuthorityLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease, ok := r.leases[roomID]
	return lease, ok, nil
}

func (r *fakeAuthorityRegistry) forceLease(roomID string, lease cache.RoomAuthorityLease) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.leases == nil {
		r.leases = make(map[string]cache.RoomAuthorityLease)
	}
	r.leases[roomID] = lease
}

type fakeAuthorityRecoverer struct {
	authority  *fakeAuthorityRegistry
	instanceID string
	epoch      int64
	calls      int
}

func (r *fakeAuthorityRecoverer) TryRecoverRoomAuthority(
	ctx context.Context,
	roomID string,
	reason string,
) (recovery.Result, error) {
	r.calls++
	lease := cache.RoomAuthorityLease{
		InstanceID:   r.instanceID,
		Epoch:        r.epoch,
		Status:       cache.RoomAuthorityStatusActive,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	r.authority.forceLease(roomID, lease)
	return recovery.Result{Lease: lease, Recovered: true}, nil
}

type fakeActiveDeviceRegistry struct {
	mu     sync.Mutex
	leases map[string]cache.ActiveDeviceLease
}

func newFakeActiveDeviceRegistry() *fakeActiveDeviceRegistry {
	return &fakeActiveDeviceRegistry{leases: make(map[string]cache.ActiveDeviceLease)}
}

func (r *fakeActiveDeviceRegistry) Acquire(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	instanceID string,
	connectionID string,
) (cache.ActiveDeviceLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomMembershipKey(roomID, userID)
	if existing, ok := r.leases[key]; ok && existing.DeviceID != deviceID {
		return existing, false, nil
	}
	lease := cache.ActiveDeviceLease{
		DeviceID:     deviceID,
		InstanceID:   instanceID,
		ConnectionID: connectionID,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	r.leases[key] = lease
	return lease, true, nil
}

func (r *fakeActiveDeviceRegistry) Get(ctx context.Context, roomID string, userID string) (cache.ActiveDeviceLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	lease, ok := r.leases[roomMembershipKey(roomID, userID)]
	return lease, ok, nil
}

func (r *fakeActiveDeviceRegistry) ReleaseIfMatch(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	connectionID string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomMembershipKey(roomID, userID)
	lease, ok := r.leases[key]
	if !ok {
		return false, nil
	}
	if lease.DeviceID != deviceID || lease.ConnectionID != connectionID {
		return false, nil
	}
	delete(r.leases, key)
	return true, nil
}

func (r *fakeActiveDeviceRegistry) forceLease(roomID string, userID string, lease cache.ActiveDeviceLease) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.leases[roomMembershipKey(roomID, userID)] = lease
}

type fakeControlRequestRegistry struct {
	mu      sync.Mutex
	records map[string]cache.ControlRequestRecord
}

func newFakeControlRequestRegistry() *fakeControlRequestRegistry {
	return &fakeControlRequestRegistry{records: make(map[string]cache.ControlRequestRecord)}
}

func (r *fakeControlRequestRegistry) Reserve(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
) (cache.ControlRequestRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomMembershipKey(roomID, requestID)
	if record, ok := r.records[key]; ok {
		return record, false, nil
	}
	record := cache.ControlRequestRecord{
		RoomID:         roomID,
		RequestID:      requestID,
		Status:         cache.ControlRequestStatusPending,
		AuthorityEpoch: authorityEpoch,
		LeaseUntilMs:   time.Now().Add(time.Minute).UnixMilli(),
	}
	r.records[key] = record
	return record, true, nil
}

func (r *fakeControlRequestRegistry) FinalizeAccepted(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
	seq int64,
	envelope []byte,
) (cache.ControlRequestRecord, bool, error) {
	return r.finalize(roomID, requestID, authorityEpoch, cache.ControlRequestStatusAccepted, seq, envelope, "")
}

func (r *fakeControlRequestRegistry) FinalizeRejected(
	ctx context.Context,
	roomID string,
	requestID string,
	authorityEpoch int64,
	seq int64,
	message string,
) (cache.ControlRequestRecord, bool, error) {
	return r.finalize(roomID, requestID, authorityEpoch, cache.ControlRequestStatusRejected, seq, nil, message)
}

func (r *fakeControlRequestRegistry) Forget(ctx context.Context, roomID string, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, roomMembershipKey(roomID, requestID))
	return nil
}

func (r *fakeControlRequestRegistry) finalize(
	roomID string,
	requestID string,
	authorityEpoch int64,
	status string,
	seq int64,
	envelope []byte,
	message string,
) (cache.ControlRequestRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomMembershipKey(roomID, requestID)
	record, ok := r.records[key]
	if !ok || record.AuthorityEpoch != authorityEpoch || record.Status != cache.ControlRequestStatusPending {
		return record, false, nil
	}
	record.Status = status
	record.Seq = seq
	record.Envelope = append([]byte(nil), envelope...)
	record.Error = message
	record.LeaseUntilMs = time.Now().Add(time.Minute).UnixMilli()
	r.records[key] = record
	return record, true, nil
}

type fakePresenceRegistry struct {
	mu      sync.Mutex
	members map[string]map[string]cache.PresenceMember
}

func newFakePresenceRegistry() *fakePresenceRegistry {
	return &fakePresenceRegistry{members: make(map[string]map[string]cache.PresenceMember)}
}

func (r *fakePresenceRegistry) Upsert(
	ctx context.Context,
	roomID string,
	userID string,
	role string,
	deviceID string,
	instanceID string,
	connectionID string,
	isHost bool,
) (cache.PresenceMember, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.members[roomID] == nil {
		r.members[roomID] = make(map[string]cache.PresenceMember)
	}
	if existing, ok := r.members[roomID][userID]; ok && existing.DeviceID != deviceID {
		return existing, false, nil
	}
	member := cache.PresenceMember{
		RoomID:       roomID,
		UserID:       userID,
		Role:         role,
		DeviceID:     deviceID,
		InstanceID:   instanceID,
		ConnectionID: connectionID,
		IsHost:       isHost,
		LastSeenMs:   time.Now().UnixMilli(),
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}
	r.members[roomID][userID] = member
	return member, true, nil
}

func (r *fakePresenceRegistry) ReleaseIfMatch(
	ctx context.Context,
	roomID string,
	userID string,
	deviceID string,
	connectionID string,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	member, ok := r.members[roomID][userID]
	if !ok || member.DeviceID != deviceID || member.ConnectionID != connectionID {
		return false, nil
	}
	delete(r.members[roomID], userID)
	return true, nil
}

func (r *fakePresenceRegistry) Snapshot(ctx context.Context, roomID string) (cache.PresenceSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	roomMembers := r.members[roomID]
	members := make([]cache.PresenceMember, 0, len(roomMembers))
	for _, member := range roomMembers {
		members = append(members, member)
	}
	return cache.PresenceSnapshot{
		RoomID:      roomID,
		OnlineCount: len(members),
		Members:     members,
	}, nil
}

type failingTimelineRecorder struct{}

func (failingTimelineRecorder) RecordTimelineEvent(context.Context, timeline.Event) error {
	return errors.New("outbox unavailable")
}

func assertRoomPresence(
	t *testing.T,
	envelope protocol.Envelope,
	roomID string,
	selfUserID string,
	onlineCount int,
) {
	t.Helper()
	if envelope.Type != protocol.TypeRoomPresence {
		t.Fatalf("expected room_presence, got %s", envelope.Type)
	}
	var payload protocol.RoomPresencePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal room_presence payload: %v", err)
	}
	if payload.RoomID != roomID || payload.OnlineCount != onlineCount {
		t.Fatalf("unexpected room_presence payload: %+v", payload)
	}
	foundSelf := false
	for _, member := range payload.Members {
		if member.UserID == selfUserID {
			foundSelf = member.IsSelf
		}
	}
	if !foundSelf {
		t.Fatalf("expected self member %s in room_presence payload: %+v", selfUserID, payload)
	}
}
