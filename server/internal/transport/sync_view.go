package transport

import "watch_together/server/internal/room"

type roomSyncView struct {
	Paused       bool
	PositionMs   int64
	Velocity     float64
	ServerTimeMs int64
	Reason       string
	PlaybackRate float64
	Ended        bool
	Seq          int64
}

func newRoomSyncView(state room.State) roomSyncView {
	return roomSyncView{
		Paused:       pausedFromVelocity(state.Velocity),
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		PlaybackRate: state.PlaybackRate,
		Ended:        state.Ended,
		Seq:          state.Seq,
	}
}

func pausedFromVelocity(velocity float64) bool {
	return velocity == 0
}
