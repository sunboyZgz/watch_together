package protocol

import "encoding/json"

const (
	TypeJoinRoom        = "join_room"
	TypeRoomState       = "room_state"
	TypePlay            = "play"
	TypePause           = "pause"
	TypeSeek            = "seek"
	TypeSetPlaybackRate = "set_playback_rate"
	TypeHeartbeat       = "heartbeat"
	TypeHeartbeatAck    = "heartbeat_ack"
	TypeError           = "error"
)

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ErrorEnvelope struct {
	Type    string       `json:"type"`
	Payload ErrorPayload `json:"payload"`
}
