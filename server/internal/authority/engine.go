package authority

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/protocol"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/timeline"
)

const (
	defaultEngineRecordTimeout  = 300 * time.Millisecond
	defaultEnginePublishTimeout = time.Second
)

var errControlProcessing = errors.New("room authority processing")

type ActiveDeviceReader interface {
	Get(ctx context.Context, roomID string, userID string) (cache.ActiveDeviceLease, bool, error)
}

type ControlRequestStore interface {
	Reserve(ctx context.Context, roomID string, requestID string, authorityEpoch int64) (cache.ControlRequestRecord, bool, error)
	FinalizeAccepted(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, envelope []byte) (cache.ControlRequestRecord, bool, error)
	FinalizeRejected(ctx context.Context, roomID string, requestID string, authorityEpoch int64, seq int64, message string) (cache.ControlRequestRecord, bool, error)
	Forget(ctx context.Context, roomID string, requestID string) error
}

type ControlRateStore interface {
	Reserve(ctx context.Context, roomID string, controlType string, interval time.Duration, authorityEpoch int64) (cache.ControlRateReservation, bool, error)
	ReleaseIfMatch(ctx context.Context, reservation cache.ControlRateReservation) (bool, error)
}

type RoomStateWriter interface {
	SetRoomState(ctx context.Context, roomID string, state protocol.RoomStatePayload) error
}

type Recoverer interface {
	TryRecoverRoomAuthority(ctx context.Context, roomID string, reason string) (recovery.Result, error)
}

type EngineConfig struct {
	InstanceID      string
	SeekMinInterval time.Duration
	RecordTimeout   time.Duration
	PublishTimeout  time.Duration
	DebugSync       bool
	Now             func() time.Time
}

type Engine struct {
	config          EngineConfig
	roomManager     *room.Manager
	authority       LifecycleRegistry
	activeDevices   ActiveDeviceReader
	controlRequests ControlRequestStore
	controlRates    ControlRateStore
	roomStore       recovery.RoomDetailStore
	timeline        interface {
		timeline.ResultRecorder
		timeline.RecoveryReader
	}
	stateWriter RoomStateWriter
	broadcast   eventbus.RoomBroadcastBus
	recoverer   Recoverer
}

