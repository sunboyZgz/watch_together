package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
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

func decodeControlPayload[T interface {
	GetRoomID() string
	GetUserID() string
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
