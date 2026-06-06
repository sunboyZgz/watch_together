package protocol

type CreateRoomRequest struct {
	UserID  string `json:"userId"`
	MediaID string `json:"mediaId"`
}

type CreateRoomResponse struct {
	RoomID    string           `json:"roomId"`
	RoomState RoomStatePayload `json:"roomState"`
}

type JoinRoomPayload struct {
	RoomID   string `json:"roomId"`
	UserID   string `json:"userId"`
	DeviceID string `json:"deviceId"`
}

type LeaveRoomPayload struct {
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
}

type RoomStateRequestPayload struct {
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
	Seq    int64  `json:"seq,omitempty"`
}

type PlayPayload struct {
	RoomID       string  `json:"roomId"`
	UserID       string  `json:"userId"`
	RequestID    string  `json:"requestId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity,omitempty"`
	ServerTimeMs int64   `json:"serverTimeMs,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Seq          int64   `json:"seq"`
}

type PausePayload struct {
	RoomID       string  `json:"roomId"`
	UserID       string  `json:"userId"`
	RequestID    string  `json:"requestId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity,omitempty"`
	ServerTimeMs int64   `json:"serverTimeMs,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Seq          int64   `json:"seq"`
}

type SeekPayload struct {
	RoomID       string  `json:"roomId"`
	UserID       string  `json:"userId"`
	RequestID    string  `json:"requestId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity,omitempty"`
	ServerTimeMs int64   `json:"serverTimeMs,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Seq          int64   `json:"seq"`
}

type SetPlaybackRatePayload struct {
	RoomID       string  `json:"roomId"`
	UserID       string  `json:"userId"`
	RequestID    string  `json:"requestId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity,omitempty"`
	ServerTimeMs int64   `json:"serverTimeMs,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	PlaybackRate float64 `json:"playbackRate"`
	Seq          int64   `json:"seq"`
}

type EndedPayload struct {
	RoomID       string  `json:"roomId"`
	UserID       string  `json:"userId"`
	RequestID    string  `json:"requestId,omitempty"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity,omitempty"`
	ServerTimeMs int64   `json:"serverTimeMs,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	Seq          int64   `json:"seq"`
}

type HeartbeatPayload struct {
	ServerTimeMs int64 `json:"serverTimeMs"`
}

type HeartbeatAckPayload struct {
	ServerTimeMs int64 `json:"serverTimeMs"`
	ClientTimeMs int64 `json:"clientTimeMs"`
}

type ClockSyncPingPayload struct {
	ClientSendMonoMs int64 `json:"clientSendMonoMs"`
}

type ClockSyncPongPayload struct {
	ServerTimeMs     int64 `json:"serverTimeMs"`
	ClientSendMonoMs int64 `json:"clientSendMonoMs"`
}

type RoomStatePayload struct {
	RoomID          string  `json:"roomId"`
	MediaID         string  `json:"mediaId"`
	MediaDurationMs *int64  `json:"mediaDurationMs,omitempty"`
	HostUserID      string  `json:"hostUserId"`
	Paused          bool    `json:"paused"`
	Ended           bool    `json:"ended"`
	PositionMs      int64   `json:"positionMs"`
	Velocity        float64 `json:"velocity"`
	ServerTimeMs    int64   `json:"serverTimeMs"`
	Reason          string  `json:"reason"`
	PlaybackRate    float64 `json:"playbackRate"`
	Seq             int64   `json:"seq"`
}

type RoomMembersChangedPayload struct {
	RoomID string `json:"roomId"`
	Reason string `json:"reason"`
}

type RoomPresenceMemberPayload struct {
	UserID string `json:"userId"`
	Role   string `json:"role,omitempty"`
	IsHost bool   `json:"isHost"`
	IsSelf bool   `json:"isSelf"`
}

type RoomPresencePayload struct {
	RoomID       string                      `json:"roomId"`
	OnlineCount  int                         `json:"onlineCount"`
	Members      []RoomPresenceMemberPayload `json:"members"`
	Reason       string                      `json:"reason"`
	ServerTimeMs int64                       `json:"serverTimeMs"`
}

type RoomDeviceSwitchRequestPayload struct {
	RoomID       string `json:"roomId"`
	TargetRoomID string `json:"targetRoomId,omitempty"`
	UserID       string `json:"userId"`
	RequestID    string `json:"requestId"`
	ExpiresAtMs  int64  `json:"expiresAtMs"`
}

type RoomDeviceSwitchReplyPayload struct {
	RoomID    string `json:"roomId"`
	UserID    string `json:"userId"`
	RequestID string `json:"requestId"`
	Approve   bool   `json:"approve"`
}

type RoomDeviceSwitchResultPayload struct {
	RoomID    string `json:"roomId"`
	UserID    string `json:"userId"`
	RequestID string `json:"requestId"`
	Approved  bool   `json:"approved"`
	Reason    string `json:"reason,omitempty"`
}

type ErrorPayload struct {
	RoomID  string `json:"roomId"`
	Message string `json:"message"`
}

func (p PlayPayload) GetRoomID() string {
	return p.RoomID
}

func (p PlayPayload) GetUserID() string {
	return p.UserID
}

func (p PlayPayload) GetSeq() int64 {
	return p.Seq
}

func (p PlayPayload) GetRequestID() string {
	return p.RequestID
}

func (p PausePayload) GetRoomID() string {
	return p.RoomID
}

func (p PausePayload) GetUserID() string {
	return p.UserID
}

func (p PausePayload) GetSeq() int64 {
	return p.Seq
}

func (p PausePayload) GetRequestID() string {
	return p.RequestID
}

func (p SeekPayload) GetRoomID() string {
	return p.RoomID
}

func (p SeekPayload) GetUserID() string {
	return p.UserID
}

func (p SeekPayload) GetSeq() int64 {
	return p.Seq
}

func (p SeekPayload) GetRequestID() string {
	return p.RequestID
}

func (p SetPlaybackRatePayload) GetRoomID() string {
	return p.RoomID
}

func (p SetPlaybackRatePayload) GetUserID() string {
	return p.UserID
}

func (p SetPlaybackRatePayload) GetSeq() int64 {
	return p.Seq
}

func (p SetPlaybackRatePayload) GetRequestID() string {
	return p.RequestID
}

func (p EndedPayload) GetRoomID() string {
	return p.RoomID
}

func (p EndedPayload) GetUserID() string {
	return p.UserID
}

func (p EndedPayload) GetSeq() int64 {
	return p.Seq
}

func (p EndedPayload) GetRequestID() string {
	return p.RequestID
}
