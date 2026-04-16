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
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
}

type PlayPayload struct {
	RoomID     string `json:"roomId"`
	UserID     string `json:"userId"`
	PositionMs int64  `json:"positionMs"`
	Seq        int64  `json:"seq"`
}

type PausePayload struct {
	RoomID     string `json:"roomId"`
	UserID     string `json:"userId"`
	PositionMs int64  `json:"positionMs"`
	Seq        int64  `json:"seq"`
}

type SeekPayload struct {
	RoomID     string `json:"roomId"`
	UserID     string `json:"userId"`
	PositionMs int64  `json:"positionMs"`
	Seq        int64  `json:"seq"`
}

type RoomStatePayload struct {
	RoomID       string  `json:"roomId"`
	MediaID      string  `json:"mediaId"`
	HostUserID   string  `json:"hostUserId"`
	Paused       bool    `json:"paused"`
	PositionMs   int64   `json:"positionMs"`
	PlaybackRate float64 `json:"playbackRate"`
	Seq          int64   `json:"seq"`
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

func (p PausePayload) GetRoomID() string {
	return p.RoomID
}

func (p PausePayload) GetUserID() string {
	return p.UserID
}

func (p SeekPayload) GetRoomID() string {
	return p.RoomID
}

func (p SeekPayload) GetUserID() string {
	return p.UserID
}
