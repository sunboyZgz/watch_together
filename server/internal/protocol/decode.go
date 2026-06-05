package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

var ErrUnsupportedMessageType = errors.New("unsupported message type")

// DecodeEnvelope parses the common type + payload wrapper used by every client message.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Envelope{}, err
	}
	if envelope.Type == "" {
		return Envelope{}, errors.New("missing message type")
	}
	return envelope, nil
}

// DecodeJoinRoom validates that the envelope is a join_room event and decodes its payload.
func DecodeJoinRoom(envelope Envelope) (JoinRoomPayload, error) {
	if envelope.Type != TypeJoinRoom {
		return JoinRoomPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload JoinRoomPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return JoinRoomPayload{}, err
	}
	if payload.RoomID == "" {
		return JoinRoomPayload{}, errors.New("missing roomId")
	}
	if payload.UserID == "" {
		return JoinRoomPayload{}, errors.New("missing userId")
	}
	if payload.DeviceID == "" {
		return JoinRoomPayload{}, errors.New("missing deviceId")
	}
	return payload, nil
}

// DecodeLeaveRoom validates that the envelope is a leave_room event and decodes its payload.
func DecodeLeaveRoom(envelope Envelope) (LeaveRoomPayload, error) {
	if envelope.Type != TypeLeaveRoom {
		return LeaveRoomPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload LeaveRoomPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return LeaveRoomPayload{}, err
	}
	if payload.RoomID == "" {
		return LeaveRoomPayload{}, errors.New("missing roomId")
	}
	if payload.UserID == "" {
		return LeaveRoomPayload{}, errors.New("missing userId")
	}
	return payload, nil
}

// DecodeRoomStateRequest validates and decodes one explicit room_state refresh request.
func DecodeRoomStateRequest(envelope Envelope) (RoomStateRequestPayload, error) {
	if envelope.Type != TypeRoomStateRequest {
		return RoomStateRequestPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload RoomStateRequestPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return RoomStateRequestPayload{}, err
	}
	if payload.RoomID == "" {
		return RoomStateRequestPayload{}, errors.New("missing roomId")
	}
	if payload.UserID == "" {
		return RoomStateRequestPayload{}, errors.New("missing userId")
	}
	return payload, nil
}

// DecodeRoomDeviceSwitchReply validates and decodes one room device switch reply event.
func DecodeRoomDeviceSwitchReply(envelope Envelope) (RoomDeviceSwitchReplyPayload, error) {
	if envelope.Type != TypeRoomDeviceSwitchReply {
		return RoomDeviceSwitchReplyPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload RoomDeviceSwitchReplyPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return RoomDeviceSwitchReplyPayload{}, err
	}
	if payload.RoomID == "" {
		return RoomDeviceSwitchReplyPayload{}, errors.New("missing roomId")
	}
	if payload.UserID == "" {
		return RoomDeviceSwitchReplyPayload{}, errors.New("missing userId")
	}
	if payload.RequestID == "" {
		return RoomDeviceSwitchReplyPayload{}, errors.New("missing requestId")
	}
	return payload, nil
}

// DecodePlay validates and decodes a play control event.
func DecodePlay(envelope Envelope) (PlayPayload, error) {
	if envelope.Type != TypePlay {
		return PlayPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}
	return decodeControlPayload[PlayPayload](envelope)
}

// DecodePause validates and decodes a pause control event.
func DecodePause(envelope Envelope) (PausePayload, error) {
	if envelope.Type != TypePause {
		return PausePayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}
	return decodeControlPayload[PausePayload](envelope)
}

// DecodeSeek validates and decodes a seek control event.
func DecodeSeek(envelope Envelope) (SeekPayload, error) {
	if envelope.Type != TypeSeek {
		return SeekPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}
	return decodeControlPayload[SeekPayload](envelope)
}

// DecodeSetPlaybackRate validates and decodes a playback-rate control event.
func DecodeSetPlaybackRate(envelope Envelope) (SetPlaybackRatePayload, error) {
	if envelope.Type != TypeSetPlaybackRate {
		return SetPlaybackRatePayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}
	payload, err := decodeControlPayload[SetPlaybackRatePayload](envelope)
	if err != nil {
		return SetPlaybackRatePayload{}, err
	}
	if payload.PlaybackRate < 0.25 ||
		payload.PlaybackRate > 2.0 ||
		math.IsNaN(payload.PlaybackRate) ||
		math.IsInf(payload.PlaybackRate, 0) {
		return SetPlaybackRatePayload{}, errors.New("invalid playbackRate")
	}
	return payload, nil
}

// DecodeEnded validates and decodes one ended control event.
func DecodeEnded(envelope Envelope) (EndedPayload, error) {
	if envelope.Type != TypeEnded {
		return EndedPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}
	return decodeControlPayload[EndedPayload](envelope)
}

// DecodeHeartbeatAck validates and decodes one heartbeat_ack event.
func DecodeHeartbeatAck(envelope Envelope) (HeartbeatAckPayload, error) {
	if envelope.Type != TypeHeartbeatAck {
		return HeartbeatAckPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload HeartbeatAckPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return HeartbeatAckPayload{}, err
	}
	if payload.ServerTimeMs == 0 {
		return HeartbeatAckPayload{}, errors.New("missing serverTimeMs")
	}
	if payload.ClientTimeMs == 0 {
		return HeartbeatAckPayload{}, errors.New("missing clientTimeMs")
	}
	return payload, nil
}

// DecodeClockSyncPing validates and decodes one clock_sync.ping event.
func DecodeClockSyncPing(envelope Envelope) (ClockSyncPingPayload, error) {
	if envelope.Type != TypeClockSyncPing {
		return ClockSyncPingPayload{}, fmt.Errorf("%w: %s", ErrUnsupportedMessageType, envelope.Type)
	}

	var payload ClockSyncPingPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return ClockSyncPingPayload{}, err
	}
	if payload.ClientSendMonoMs == 0 {
		return ClockSyncPingPayload{}, errors.New("missing clientSendMonoMs")
	}
	return payload, nil
}

func decodeControlPayload[T interface {
	GetRoomID() string
	GetUserID() string
	GetSeq() int64
	GetRequestID() string
}](envelope Envelope) (T, error) {
	var payload T
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return payload, err
	}
	if payload.GetRoomID() == "" {
		return payload, errors.New("missing roomId")
	}
	if payload.GetUserID() == "" {
		return payload, errors.New("missing userId")
	}
	return payload, nil
}
