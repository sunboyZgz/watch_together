package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
)

type RoomHTTPHandler struct {
	roomManager *room.Manager
	roomService *roomapi.Service
}

type createRoomRequest struct {
	MediaItemID string `json:"mediaItemId"`
}

type roomResponse struct {
	ID          string `json:"id"`
	RoomCode    string `json:"roomCode"`
	HostUserID  string `json:"hostUserId"`
	MediaItemID string `json:"mediaItemId"`
	Status      string `json:"status"`
}

type roomMediaResponse struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Subtitle     *string `json:"subtitle,omitempty"`
	MediaURL     string  `json:"mediaUrl"`
	DurationMs   *int64  `json:"durationMs"`
	SeasonLabel  *string `json:"seasonLabel"`
	EpisodeLabel *string `json:"episodeLabel"`
}

type roomStateResponse struct {
	Paused       bool    `json:"paused"`
	PositionMs   int64   `json:"positionMs"`
	Velocity     float64 `json:"velocity"`
	ServerTimeMs int64   `json:"serverTimeMs"`
	Reason       string  `json:"reason"`
	PlaybackRate float64 `json:"playbackRate"`
	Ended        bool    `json:"ended"`
	Seq          int64   `json:"seq"`
}

type createRoomResponse struct {
	Room      roomResponse      `json:"room"`
	Media     roomMediaResponse `json:"media"`
	RoomState roomStateResponse `json:"roomState"`
}

type joinRoomResponse struct {
	Room   roomResponse   `json:"room"`
	Member memberResponse `json:"member"`
}

type memberResponse struct {
	UserID     string  `json:"userId"`
	Nickname   string  `json:"nickname,omitempty"`
	AvatarSeed string  `json:"avatarSeed,omitempty"`
	AvatarURL  *string `json:"avatarUrl"`
	Role       string  `json:"role"`
}

type roomDetailResponse struct {
	Room    roomResponse      `json:"room"`
	Media   roomMediaResponse `json:"media"`
	Members []memberResponse  `json:"members"`
}

// NewRoomHTTPHandler builds the HTTP entrypoint for room creation and join-by-code.
func NewRoomHTTPHandler(roomManager *room.Manager, roomService *roomapi.Service) *RoomHTTPHandler {
	return &RoomHTTPHandler{
		roomManager: roomManager,
		roomService: roomService,
	}
}

// CreateRoom handles POST /rooms, persists room business data, and prepares runtime sync state.
func (h *RoomHTTPHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	hostUserID, ok := userIDFromAuthorization(r.Header.Get("Authorization"))
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
		return
	}

	var request createRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body", nil)
		return
	}

	result, err := h.roomService.CreateRoom(r.Context(), hostUserID, request.MediaItemID)
	if err != nil {
		h.writeRoomError(w, err)
		return
	}

	runtimeRoom := h.roomManager.RegisterCreatedRoom(result.Room.RoomCode, result.Room.HostUserID, result.Room.MediaItemID)
	state := runtimeRoom.StateSnapshot()
	writeAPISuccess(w, http.StatusCreated, createRoomResponse{
		Room:      roomToResponse(result.Room),
		Media:     roomMediaToResponse(result.Media),
		RoomState: roomStateToResponse(state),
	})
}

// JoinRoomByCode handles POST /rooms/{roomCode}/join for business membership.
func (h *RoomHTTPHandler) JoinRoomByCode(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	userID, ok := userIDFromAuthorization(r.Header.Get("Authorization"))
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
		return
	}

	roomCode, ok := roomCodeFromJoinPath(r.URL.Path)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "room route not found", nil)
		return
	}

	result, err := h.roomService.JoinRoomByCode(r.Context(), roomCode, userID)
	if err != nil {
		h.writeRoomError(w, err)
		return
	}

	// Keep the runtime room available for the following WebSocket join_room call.
	h.roomManager.RegisterCreatedRoom(result.Room.RoomCode, result.Room.HostUserID, result.Room.MediaItemID)
	writeAPISuccess(w, http.StatusOK, joinRoomResponse{
		Room:   roomToResponse(result.Room),
		Member: memberToResponse(result.Member),
	})
}

