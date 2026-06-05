package transport

import (
	"context"
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
	t.Helper()
	handler := NewWebSocketHandler(roomManager, true)
	handler.SetRoomBroadcastBus(instanceID, broadcastBus)
	handler.SetDistributedAuthorityRuntime(instanceID, authority, activeDevices, controlBus, timeline.NoopRecorder{})
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
