package protocol

type JoinRoomPayload struct {
	RoomID string `json:"roomId"`
	UserID string `json:"userId"`
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