// RoomRoute dispatches /rooms/{roomCode} and /rooms/{roomCode}/join requests.
func (h *RoomHTTPHandler) RoomRoute(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.DetailByCode(w, r)
		return
	}
	if r.Method == http.MethodPost {
		h.JoinRoomByCode(w, r)
		return
	}
	writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
}

// DetailByCode handles GET /rooms/{roomCode} for theater page bootstrap data.
func (h *RoomHTTPHandler) DetailByCode(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	roomCode, ok := roomCodeFromDetailPath(r.URL.Path)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "room route not found", nil)
		return
	}

	result, err := h.roomService.DetailByCode(r.Context(), roomCode)
	if err != nil {
		h.writeRoomError(w, err)
		return
	}
	h.roomManager.RegisterCreatedRoom(result.Room.RoomCode, result.Room.HostUserID, result.Room.MediaItemID)
	writeAPISuccess(w, http.StatusOK, roomDetailResponse{
		Room:    roomToResponse(result.Room),
		Media:   roomMediaToResponse(result.Media),
		Members: membersToResponse(result.Members),
	})
}

func (h *RoomHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.roomManager == nil || h.roomService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "room service is unavailable", nil)
		return false
	}
	return true
}

func (h *RoomHTTPHandler) writeRoomError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, roomapi.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "room request is invalid", nil)
	case errors.Is(err, roomapi.ErrUserNotFound):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found", nil)
	case errors.Is(err, roomapi.ErrMediaNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "media item not found", nil)
	case errors.Is(err, roomapi.ErrRoomNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "room not found", nil)
	case errors.Is(err, roomapi.ErrUnableToCreate):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "unable to generate room code", nil)
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "room request failed", nil)
	}
}

func roomCodeFromJoinPath(path string) (string, bool) {
	const prefix = "/rooms/"
	const suffix = "/join"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	roomCode := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	roomCode = strings.ToUpper(strings.Trim(roomCode, "/"))
	return roomCode, len(roomCode) == 6
}

func roomCodeFromDetailPath(path string) (string, bool) {
	const prefix = "/rooms/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	roomCode := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	return strings.ToUpper(roomCode), len(roomCode) == 6
}

func roomToResponse(room roomapi.Room) roomResponse {
	return roomResponse{
		ID:          room.ID,
		RoomCode:    room.RoomCode,
		HostUserID:  room.HostUserID,
		MediaItemID: room.MediaItemID,
		Status:      room.Status,
	}
}

func roomMediaToResponse(media roomapi.Media) roomMediaResponse {
	return roomMediaResponse{
		ID:           media.ID,
		Title:        media.Title,
		Subtitle:     media.Subtitle,
		MediaURL:     media.MediaURL,
		DurationMs:   media.DurationMs,
		SeasonLabel:  media.SeasonLabel,
		EpisodeLabel: media.EpisodeLabel,
	}
}

func roomStateToResponse(state room.State) roomStateResponse {
	return roomStateResponse{
		Paused:       state.Paused,
		PositionMs:   state.PositionMs,
		Velocity:     state.Velocity,
		ServerTimeMs: state.ServerTimeMs,
		Reason:       state.Reason,
		PlaybackRate: state.PlaybackRate,
		Ended:        state.Ended,
		Seq:          state.Seq,
	}
}

func memberToResponse(member roomapi.Member) memberResponse {
	return memberResponse{
		UserID:     member.UserID,
		Nickname:   member.Nickname,
		AvatarSeed: member.AvatarSeed,
		AvatarURL:  member.AvatarURL,
		Role:       member.Role,
	}
}

func membersToResponse(members []roomapi.Member) []memberResponse {
	responses := make([]memberResponse, 0, len(members))
	for _, member := range members {
		responses = append(responses, memberToResponse(member))
	}
	return responses
}
