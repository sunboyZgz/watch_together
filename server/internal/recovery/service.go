package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
	"watch_together/server/internal/timeline"
)

var (
	ErrAuthorityActive     = errors.New("room authority is active")
	ErrAuthorityRecovering = errors.New("room authority recovering")
)

type AuthorityRegistry interface {
	BeginRecovery(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error)
	CompleteRecovery(ctx context.Context, roomID string, instanceID string, epoch int64) (cache.RoomAuthorityLease, bool, error)
	GetAuthority(ctx context.Context, roomID string) (cache.RoomAuthorityLease, bool, error)
}

type RoomDetailStore interface {
	RuntimeBootstrapByCode(ctx context.Context, roomCode string) (roomapi.RuntimeBootstrapResult, error)
}

type RecoverableRoomStore interface {
	ListRecoverableRoomCodes(ctx context.Context, limit int) ([]string, error)
}

type RoomStateWriter interface {
	SetRoomState(ctx context.Context, roomID string, state protocol.RoomStatePayload) error
}

type Config struct {
	InstanceID       string
	RecoveryTimeout  time.Duration
	ScannerBatchSize int
}

type Service struct {
	config      Config
	authority   AuthorityRegistry
	roomManager *room.Manager
	roomStore   RoomDetailStore
	scanStore   RecoverableRoomStore
	timeline    timeline.RecoveryReader
	stateWriter RoomStateWriter
	now         func() time.Time
}

type Result struct {
	Lease      cache.RoomAuthorityLease
	Recovered  bool
	State      room.State
	RequestIDs []string
	Requests   []RecoveredRequest
}

type RecoveredRequest struct {
	RequestID string
	Seq       int64
	Envelope  json.RawMessage
}

func NewService(
	config Config,
	authority AuthorityRegistry,
	roomManager *room.Manager,
	roomStore RoomDetailStore,
	timelineReader timeline.RecoveryReader,
	stateWriter RoomStateWriter,
) *Service {
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 5 * time.Second
	}
	if config.ScannerBatchSize <= 0 {
		config.ScannerBatchSize = 100
	}
	var scanStore RecoverableRoomStore
	if store, ok := roomStore.(RecoverableRoomStore); ok {
		scanStore = store
	}
	return &Service{
		config:      config,
		authority:   authority,
		roomManager: roomManager,
		roomStore:   roomStore,
		scanStore:   scanStore,
		timeline:    timelineReader,
		stateWriter: stateWriter,
		now:         time.Now,
	}
}

func (s *Service) TryRecoverRoomAuthority(ctx context.Context, roomID string, reason string) (Result, error) {
	if s == nil || s.authority == nil || s.roomManager == nil || s.roomStore == nil || s.timeline == nil {
		return Result{}, errors.New("authority recovery is unavailable")
	}
	if roomID == "" || s.config.InstanceID == "" {
		return Result{}, errors.New("roomID and instanceID are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, s.config.RecoveryTimeout)
	defer cancel()

	lease, started, err := s.authority.BeginRecovery(recoveryCtx, roomID, s.config.InstanceID)
	if err != nil {
		return Result{}, err
	}
	if !started {
		if lease.IsRecovering() && !lease.ExpiredAt(s.now()) {
			return Result{Lease: lease}, ErrAuthorityRecovering
		}
		if lease.IsActive() && lease.InstanceID == s.config.InstanceID {
			return Result{Lease: lease}, nil
		}
		return Result{Lease: lease}, ErrAuthorityActive
	}

	bootstrap, err := s.roomStore.RuntimeBootstrapByCode(recoveryCtx, roomID)
	if err != nil {
		return Result{Lease: lease}, fmt.Errorf("load room metadata: %w", err)
	}
	state := baseStateFromRoomBootstrap(bootstrap, s.now())

	events, err := s.timeline.ReadRoomRecoveryEvents(recoveryCtx, roomID)
	if err != nil {
		return Result{Lease: lease}, fmt.Errorf("read room recovery timeline: %w", err)
	}

	state, requests, err := RecoverStateFromEvents(state, events)
	if err != nil {
		return Result{Lease: lease}, err
	}
	s.roomManager.RegisterRecoveredRoom(state)
	if s.stateWriter != nil {
		_ = s.stateWriter.SetRoomState(recoveryCtx, roomID, roomStatePayload(state))
	}

	activeLease, completed, err := s.authority.CompleteRecovery(recoveryCtx, roomID, s.config.InstanceID, lease.Epoch)
	if err != nil {
		return Result{Lease: lease}, err
	}
	if !completed {
		return Result{Lease: activeLease}, ErrAuthorityRecovering
	}
	return Result{
		Lease:      activeLease,
		Recovered:  true,
		State:      state,
		RequestIDs: recoveredRequestIDs(requests),
		Requests:   requests,
	}, nil
}

func (s *Service) RunScanner(ctx context.Context, interval time.Duration) {
	if s == nil || s.scanStore == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ScanOnce(ctx)
		}
	}
}

