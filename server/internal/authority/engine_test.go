package authority

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/timeline"
)

func TestEngineAppliesAcceptedControlsWithoutTransportHandler(t *testing.T) {
	fixture := newEngineFixture(t)
	controls := []struct {
		eventType string
		payload   any
		wantSeq   int64
	}{
		{protocol.TypePlay, protocol.PlayPayload{RoomID: "ROOM01", UserID: "host", RequestID: "play-1", PositionMs: 1000, Seq: 1}, 2},
		{protocol.TypePause, protocol.PausePayload{RoomID: "ROOM01", UserID: "host", RequestID: "pause-1", PositionMs: 2000, Seq: 2}, 3},
		{protocol.TypeSeek, protocol.SeekPayload{RoomID: "ROOM01", UserID: "host", RequestID: "seek-1", PositionMs: 3000, Seq: 3}, 4},
		{protocol.TypeSetPlaybackRate, protocol.SetPlaybackRatePayload{RoomID: "ROOM01", UserID: "host", RequestID: "rate-1", PositionMs: 3000, PlaybackRate: 1.25, Seq: 4}, 5},
		{protocol.TypeEnded, protocol.EndedPayload{RoomID: "ROOM01", UserID: "host", RequestID: "ended-1", PositionMs: 120000, Seq: 5}, 6},
	}

	for _, control := range controls {
		response, err := fixture.engine.ApplyRoomControl(context.Background(), ApplyControlRequest{
			SourceInstanceID:       "roomserver-a",
			RoomID:                 "ROOM01",
			UserID:                 "host",
			DeviceID:               "host-device",
			ConnectionID:           "host-conn",
			Type:                   control.eventType,
			Payload:                mustRaw(t, control.payload),
			RequestID:              requestIDFromPayload(t, control.payload),
			Seq:                    control.wantSeq - 1,
			ExpectedAuthorityEpoch: 1,
			RequestedAtMs:          time.Now().UnixMilli(),
		})
		if err != nil {
			t.Fatalf("apply %s: %v", control.eventType, err)
		}
		if !response.Accepted || response.Type != control.eventType || response.Seq != control.wantSeq || response.Error != "" {
			t.Fatalf("unexpected %s response: %+v", control.eventType, response)
		}
	}

	if got := fixture.timeline.acceptedCount(); got != len(controls) {
		t.Fatalf("expected %d accepted timeline results, got %d", len(controls), got)
	}
	if got := fixture.broadcast.count(); got != len(controls) {
		t.Fatalf("expected %d NATS broadcast envelopes, got %d", len(controls), got)
	}
}

func TestEngineRejectsInvalidAuthorityControls(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*ApplyControlRequest)
		wantError string
		wantType  string
	}{
		{
			name: "active device mismatch",
			mutate: func(request *ApplyControlRequest) {
				request.DeviceID = "other-device"
			},
			wantError: "active room device required",
		},
		{
			name: "non host",
			mutate: func(request *ApplyControlRequest) {
				request.UserID = "viewer"
				request.DeviceID = "viewer-device"
				request.ConnectionID = "viewer-conn"
			},
			wantError: "only host can control playback",
		},
		{
			name: "seq mismatch",
			mutate: func(request *ApplyControlRequest) {
				request.Seq = 99
			},
			wantType: protocol.TypeRoomState,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newEngineFixture(t)
			request := fixture.playRequest("reject-"+tc.name, 1)
			tc.mutate(&request)

			response, err := fixture.engine.ApplyRoomControl(context.Background(), request)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if tc.wantError != "" {
				if response.Error != tc.wantError || response.Accepted {
					t.Fatalf("response = %+v, want error %q", response, tc.wantError)
				}
				return
			}
			if response.Type != tc.wantType || response.Accepted {
				t.Fatalf("response = %+v, want type %q without accepted", response, tc.wantType)
			}
		})
	}
}

func TestEngineTimelineUnavailableRollsBackAcceptedControl(t *testing.T) {
	fixture := newEngineFixture(t)
	fixture.timeline.failAccepted = true

	response, err := fixture.engine.ApplyRoomControl(context.Background(), fixture.playRequest("timeline-fails", 1))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if response.Error != "room timeline unavailable" || response.Accepted {
		t.Fatalf("unexpected timeline failure response: %+v", response)
	}
	state := fixture.roomManager.GetOrCreate("ROOM01").StateSnapshot()
	if state.Seq != 1 || !state.Paused {
		t.Fatalf("expected room state rollback to seq=1 paused=true, got %+v", state)
	}
	record := fixture.controlRequests.record("ROOM01", "timeline-fails")
	if record.Status == cache.ControlRequestStatusAccepted {
		t.Fatalf("expected request not to finalize accepted: %+v", record)
	}
	if got := fixture.broadcast.count(); got != 0 {
		t.Fatalf("expected no accepted broadcast on timeline failure, got %d", got)
	}
}

