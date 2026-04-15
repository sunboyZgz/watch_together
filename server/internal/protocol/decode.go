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