func (s *Service) ScanOnce(ctx context.Context) (int, error) {
	if s == nil || s.scanStore == nil {
		return 0, nil
	}
	roomIDs, err := s.scanStore.ListRecoverableRoomCodes(ctx, s.config.ScannerBatchSize)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, roomID := range roomIDs {
		_, err := s.TryRecoverRoomAuthority(ctx, roomID, "scanner")
		if err == nil {
			recovered++
			continue
		}
		if errors.Is(err, ErrAuthorityActive) || errors.Is(err, ErrAuthorityRecovering) {
			continue
		}
	}
	return recovered, nil
}

func RecoverStateFromEvents(base room.State, events []timeline.Event) (room.State, []RecoveredRequest, error) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAtMs != events[j].OccurredAtMs {
			return events[i].OccurredAtMs < events[j].OccurredAtMs
		}
		if events[i].Seq != events[j].Seq {
			return events[i].Seq < events[j].Seq
		}
		return events[i].EventID < events[j].EventID
	})

	seenEvents := make(map[string]struct{}, len(events))
	seenRequests := make(map[string]struct{})
	requests := make([]RecoveredRequest, 0)
	state := base
	for _, event := range events {
		if event.EventID != "" {
			if _, ok := seenEvents[event.EventID]; ok {
				continue
			}
			seenEvents[event.EventID] = struct{}{}
		}
		if event.EventType != timeline.EventTypeControlAccepted {
			continue
		}
		next, request, err := applyAcceptedControlEvent(state, event)
		if err != nil {
			return room.State{}, nil, err
		}
		state = next
		if request.RequestID != "" {
			if _, ok := seenRequests[request.RequestID]; !ok {
				seenRequests[request.RequestID] = struct{}{}
				requests = append(requests, request)
			}
		}
	}
	return state, requests, nil
}

func applyAcceptedControlEvent(state room.State, event timeline.Event) (room.State, RecoveredRequest, error) {
	var envelope protocol.Envelope
	if err := json.Unmarshal(event.Payload, &envelope); err != nil {
		return room.State{}, RecoveredRequest{}, fmt.Errorf("decode accepted control envelope: %w", err)
	}
	if envelope.Type == "" {
		envelope.Type = event.ControlType
		envelope.Payload = event.Payload
	}
	switch envelope.Type {
	case protocol.TypePlay:
		var payload protocol.PlayPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return room.State{}, RecoveredRequest{}, err
		}
		state = applyPlaybackPayload(state, payload.RoomID, payload.UserID, payload.PositionMs, payload.Velocity, payload.ServerTimeMs, payload.Reason, payload.Seq)
		state.Paused = payload.Velocity == 0
		state.Ended = atMediaEnd(state)
		return state, recoveredRequest(payload.RequestID, payload.Seq, event.Payload), nil
	case protocol.TypePause:
		var payload protocol.PausePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return room.State{}, RecoveredRequest{}, err
		}
		state = applyPlaybackPayload(state, payload.RoomID, payload.UserID, payload.PositionMs, payload.Velocity, payload.ServerTimeMs, payload.Reason, payload.Seq)
		state.Paused = true
		state.Ended = state.Ended || atMediaEnd(state)
		return state, recoveredRequest(payload.RequestID, payload.Seq, event.Payload), nil
	case protocol.TypeSeek:
		var payload protocol.SeekPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return room.State{}, RecoveredRequest{}, err
		}
		state = applyPlaybackPayload(state, payload.RoomID, payload.UserID, payload.PositionMs, payload.Velocity, payload.ServerTimeMs, payload.Reason, payload.Seq)
		state.Paused = payload.Velocity == 0
		state.Ended = atMediaEnd(state)
		return state, recoveredRequest(payload.RequestID, payload.Seq, event.Payload), nil
	case protocol.TypeSetPlaybackRate:
		var payload protocol.SetPlaybackRatePayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return room.State{}, RecoveredRequest{}, err
		}
		state = applyPlaybackPayload(state, payload.RoomID, payload.UserID, payload.PositionMs, payload.Velocity, payload.ServerTimeMs, payload.Reason, payload.Seq)
		state.PlaybackRate = payload.PlaybackRate
		state.Paused = payload.Velocity == 0
		state.Ended = state.Ended || atMediaEnd(state)
		return state, recoveredRequest(payload.RequestID, payload.Seq, event.Payload), nil
	case protocol.TypeEnded:
		var payload protocol.EndedPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return room.State{}, RecoveredRequest{}, err
		}
		state = applyPlaybackPayload(state, payload.RoomID, payload.UserID, payload.PositionMs, payload.Velocity, payload.ServerTimeMs, payload.Reason, payload.Seq)
		state.Paused = true
		state.Ended = true
		state.Velocity = 0
		return state, recoveredRequest(payload.RequestID, payload.Seq, event.Payload), nil
	default:
		return room.State{}, RecoveredRequest{}, fmt.Errorf("unsupported accepted control type %q", envelope.Type)
	}
}

