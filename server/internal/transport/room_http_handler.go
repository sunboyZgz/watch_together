package transport

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/recovery"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
)

type RoomHTTPHandler struct {
	roomRuntime       roomRuntimeRegistry
	roomService       roomapi.BusinessService
	tokenVerifier     accessTokenVerifier
	playbackSigner    *mediaPlaybackSigner
	delivery          *mediaDelivery
	authorityRecovery roomAuthorityRecovery
	authorityClaimer  roomAuthorityClaimer
	authorityInstance string
	runtimeRequired   bool
	authorityRequired bool
}

type roomRuntimeRegistry interface {
	RegisterCreatedRoomWithMedia(
		roomID string,
		hostUserID string,
		mediaID string,
		mediaDurationMs *int64,
	) roomStateSnapshotter
	StoreLatestRoomState(ctx context.Context, state room.State) error
}

type roomAuthorityClaimer interface {
	ClaimAuthority(ctx context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error)
}

type roomStateSnapshotter interface {
	StateSnapshot() room.State
}

type roomManagerRuntime struct {
	manager         *room.Manager
	roomStateWriter latestRoomStateWriter
}

func (r roomManagerRuntime) RegisterCreatedRoomWithMedia(
	roomID string,
	hostUserID string,
	mediaID string,
	mediaDurationMs *int64,
) roomStateSnapshotter {
	if r.manager == nil {
		return nil
	}
	runtimeRoom := r.manager.RegisterCreatedRoomWithMedia(roomID, hostUserID, mediaID, mediaDurationMs)
	return runtimeRoom
}