func TestEngineDuplicateAcceptedReturnsOriginalEnvelope(t *testing.T) {
	fixture := newEngineFixture(t)
	request := fixture.playRequest("duplicate-play", 1)
	first, err := fixture.engine.ApplyRoomControl(context.Background(), request)
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := fixture.engine.ApplyRoomControl(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate apply: %v", err)
	}
	if !second.Accepted || second.Type != first.Type || second.Seq != first.Seq || string(second.Payload) != string(first.Payload) {
		t.Fatalf("duplicate response = %+v, want original %+v", second, first)
	}
	if got := fixture.timeline.acceptedCount(); got != 1 {
		t.Fatalf("expected duplicate accepted not to write a second timeline result, got %d", got)
	}
}

func TestEngineDuplicatePendingReturnsProcessingWithoutAdvancingState(t *testing.T) {
	fixture := newEngineFixture(t)
	request := fixture.playRequest("pending-play", 1)
	_, reserved, err := fixture.controlRequests.Reserve(context.Background(), request.RoomID, request.RequestID, request.ExpectedAuthorityEpoch)
	if err != nil || !reserved {
		t.Fatalf("seed pending request: reserved=%v err=%v", reserved, err)
	}

	response, err := fixture.engine.ApplyRoomControl(context.Background(), request)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if response.Error != "room authority processing" || response.Accepted {
		t.Fatalf("duplicate pending response = %+v", response)
	}
	state := fixture.roomManager.GetOrCreate("ROOM01").StateSnapshot()
	if state.Seq != 1 || !state.Paused {
		t.Fatalf("expected duplicate pending not to advance room state, got %+v", state)
	}
	record := fixture.controlRequests.record("ROOM01", "pending-play")
	if record.Status != cache.ControlRequestStatusPending {
		t.Fatalf("expected pending request to remain pending, got %+v", record)
	}
	if got := fixture.timeline.acceptedCount(); got != 0 {
		t.Fatalf("expected duplicate pending not to write accepted timeline result, got %d", got)
	}
	if got := fixture.broadcast.count(); got != 0 {
		t.Fatalf("expected duplicate pending not to publish accepted broadcast, got %d", got)
	}
}

func TestEngineRecoveryFeedFailureDoesNotRegisterRoomState(t *testing.T) {
	fixture := newEngineFixture(t)
	fixture.roomManager = room.NewManager()
	fixture.timeline.readErr = errors.New("timeline unavailable")
	fixture.engine = fixture.newEngine()

	response, err := fixture.engine.ApplyRoomControl(context.Background(), fixture.playRequest("recovery-feed-fails", 1))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if response.Error != "room authority unavailable" || response.Accepted {
		t.Fatalf("unexpected recovery failure response: %+v", response)
	}
	if _, ok := fixture.roomManager.Get("ROOM01"); ok {
		t.Fatalf("expected failed recovery feed not to register room state")
	}
}

type engineFixture struct {
	roomManager     *room.Manager
	authority       *fakeEngineAuthority
	activeDevices   *fakeEngineActiveDevices
	controlRequests *fakeEngineControlRequests
	controlRates    *fakeEngineControlRates
	roomStore       *fakeEngineRoomStore
	timeline        *fakeEngineTimeline
	broadcast       *fakeEngineBroadcast
	engine          *Engine
}

func newEngineFixture(t *testing.T) *engineFixture {
	t.Helper()
	manager := room.NewManager()
	created := manager.RegisterCreatedRoomWithMedia("ROOM01", "host", "episode-1", int64Ptr(120000))
	host := room.NewClientConnection(nil)
	host.SetIdentity("host", "ROOM01")
	host.SetDeviceID("host-device")
	created.Join(host)
	viewer := room.NewClientConnection(nil)
	viewer.SetIdentity("viewer", "ROOM01")
	viewer.SetDeviceID("viewer-device")
	created.Join(viewer)

	fixture := &engineFixture{
		roomManager:     manager,
		authority:       newFakeEngineAuthority("ROOM01", "roomauthorityservice-1", 1),
		activeDevices:   newFakeEngineActiveDevices(),
		controlRequests: newFakeEngineControlRequests(),
		controlRates:    &fakeEngineControlRates{},
		roomStore:       &fakeEngineRoomStore{},
		timeline:        &fakeEngineTimeline{},
		broadcast:       &fakeEngineBroadcast{},
	}
	fixture.activeDevices.set("ROOM01", "host", cache.ActiveDeviceLease{
		DeviceID:     "host-device",
		InstanceID:   "roomserver-a",
		ConnectionID: "host-conn",
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	})
	fixture.activeDevices.set("ROOM01", "viewer", cache.ActiveDeviceLease{
		DeviceID:     "viewer-device",
		InstanceID:   "roomserver-a",
		ConnectionID: "viewer-conn",
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	})
	fixture.roomStore.bootstrap = roomapi.RuntimeBootstrapResult{
		Room:  roomapi.Room{RoomCode: "ROOM01", HostUserID: "host", MediaItemID: "episode-1"},
		Media: roomapi.Media{ID: "episode-1", DurationMs: int64Ptr(120000)},
	}
	fixture.engine = fixture.newEngine()
	return fixture
}