func NewEngine(
	config EngineConfig,
	roomManager *room.Manager,
	authority LifecycleRegistry,
	activeDevices ActiveDeviceReader,
	controlRequests ControlRequestStore,
	controlRates ControlRateStore,
	roomStore recovery.RoomDetailStore,
	timelineBoundary interface {
		timeline.ResultRecorder
		timeline.RecoveryReader
	},
	stateWriter RoomStateWriter,
	broadcast eventbus.RoomBroadcastBus,
	recoverer Recoverer,
) *Engine {
	if roomManager == nil {
		roomManager = room.NewManager()
	}
	if config.RecordTimeout <= 0 {
		config.RecordTimeout = defaultEngineRecordTimeout
	}
	if config.PublishTimeout <= 0 {
		config.PublishTimeout = defaultEnginePublishTimeout
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Engine{
		config:          config,
		roomManager:     roomManager,
		authority:       authority,
		activeDevices:   activeDevices,
		controlRequests: controlRequests,
		controlRates:    controlRates,
		roomStore:       roomStore,
		timeline:        timelineBoundary,
		stateWriter:     stateWriter,
		broadcast:       broadcast,
		recoverer:       recoverer,
	}
}

func (e *Engine) ApplyRoomControl(ctx context.Context, request ApplyControlRequest) (ApplyControlResponse, error) {
	if e == nil || e.roomManager == nil {
		return ApplyControlResponse{Error: "room authority unavailable"}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if request.RoomID == "" || request.UserID == "" || request.Type == "" {
		return ApplyControlResponse{Error: "invalid control request"}, nil
	}
	lease, err := e.ensureAuthority(ctx, request.RoomID, request.ExpectedAuthorityEpoch)
	if err != nil {
		return ApplyControlResponse{Error: authorityErrorMessage(err)}, nil
	}
	if err := e.ensureRoomState(ctx, request.RoomID); err != nil {
		return ApplyControlResponse{Error: "room authority unavailable"}, nil
	}
	if err := e.ensureActiveDevice(ctx, request); err != nil {
		e.recordRejected(ctx, request, request.Seq, err.Error())
		e.finalizeRejected(ctx, request, lease.Epoch, request.Seq, err.Error())
		return ApplyControlResponse{Seq: request.Seq, AuthorityEpoch: lease.Epoch, Error: err.Error()}, nil
	}
	existingRoom, ok := e.roomManager.Get(request.RoomID)
	if !ok {
		return ApplyControlResponse{AuthorityEpoch: lease.Epoch, Error: "room not found"}, nil
	}
	return e.apply(ctx, existingRoom, request, lease.Epoch)
}

func (e *Engine) ensureAuthority(ctx context.Context, roomID string, expectedEpoch int64) (cache.RoomAuthorityLease, error) {
	if e.authority == nil || e.config.InstanceID == "" {
		return cache.RoomAuthorityLease{}, errors.New("room authority unavailable")
	}
	lease, found, err := e.authority.GetAuthority(ctx, roomID)
	if err != nil {
		return cache.RoomAuthorityLease{}, err
	}
	if found && lease.IsRecovering() && !lease.ExpiredAt(e.config.Now()) {
		return lease, recovery.ErrAuthorityRecovering
	}
	if found &&
		lease.InstanceID == e.config.InstanceID &&
		lease.IsActive() &&
		!lease.ExpiredAt(e.config.Now()) &&
		(expectedEpoch <= 0 || lease.Epoch == expectedEpoch) {
		return lease, nil
	}
	if e.recoverer == nil {
		return lease, errors.New("room authority unavailable")
	}
	result, err := e.recoverer.TryRecoverRoomAuthority(ctx, roomID, "authority_rpc_control")
	if result.Recovered {
		e.seedRecoveredControlRequests(ctx, roomID, result.Lease.Epoch, result.Requests)
	}
	if err != nil && !errors.Is(err, recovery.ErrAuthorityActive) {
		return result.Lease, err
	}
	lease = result.Lease
	if lease.InstanceID == "" {
		return lease, errors.New("room authority unavailable")
	}
	if lease.InstanceID != e.config.InstanceID || !lease.IsActive() || (expectedEpoch > 0 && lease.Epoch != expectedEpoch) {
		return lease, errors.New("room authority unavailable")
	}
	return lease, nil
}

func (e *Engine) ensureRoomState(ctx context.Context, roomID string) error {
	if _, ok := e.roomManager.Get(roomID); ok {
		return nil
	}
	if e.roomStore == nil || e.timeline == nil {
		return errors.New("room authority unavailable")
	}
	bootstrap, err := e.roomStore.RuntimeBootstrapByCode(ctx, roomID)
	if err != nil {
		return err
	}
	base := recovery.BaseStateFromRoomBootstrap(bootstrap, e.config.Now())
	events, err := e.timeline.ReadRoomRecoveryEvents(ctx, roomID)
	if err != nil {
		return err
	}
	state, requests, err := recovery.RecoverStateFromEvents(base, events)
	if err != nil {
		return err
	}
	e.roomManager.RegisterRecoveredRoom(state)
	if e.stateWriter != nil {
		_ = e.stateWriter.SetRoomState(ctx, roomID, roomStatePayload(state))
	}
	lease, found, err := e.authority.GetAuthority(ctx, roomID)
	if err == nil && found {
		e.seedRecoveredControlRequests(ctx, roomID, lease.Epoch, requests)
	}
	return nil
}

func (e *Engine) ensureActiveDevice(ctx context.Context, request ApplyControlRequest) error {
	if e.activeDevices == nil {
		return nil
	}
	lease, found, err := e.activeDevices.Get(ctx, request.RoomID, request.UserID)
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

func (e *Engine) apply(ctx context.Context, existingRoom *room.Room, request ApplyControlRequest, authorityEpoch int64) (ApplyControlResponse, error) {
	previous := existingRoom.StateSnapshot()
	duplicate, handled, err := e.reserveControlRequest(ctx, request, authorityEpoch)
	if err != nil {
		if !errors.Is(err, errControlProcessing) {
			e.recordRejected(ctx, request, previous.Seq, controlErrorMessage(err))
		}
		return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: controlErrorMessage(err)}, nil
	}
	if handled {
		return envelopeResponse(duplicate, authorityEpoch), nil
	}
	if request.Seq != previous.Seq {
		envelope := protocol.Envelope{Type: protocol.TypeRoomState, Payload: mustJSONRaw(roomStatePayload(previous))}
		message := room.ErrSeqMismatch.Error()
		e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
		e.recordRejected(ctx, request, previous.Seq, message)
		return ApplyControlResponse{Accepted: false, Type: envelope.Type, Payload: envelope.Payload, Seq: previous.Seq, AuthorityEpoch: authorityEpoch}, nil
	}

	var rateReservation cache.ControlRateReservation
	rateReserved := false
	if request.Type == protocol.TypeSeek && e.config.SeekMinInterval > 0 {
		if e.controlRates == nil {
			message := "control rate limiter unavailable"
			e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
			e.recordRejected(ctx, request, previous.Seq, message)
			return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: message}, nil
		}
		reservation, ok, err := e.controlRates.Reserve(ctx, request.RoomID, protocol.TypeSeek, e.config.SeekMinInterval, authorityEpoch)
		if err != nil {
			message := "control rate limiter unavailable"
			e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
			e.recordRejected(ctx, request, previous.Seq, message)
			return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: message}, nil
		}
		if !ok {
			envelope := protocol.Envelope{Type: protocol.TypeRoomState, Payload: mustJSONRaw(roomStatePayload(previous))}
			message := "control rate limited"
			e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
			e.recordRejected(ctx, request, previous.Seq, message)
			return ApplyControlResponse{Accepted: false, Type: envelope.Type, Payload: envelope.Payload, Seq: previous.Seq, AuthorityEpoch: authorityEpoch}, nil
		}
		rateReservation = reservation
		rateReserved = true
	}

	state, envelope, err := e.applyControl(existingRoom, request)
	if err != nil {
		if rateReserved {
			_, _ = e.controlRates.ReleaseIfMatch(ctx, rateReservation)
		}
		if errors.Is(err, room.ErrSeqMismatch) {
			latest := existingRoom.StateSnapshot()
			envelope := protocol.Envelope{Type: protocol.TypeRoomState, Payload: mustJSONRaw(roomStatePayload(latest))}
			e.finalizeRejected(ctx, request, authorityEpoch, latest.Seq, err.Error())
			e.recordRejected(ctx, request, latest.Seq, err.Error())
			return ApplyControlResponse{Type: envelope.Type, Payload: envelope.Payload, Seq: latest.Seq, AuthorityEpoch: authorityEpoch}, nil
		}
		message := err.Error()
		if errors.Is(err, room.ErrNotHost) {
			message = "only host can control playback"
		}
		e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
		e.recordRejected(ctx, request, previous.Seq, message)
		return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: message}, nil
	}

	if err := e.ensureAuthorityEpoch(ctx, request.RoomID, authorityEpoch); err != nil {
		existingRoom.RestoreState(previous)
		e.cacheState(ctx, previous)
		message := authorityErrorMessage(err)
		e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
		e.recordRejected(ctx, request, previous.Seq, message)
		return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: message}, nil
	}
	if err := e.recordAccepted(ctx, request, state.Seq, envelope); err != nil {
		existingRoom.RestoreState(previous)
		e.cacheState(ctx, previous)
		message := "room timeline unavailable"
		e.finalizeRejected(ctx, request, authorityEpoch, previous.Seq, message)
		return ApplyControlResponse{Seq: previous.Seq, AuthorityEpoch: authorityEpoch, Error: message}, nil
	}
	if err := e.finalizeAccepted(ctx, request, authorityEpoch, state.Seq, envelope); err != nil && e.config.DebugSync {
		log.Printf("authority engine accepted finalize failed room=%s request_id=%q epoch=%d err=%v", request.RoomID, request.RequestID, authorityEpoch, err)
	}
	e.cacheState(ctx, state)
	e.publishAccepted(ctx, request.RoomID, state.Seq, authorityEpoch, envelope)
	return ApplyControlResponse{
		Accepted:       true,
		Type:           envelope.Type,
		Payload:        envelope.Payload,
		Seq:            state.Seq,
		AuthorityEpoch: authorityEpoch,
	}, nil
}