func (r roomManagerRuntime) StoreLatestRoomState(ctx context.Context, state room.State) error {
	if r.roomStateWriter == nil || state.RoomID == "" {
		return nil
	}
	return r.roomStateWriter.SetRoomState(ctx, state.RoomID, roomStatePayload(state))
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
	MediaDurationMs *int64  `json:"mediaDurationMs,omitempty"`
	Paused          bool    `json:"paused"`
	PositionMs      int64   `json:"positionMs"`
	Velocity        float64 `json:"velocity"`
	ServerTimeMs    int64   `json:"serverTimeMs"`
	Reason          string  `json:"reason"`
	PlaybackRate    float64 `json:"playbackRate"`
	Ended           bool    `json:"ended"`
	Seq             int64   `json:"seq"`
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
func NewRoomHTTPHandler(roomManager *room.Manager, roomService roomapi.BusinessService) *RoomHTTPHandler {
	return NewRoomHTTPHandlerWithTokenVerifier(roomManager, roomService, nil)
}

func NewRoomHTTPHandlerWithTokenVerifier(
	roomManager *room.Manager,
	roomService roomapi.BusinessService,
	tokenVerifier accessTokenVerifier,
	playbackConfigs ...MediaPlaybackConfig,
) *RoomHTTPHandler {
	return NewRoomHTTPHandlerWithTokenVerifierAndRoomStateWriter(
		roomManager,
		roomService,
		tokenVerifier,
		nil,
		playbackConfigs...,
	)
}

func NewRoomHTTPHandlerWithTokenVerifierAndRoomStateWriter(
	roomManager *room.Manager,
	roomService roomapi.BusinessService,
	tokenVerifier accessTokenVerifier,
	roomStateWriter latestRoomStateWriter,
	playbackConfigs ...MediaPlaybackConfig,
) *RoomHTTPHandler {
	var roomRuntime roomRuntimeRegistry
	if roomManager != nil {
		roomRuntime = roomManagerRuntime{
			manager:         roomManager,
			roomStateWriter: roomStateWriter,
		}
	}
	return newRoomHTTPHandlerWithRuntime(
		roomRuntime,
		roomService,
		tokenVerifier,
		roomRuntime != nil,
		false,
		playbackConfigs...,
	)
}

func NewRoomHTTPGatewayHandler(
	roomService roomapi.BusinessService,
	tokenVerifier accessTokenVerifier,
	playbackConfigs ...MediaPlaybackConfig,
) *RoomHTTPHandler {
	return newRoomHTTPHandlerWithRuntime(
		nil,
		roomService,
		tokenVerifier,
		false,
		true,
		playbackConfigs...,
	)
}

func (h *RoomHTTPHandler) SetRoomAuthorityClaimer(instanceID string, claimer roomAuthorityClaimer) {
	if h == nil {
		return
	}
	h.authorityClaimer = claimer
	h.authorityInstance = strings.TrimSpace(instanceID)
}

func (h *RoomHTTPHandler) SetRoomAuthorityRecovery(recoveryService roomAuthorityRecovery) {
	if h == nil {
		return
	}
	h.authorityRecovery = recoveryService
}

func newRoomHTTPHandlerWithRuntime(
	roomRuntime roomRuntimeRegistry,
	roomService roomapi.BusinessService,
	tokenVerifier accessTokenVerifier,
	runtimeRequired bool,
	authorityRequired bool,
	playbackConfigs ...MediaPlaybackConfig,
) *RoomHTTPHandler {
	playbackConfig := MediaPlaybackConfig{}
	if len(playbackConfigs) > 0 {
		playbackConfig = playbackConfigs[0]
	}
	return &RoomHTTPHandler{
		roomRuntime:       roomRuntime,
		roomService:       roomService,
		tokenVerifier:     tokenVerifier,
		playbackSigner:    newMediaPlaybackSigner(playbackConfig),
		delivery:          newMediaDelivery(playbackConfig),
		runtimeRequired:   runtimeRequired,
		authorityRequired: authorityRequired,
	}
}

func (h *RoomHTTPHandler) RoomLeaver() roomMembershipLeaver {
	if h == nil {
		return nil
	}
	return h.roomService
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

	hostUserID, ok := userIDFromAuthorization(r.Header.Get("Authorization"), h.tokenVerifier)
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

	state, ok := h.prepareRoomRuntime(r.Context(), result.Room.RoomCode, result.Room.HostUserID, result.Room.MediaItemID, result.Media.DurationMs)
	if !ok {
		writeAPIError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "room runtime is unavailable", nil)
		return
	}
	if !h.claimRoomAuthority(w, r.Context(), result.Room.RoomCode) {
		return
	}
	writeAPISuccess(w, http.StatusCreated, createRoomResponse{
		Room:      roomToResponse(result.Room),
		Media:     h.roomMediaToResponse(r, result.Media),
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

	userID, ok := userIDFromAuthorization(r.Header.Get("Authorization"), h.tokenVerifier)
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

	state, _ := h.prepareRoomRuntime(
		r.Context(),
		result.Room.RoomCode,
		result.Room.HostUserID,
		result.Room.MediaItemID,
		result.Media.DurationMs,
	)
	_ = state
	if !h.claimRoomAuthority(w, r.Context(), result.Room.RoomCode) {
		return
	}
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
	userID, ok := userIDFromAuthorization(r.Header.Get("Authorization"), h.tokenVerifier)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
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
	if !roomDetailHasActiveMember(result, userID) {
		writeAPIError(w, http.StatusForbidden, "FORBIDDEN", "room membership required", nil)
		return
	}
	_, _ = h.prepareRoomRuntime(r.Context(), result.Room.RoomCode, result.Room.HostUserID, result.Room.MediaItemID, result.Media.DurationMs)
	writeAPISuccess(w, http.StatusOK, roomDetailResponse{
		Room:    roomToResponse(result.Room),
		Media:   h.roomMediaToResponse(r, result.Media),
		Members: membersToResponse(result.Members),
	})
}

func roomDetailHasActiveMember(result roomapi.DetailResult, userID string) bool {
	for _, member := range result.Members {
		if member.UserID == userID {
			return true
		}
	}
	return false
}

func (h *RoomHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.roomService == nil || (h.runtimeRequired && h.roomRuntime == nil) {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "room service is unavailable", nil)
		return false
	}
	return true
}

func (h *RoomHTTPHandler) prepareRoomRuntime(
	ctx context.Context,
	roomCode string,
	hostUserID string,
	mediaItemID string,
	mediaDurationMs *int64,
) (room.State, bool) {
	if h == nil || h.roomRuntime == nil {
		state := room.NewCreatedRoomWithMedia(roomCode, hostUserID, mediaItemID, mediaDurationMs).StateSnapshot()
		return state, !h.runtimeRequired
	}
	runtimeRoom := h.roomRuntime.RegisterCreatedRoomWithMedia(roomCode, hostUserID, mediaItemID, mediaDurationMs)
	if runtimeRoom == nil {
		return room.State{}, false
	}
	h.tryRecoverRoomAuthority(ctx, roomCode, "room_http")
	state := runtimeRoom.StateSnapshot()
	h.storeLatestRoomState(ctx, state)
	return state, true
}

func (h *RoomHTTPHandler) claimRoomAuthority(w http.ResponseWriter, ctx context.Context, roomCode string) bool {
	if h == nil || h.authorityClaimer == nil || h.authorityInstance == "" || roomCode == "" {
		if h != nil && h.authorityRequired {
			writeAPIError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "room authority is unavailable", nil)
			return false
		}
		return true
	}
	claimCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, claimed, err := h.authorityClaimer.ClaimAuthority(claimCtx, roomCode, h.authorityInstance); err != nil {
		log.Printf("room authority claim failed room=%s instance=%s err=%v", roomCode, h.authorityInstance, err)
		if h.authorityRequired {
			writeAPIError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "room authority is unavailable", nil)
			return false
		}
	} else if !claimed {
		log.Printf("room authority claim skipped room=%s instance=%s reason=another_authority", roomCode, h.authorityInstance)
		if h.authorityRequired {
			writeAPIError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "room authority is unavailable", nil)
			return false
		}
	}
	return true
}

