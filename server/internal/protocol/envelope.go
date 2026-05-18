package protocol

import "encoding/json"

const (
	TypeJoinRoom           = "join_room"
	TypeLeaveRoom          = "leave_room"
	TypeRoomState          = "room_state"
	TypeRoomStateRequest   = "room_state.request"
	TypeRoomMembersChanged = "room_members_changed"
	TypePlay               = "play"
	TypePause              = "pause"
	TypeSeek               = "seek"
	TypeSetPlaybackRate    = "set_playback_rate"
	TypeEnded              = "ended"
	TypeHeartbeat          = "heartbeat"
	TypeHeartbeatAck       = "heartbeat_ack"
	TypeClockSyncPing      = "clock_sync.ping"
	TypeClockSyncPong      = "clock_sync.pong"
	TypeError              = "error"
)

type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ErrorEnvelope struct {
	Type    string       `json:"type"`
	Payload ErrorPayload `json:"payload"`
}

func (e Envelope) OutboxCoalesceKey() string {
	if e.Type == TypeRoomState {
		return TypeRoomState
	}
	return ""
}