func (f *engineFixture) newEngine() *Engine {
	return NewEngine(
		EngineConfig{
			InstanceID:      "roomauthorityservice-1",
			SeekMinInterval: 250 * time.Millisecond,
		},
		f.roomManager,
		f.authority,
		f.activeDevices,
		f.controlRequests,
		f.controlRates,
		f.roomStore,
		f.timeline,
		nil,
		f.broadcast,
		nil,
	)
}

func (f *engineFixture) playRequest(requestID string, seq int64) ApplyControlRequest {
	return ApplyControlRequest{
		SourceInstanceID:       "roomserver-a",
		RoomID:                 "ROOM01",
		UserID:                 "host",
		DeviceID:               "host-device",
		ConnectionID:           "host-conn",
		Type:                   protocol.TypePlay,
		Payload:                mustRawMessage(protocol.PlayPayload{RoomID: "ROOM01", UserID: "host", RequestID: requestID, PositionMs: 1000, Seq: seq}),
		RequestID:              requestID,
		Seq:                    seq,
		ExpectedAuthorityEpoch: 1,
		RequestedAtMs:          time.Now().UnixMilli(),
	}
}

type fakeEngineAuthority struct {
	lease cache.RoomAuthorityLease
}

func newFakeEngineAuthority(roomID string, instanceID string, epoch int64) *fakeEngineAuthority {
	return &fakeEngineAuthority{lease: cache.RoomAuthorityLease{
		InstanceID:   instanceID,
		Epoch:        epoch,
		Status:       cache.RoomAuthorityStatusActive,
		LeaseUntilMs: time.Now().Add(time.Minute).UnixMilli(),
	}}
}

func (a *fakeEngineAuthority) GetAuthority(context.Context, string) (cache.RoomAuthorityLease, bool, error) {
	return a.lease, a.lease.InstanceID != "", nil
}

func (a *fakeEngineAuthority) ClaimAuthority(context.Context, string, string) (cache.RoomAuthorityLease, bool, error) {
	return a.lease, true, nil
}

func (a *fakeEngineAuthority) RenewAuthorityEpoch(context.Context, string, string, int64) (cache.RoomAuthorityLease, bool, error) {
	return a.lease, true, nil
}

func (a *fakeEngineAuthority) BeginRecovery(context.Context, string, string) (cache.RoomAuthorityLease, bool, error) {
	return a.lease, false, nil
}

func (a *fakeEngineAuthority) CompleteRecovery(context.Context, string, string, int64) (cache.RoomAuthorityLease, bool, error) {
	return a.lease, true, nil
}

type fakeEngineActiveDevices struct {
	mu     sync.Mutex
	leases map[string]cache.ActiveDeviceLease
}

func newFakeEngineActiveDevices() *fakeEngineActiveDevices {
	return &fakeEngineActiveDevices{leases: make(map[string]cache.ActiveDeviceLease)}
}

func (d *fakeEngineActiveDevices) set(roomID string, userID string, lease cache.ActiveDeviceLease) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.leases[roomID+":"+userID] = lease
}

func (d *fakeEngineActiveDevices) Get(ctx context.Context, roomID string, userID string) (cache.ActiveDeviceLease, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	lease, ok := d.leases[roomID+":"+userID]
	return lease, ok, nil
}

type fakeEngineControlRequests struct {
	mu      sync.Mutex
	records map[string]cache.ControlRequestRecord
}

func newFakeEngineControlRequests() *fakeEngineControlRequests {
	return &fakeEngineControlRequests{records: make(map[string]cache.ControlRequestRecord)}
}

func (r *fakeEngineControlRequests) Reserve(ctx context.Context, roomID string, requestID string, authorityEpoch int64) (cache.ControlRequestRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomID + ":" + requestID
	if record, ok := r.records[key]; ok {
		return record, false, nil
	}
	record := cache.ControlRequestRecord{RoomID: roomID, RequestID: requestID, Status: cache.ControlRequestStatusPending, AuthorityEpoch: authorityEpoch}
	r.records[key] = record
	return record, true, nil
}