func applyPlaybackPayload(
	state room.State,
	roomID string,
	hostUserID string,
	positionMs int64,
	velocity float64,
	serverTimeMs int64,
	reason string,
	seq int64,
) room.State {
	if roomID != "" {
		state.RoomID = roomID
	}
	if hostUserID != "" {
		state.HostUserID = hostUserID
	}
	state.PositionMs = positionMs
	state.Velocity = velocity
	if serverTimeMs > 0 {
		state.ServerTimeMs = serverTimeMs
	}
	if reason != "" {
		state.Reason = reason
	}
	if seq > 0 {
		state.Seq = seq
	}
	if state.PlaybackRate <= 0 {
		state.PlaybackRate = 1.0
	}
	return state
}

func baseStateFromRoomBootstrap(detail roomapi.RuntimeBootstrapResult, now time.Time) room.State {
	return room.State{
		RoomID:          detail.Room.RoomCode,
		MediaID:         detail.Room.MediaItemID,
		MediaDurationMs: cloneDurationMs(detail.Media.DurationMs),
		HostUserID:      detail.Room.HostUserID,
		Paused:          true,
		Ended:           false,
		PositionMs:      0,
		Velocity:        0,
		ServerTimeMs:    now.UnixMilli(),
		PlaybackRate:    1.0,
		Seq:             1,
	}
}

func BaseStateFromRoomBootstrap(detail roomapi.RuntimeBootstrapResult, now time.Time) room.State {
	return baseStateFromRoomBootstrap(detail, now)
}

func roomStatePayload(state room.State) protocol.RoomStatePayload {
	return protocol.RoomStatePayload{
		RoomID:          state.RoomID,
		MediaID:         state.MediaID,
		MediaDurationMs: cloneDurationMs(state.MediaDurationMs),
		HostUserID:      state.HostUserID,
		Paused:          state.Paused,
		Ended:           state.Ended,
		PositionMs:      state.PositionMs,
		Velocity:        state.Velocity,
		ServerTimeMs:    state.ServerTimeMs,
		Reason:          state.Reason,
		PlaybackRate:    state.PlaybackRate,
		Seq:             state.Seq,
	}
}

func recoveredRequest(requestID string, seq int64, envelope []byte) RecoveredRequest {
	return RecoveredRequest{
		RequestID: requestID,
		Seq:       seq,
		Envelope:  cloneRawMessage(envelope),
	}
}

func recoveredRequestIDs(requests []RecoveredRequest) []string {
	ids := make([]string, 0, len(requests))
	for _, request := range requests {
		if request.RequestID != "" {
			ids = append(ids, request.RequestID)
		}
	}
	return ids
}

func cloneRawMessage(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}

func atMediaEnd(state room.State) bool {
	return state.MediaDurationMs != nil && state.PositionMs >= *state.MediaDurationMs
}

func cloneDurationMs(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