func (e *Engine) applyControl(existingRoom *room.Room, request ApplyControlRequest) (room.State, protocol.Envelope, error) {
	envelope := protocol.Envelope{Type: request.Type, Payload: request.Payload}
	switch request.Type {
	case protocol.TypePlay:
		payload, err := protocol.DecodePlay(envelope)
		if err != nil {
			return room.State{}, protocol.Envelope{}, err
		}
		state, _, err := existingRoom.ApplyPlayIfSeq(request.UserID, payload.PositionMs, request.Seq)
		if err != nil {
			return state, protocol.Envelope{}, err
		}
		return state, controlEnvelopeFromState(protocol.TypePlay, state, payload.RequestID), nil
	case protocol.TypePause:
		payload, err := protocol.DecodePause(envelope)
		if err != nil {
			return room.State{}, protocol.Envelope{}, err
		}
		state, _, err := existingRoom.ApplyPauseIfSeq(request.UserID, payload.PositionMs, request.Seq)
		if err != nil {
			return state, protocol.Envelope{}, err
		}
		return state, controlEnvelopeFromState(protocol.TypePause, state, payload.RequestID), nil
	case protocol.TypeSeek:
		payload, err := protocol.DecodeSeek(envelope)
		if err != nil {
			return room.State{}, protocol.Envelope{}, err
		}
		state, _, err := existingRoom.ApplySeekIfSeq(request.UserID, payload.PositionMs, request.Seq)
		if err != nil {
			return state, protocol.Envelope{}, err
		}
		return state, controlEnvelopeFromState(protocol.TypeSeek, state, payload.RequestID), nil
	case protocol.TypeSetPlaybackRate:
		payload, err := protocol.DecodeSetPlaybackRate(envelope)
		if err != nil {
			return room.State{}, protocol.Envelope{}, err
		}
		state, _, err := existingRoom.ApplyPlaybackRateIfSeq(request.UserID, payload.PlaybackRate, request.Seq)
		if err != nil {
			return state, protocol.Envelope{}, err
		}
		return state, controlEnvelopeFromState(protocol.TypeSetPlaybackRate, state, payload.RequestID), nil
	case protocol.TypeEnded:
		payload, err := protocol.DecodeEnded(envelope)
		if err != nil {
			return room.State{}, protocol.Envelope{}, err
		}
		state, _, err := existingRoom.ApplyEndedIfSeq(request.UserID, payload.PositionMs, request.Seq)
		if err != nil {
			return state, protocol.Envelope{}, err
		}
		return state, controlEnvelopeFromState(protocol.TypeEnded, state, payload.RequestID), nil
	default:
		return room.State{}, protocol.Envelope{}, protocol.ErrUnsupportedMessageType
	}
}