func (r *fakeEngineControlRequests) FinalizeAccepted(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, envelope []byte) (cache.ControlRequestRecord, bool, error) {
	return r.finalize(roomID, requestID, authorityEpoch, cache.ControlRequestStatusAccepted, seq, envelope, "")
}

func (r *fakeEngineControlRequests) FinalizeRejected(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, message string) (cache.ControlRequestRecord, bool, error) {
	return r.finalize(roomID, requestID, authorityEpoch, cache.ControlRequestStatusRejected, seq, nil, message)
}

func (r *fakeEngineControlRequests) Forget(ctx context.Context, roomID string, requestID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.records, roomID+":"+requestID)
	return nil
}

func (r *fakeEngineControlRequests) record(roomID string, requestID string) cache.ControlRequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.records[roomID+":"+requestID]
}

func (r *fakeEngineControlRequests) finalize(roomID string, requestID string, authorityEpoch int64, status string, seq int64, envelope []byte, message string) (cache.ControlRequestRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := roomID + ":" + requestID
	record, ok := r.records[key]
	if !ok || record.AuthorityEpoch != authorityEpoch || record.Status != cache.ControlRequestStatusPending {
		return record, false, nil
	}
	record.Status = status
	record.Seq = seq
	record.Envelope = append([]byte(nil), envelope...)
	record.Error = message
	r.records[key] = record
	return record, true, nil
}

type fakeEngineControlRates struct{}

func (r *fakeEngineControlRates) Reserve(ctx context.Context, roomID string, controlType string, interval time.Duration, authorityEpoch int64) (cache.ControlRateReservation, bool, error) {
	return cache.ControlRateReservation{RoomID: roomID, ControlType: controlType, AuthorityEpoch: authorityEpoch, Token: "token"}, true, nil
}

func (r *fakeEngineControlRates) ReleaseIfMatch(ctx context.Context, reservation cache.ControlRateReservation) (bool, error) {
	return true, nil
}

type fakeEngineRoomStore struct {
	bootstrap roomapi.RuntimeBootstrapResult
}

func (s *fakeEngineRoomStore) RuntimeBootstrapByCode(ctx context.Context, roomCode string) (roomapi.RuntimeBootstrapResult, error) {
	return s.bootstrap, nil
}

type fakeEngineTimeline struct {
	mu           sync.Mutex
	results      []timeline.ControlResult
	events       []timeline.Event
	readErr      error
	failAccepted bool
}

func (t *fakeEngineTimeline) RecordControlResult(ctx context.Context, result timeline.ControlResult) (timeline.Event, error) {
	if result.Accepted && t.failAccepted {
		return timeline.Event{}, timeline.ErrTimelineUnavailable
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.results = append(t.results, result)
	return timeline.Event{EventType: timeline.EventTypeControlAccepted, RoomID: result.RoomID, Seq: result.Seq}, nil
}

func (t *fakeEngineTimeline) RecordMembershipResult(ctx context.Context, result timeline.MembershipResult) (timeline.Event, error) {
	return timeline.Event{}, nil
}

func (t *fakeEngineTimeline) ReadRoomRecoveryEvents(ctx context.Context, roomID string) ([]timeline.Event, error) {
	if t.readErr != nil {
		return nil, t.readErr
	}
	return append([]timeline.Event(nil), t.events...), nil
}

func (t *fakeEngineTimeline) acceptedCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := 0
	for _, result := range t.results {
		if result.Accepted {
			count++
		}
	}
	return count
}

type fakeEngineBroadcast struct {
	mu     sync.Mutex
	events []eventbus.RoomBroadcastEvent
}

func (b *fakeEngineBroadcast) PublishRoomEnvelope(ctx context.Context, event eventbus.RoomBroadcastEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
	return nil
}

func (b *fakeEngineBroadcast) SubscribeRoomBroadcasts(ctx context.Context, handler eventbus.RoomBroadcastHandler) error {
	return nil
}

func (b *fakeEngineBroadcast) Close() error {
	return nil
}

func (b *fakeEngineBroadcast) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}

func mustRaw(t *testing.T, value any) json.RawMessage {
	if t != nil {
		t.Helper()
	}
	data, err := json.Marshal(value)
	if err != nil && t != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return data
}

func mustRawMessage(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func requestIDFromPayload(t *testing.T, payload any) string {
	t.Helper()
	data := mustRaw(t, payload)
	var decoded struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode request id: %v", err)
	}
	return decoded.RequestID
}

func int64Ptr(value int64) *int64 {
	return &value
}
