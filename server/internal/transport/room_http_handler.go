package transport

import (
	"encoding/json"
	"net/http"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

type RoomHTTPHandler struct {
	roomManager *room.Manager
}

// NewRoomHTTPHandler builds the HTTP entrypoint for room creation.
func NewRoomHTTPHandler(roomManager *room.Manager) *RoomHTTPHandler {
	return &RoomHTTPHandler{roomManager: roomManager}
}

// CreateRoom handles POST /rooms and returns a fresh room ID plus its initial room_state.
func (h *RoomHTTPHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request protocol.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if request.UserID == "" {
		http.Error(w, "userId is required", http.StatusBadRequest)
		return
	}
	if request.MediaID == "" {
		request.MediaID = "sample_001"
	}

	createdRoom, err := h.roomManager.CreateRoom(request.UserID, request.MediaID)
	if err != nil {
		http.Error(w, "failed to create room", http.StatusInternalServerError)
		return
	}

	state := createdRoom.StateSnapshot()
	response := protocol.CreateRoomResponse{
		RoomID: createdRoom.ID(),
		RoomState: protocol.RoomStatePayload{
			RoomID:       state.RoomID,
			MediaID:      state.MediaID,
			HostUserID:   state.HostUserID,
			Paused:       state.Paused,
			PositionMs:   state.PositionMs,
			PlaybackRate: state.PlaybackRate,
			Seq:          state.Seq,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}