func (e *Engine) reserveControlRequest(ctx context.Context, request ApplyControlRequest, authorityEpoch int64) (protocol.Envelope, bool, error) {
	if request.RequestID == "" {
		return protocol.Envelope{}, false, nil
	}
	if e.controlRequests == nil {
		return protocol.Envelope{}, false, errors.New("control idempotency unavailable")
	}
	record, reserved, err := e.controlRequests.Reserve(ctx, request.RoomID, request.RequestID, authorityEpoch)
	if err != nil {
		return protocol.Envelope{}, false, errors.New("control idempotency unavailable")
	}
	if reserved {
		return protocol.Envelope{}, false, nil
	}
	switch record.Status {
	case cache.ControlRequestStatusAccepted:
		if len(record.Envelope) == 0 {
			return protocol.Envelope{}, false, nil
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal(record.Envelope, &envelope); err != nil {
			return protocol.Envelope{}, false, err
		}
		return envelope, true, nil
	case cache.ControlRequestStatusRejected:
		return protocol.Envelope{}, false, errors.New(record.Error)
	case cache.ControlRequestStatusPending:
		return protocol.Envelope{}, false, errControlProcessing
	default:
		return protocol.Envelope{}, false, nil
	}
}

func (e *Engine) ensureAuthorityEpoch(ctx context.Context, roomID string, authorityEpoch int64) error {
	lease, found, err := e.authority.GetAuthority(ctx, roomID)
	if err != nil {
		return err
	}
	if found && lease.IsRecovering() && !lease.ExpiredAt(e.config.Now()) {
		return recovery.ErrAuthorityRecovering
	}
	if !found ||
		lease.InstanceID != e.config.InstanceID ||
		!lease.IsActive() ||
		lease.ExpiredAt(e.config.Now()) ||
		(authorityEpoch > 0 && lease.Epoch != authorityEpoch) {
		return errors.New("room authority unavailable")
	}
	return nil
}

func (e *Engine) recordAccepted(ctx context.Context, request ApplyControlRequest, seq int64, envelope protocol.Envelope) error {
	if e.timeline == nil || request.RoomID == "" {
		return nil
	}
	recordCtx, cancel := context.WithTimeout(contextOrBackground(ctx), e.config.RecordTimeout)
	defer cancel()
	_, err := e.timeline.RecordControlResult(recordCtx, timeline.ControlResult{
		RoomID:       request.RoomID,
		UserID:       request.UserID,
		DeviceID:     request.DeviceID,
		ConnectionID: request.ConnectionID,
		InstanceID:   e.config.InstanceID,
		ControlType:  request.Type,
		Seq:          seq,
		Accepted:     true,
		Payload:      envelope,
	})
	return err
}

func (e *Engine) recordRejected(ctx context.Context, request ApplyControlRequest, seq int64, reason string) {
	if e.timeline == nil || request.RoomID == "" {
		return
	}
	recordCtx, cancel := context.WithTimeout(contextOrBackground(ctx), e.config.RecordTimeout)
	defer cancel()
	_, _ = e.timeline.RecordControlResult(recordCtx, timeline.ControlResult{
		RoomID:       request.RoomID,
		UserID:       request.UserID,
		DeviceID:     request.DeviceID,
		ConnectionID: request.ConnectionID,
		InstanceID:   e.config.InstanceID,
		ControlType:  request.Type,
		Seq:          seq,
		Accepted:     false,
		Payload: map[string]any{
			"type":      request.Type,
			"requestId": request.RequestID,
			"reason":    reason,
		},
	})
}

func (e *Engine) finalizeAccepted(ctx context.Context, request ApplyControlRequest, authorityEpoch int64, seq int64, envelope protocol.Envelope) error {
	if e.controlRequests == nil || request.RequestID == "" {
		return nil
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, finalized, err := e.controlRequests.FinalizeAccepted(ctx, request.RoomID, request.RequestID, authorityEpoch, seq, data)
	if err != nil {
		return err
	}
	if !finalized {
		return errors.New("control idempotency finalize rejected stale epoch")
	}
	return nil
}

func (e *Engine) finalizeRejected(ctx context.Context, request ApplyControlRequest, authorityEpoch int64, seq int64, message string) {
	if e.controlRequests == nil || request.RequestID == "" {
		return
	}
	_, _, _ = e.controlRequests.FinalizeRejected(ctx, request.RoomID, request.RequestID, authorityEpoch, seq, message)
}

func (e *Engine) seedRecoveredControlRequests(ctx context.Context, roomID string, authorityEpoch int64, requests []recovery.RecoveredRequest) {
	if e.controlRequests == nil || authorityEpoch <= 0 {
		return
	}
	for _, request := range requests {
		if request.RequestID == "" {
			continue
		}
		record, reserved, err := e.controlRequests.Reserve(ctx, roomID, request.RequestID, authorityEpoch)
		if err != nil {
			continue
		}
		if !reserved && record.Status == cache.ControlRequestStatusAccepted {
			continue
		}
		if !reserved && record.Status != cache.ControlRequestStatusPending {
			_ = e.controlRequests.Forget(ctx, roomID, request.RequestID)
			if _, _, err = e.controlRequests.Reserve(ctx, roomID, request.RequestID, authorityEpoch); err != nil {
				continue
			}
		}
		_, _, _ = e.controlRequests.FinalizeAccepted(ctx, roomID, request.RequestID, authorityEpoch, request.Seq, request.Envelope)
	}
}

func (e *Engine) cacheState(ctx context.Context, state room.State) {
	if e.stateWriter == nil || state.RoomID == "" {
		return
	}
	_ = e.stateWriter.SetRoomState(ctx, state.RoomID, roomStatePayload(state))
}

func (e *Engine) publishAccepted(ctx context.Context, roomID string, seq int64, authorityEpoch int64, envelope protocol.Envelope) {
	if e.broadcast == nil || e.config.InstanceID == "" || roomID == "" || envelope.Type == "" {
		return
	}
	publishCtx, cancel := context.WithTimeout(contextOrBackground(ctx), e.config.PublishTimeout)
	defer cancel()
	if err := e.broadcast.PublishRoomEnvelope(publishCtx, eventbus.RoomBroadcastEvent{
		InstanceID:     e.config.InstanceID,
		RoomID:         roomID,
		Type:           envelope.Type,
		Payload:        envelope.Payload,
		Seq:            seq,
		AuthorityEpoch: authorityEpoch,
		PublishedAtMs:  e.config.Now().UnixMilli(),
	}); err != nil && e.config.DebugSync {
		log.Printf("authority engine broadcast publish failed room=%s type=%s seq=%d err=%v", roomID, envelope.Type, seq, err)
	}
}

func envelopeResponse(envelope protocol.Envelope, authorityEpoch int64) ApplyControlResponse {
	return ApplyControlResponse{
		Accepted:       envelope.Type != protocol.TypeRoomState && envelope.Type != "",
		Type:           envelope.Type,
		Payload:        envelope.Payload,
		Seq:            responseSeq(envelope),
		AuthorityEpoch: authorityEpoch,
	}
}

func controlErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func authorityErrorMessage(err error) string {
	if errors.Is(err, recovery.ErrAuthorityRecovering) {
		return "room authority recovering"
	}
	return "room authority unavailable"
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
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

func controlEnvelopeFromState(eventType string, state room.State, requestID string) protocol.Envelope {
	switch eventType {
	case protocol.TypePlay:
		return protocol.Envelope{Type: protocol.TypePlay, Payload: mustJSONRaw(playPayloadFromState(state, requestID))}
	case protocol.TypePause:
		return protocol.Envelope{Type: protocol.TypePause, Payload: mustJSONRaw(pausePayloadFromState(state, requestID))}
	case protocol.TypeSeek:
		return protocol.Envelope{Type: protocol.TypeSeek, Payload: mustJSONRaw(seekPayloadFromState(state, requestID))}
	case protocol.TypeSetPlaybackRate:
		return protocol.Envelope{Type: protocol.TypeSetPlaybackRate, Payload: mustJSONRaw(setPlaybackRatePayloadFromState(state, requestID))}
	case protocol.TypeEnded:
		return protocol.Envelope{Type: protocol.TypeEnded, Payload: mustJSONRaw(endedPayloadFromState(state, requestID))}
	default:
		return protocol.Envelope{Type: protocol.TypeRoomState, Payload: mustJSONRaw(roomStatePayload(state))}
	}
}

func playPayloadFromState(state room.State, requestID string) protocol.PlayPayload {
	return protocol.PlayPayload{RoomID: state.RoomID, UserID: state.HostUserID, RequestID: requestID, PositionMs: state.PositionMs, Velocity: state.Velocity, ServerTimeMs: state.ServerTimeMs, Reason: state.Reason, Seq: state.Seq}
}

func pausePayloadFromState(state room.State, requestID string) protocol.PausePayload {
	return protocol.PausePayload{RoomID: state.RoomID, UserID: state.HostUserID, RequestID: requestID, PositionMs: state.PositionMs, Velocity: state.Velocity, ServerTimeMs: state.ServerTimeMs, Reason: state.Reason, Seq: state.Seq}
}

func seekPayloadFromState(state room.State, requestID string) protocol.SeekPayload {
	return protocol.SeekPayload{RoomID: state.RoomID, UserID: state.HostUserID, RequestID: requestID, PositionMs: state.PositionMs, Velocity: state.Velocity, ServerTimeMs: state.ServerTimeMs, Reason: state.Reason, Seq: state.Seq}
}

func setPlaybackRatePayloadFromState(state room.State, requestID string) protocol.SetPlaybackRatePayload {
	return protocol.SetPlaybackRatePayload{RoomID: state.RoomID, UserID: state.HostUserID, RequestID: requestID, PositionMs: state.PositionMs, Velocity: state.Velocity, ServerTimeMs: state.ServerTimeMs, Reason: state.Reason, PlaybackRate: state.PlaybackRate, Seq: state.Seq}
}

func endedPayloadFromState(state room.State, requestID string) protocol.EndedPayload {
	return protocol.EndedPayload{RoomID: state.RoomID, UserID: state.HostUserID, RequestID: requestID, PositionMs: state.PositionMs, Velocity: state.Velocity, ServerTimeMs: state.ServerTimeMs, Reason: state.Reason, Seq: state.Seq}
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

func mustJSONRaw(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func cloneDurationMs(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

var _ ControlApplier = (*Engine)(nil)