func (h *RoomHTTPHandler) storeLatestRoomState(ctx context.Context, state room.State) {
	if h == nil || h.roomRuntime == nil || state.RoomID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	writeCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	if err := h.roomRuntime.StoreLatestRoomState(writeCtx, state); err != nil {
		log.Printf("room_state cache write failed room=%s seq=%d err=%v", state.RoomID, state.Seq, err)
	}
}

func (h *RoomHTTPHandler) tryRecoverRoomAuthority(ctx context.Context, roomID string, reason string) {
	if h == nil || h.authorityRecovery == nil || roomID == "" {
		return
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := h.authorityRecovery.TryRecoverRoomAuthority(recoveryCtx, roomID, reason); err != nil &&
		!errors.Is(err, recovery.ErrAuthorityActive) &&
		!errors.Is(err, recovery.ErrAuthorityRecovering) {
		log.Printf("room authority recovery failed room=%s reason=%s err=%v", roomID, reason, err)
	}
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
	case errors.Is(err, roomapi.ErrIdentityUnavailable):
		writeAPIError(w, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "identity service is unavailable", nil)
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

func (h *RoomHTTPHandler) roomMediaToResponse(r *http.Request, media roomapi.Media) roomMediaResponse {
	return roomMediaResponse{
		ID:           media.ID,
		Title:        media.Title,
		Subtitle:     media.Subtitle,
		MediaURL:     h.delivery.PlaybackURL(r, media.ID, media.MediaURL),
		DurationMs:   media.DurationMs,
		SeasonLabel:  media.SeasonLabel,
		EpisodeLabel: media.EpisodeLabel,
	}
}

func roomStateToResponse(state room.State) roomStateResponse {
	view := newRoomSyncView(state)
	return roomStateResponse{
		MediaDurationMs: view.MediaDurationMs,
		Paused:          view.Paused,
		PositionMs:      view.PositionMs,
		Velocity:        view.Velocity,
		ServerTimeMs:    view.ServerTimeMs,
		Reason:          view.Reason,
		PlaybackRate:    view.PlaybackRate,
		Ended:           view.Ended,
		Seq:             view.Seq,
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
