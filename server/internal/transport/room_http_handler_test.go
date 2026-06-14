package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/room"
	"watch_together/server/internal/roomapi"
)

func TestCreateRoomFlow(t *testing.T) {
	durationMs := int64(1_458_000)
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(&fakeRoomStore{
		createResult: roomapi.CreateRoomResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Media: roomapi.Media{
				ID:         "media_001",
				Title:      "紫罗兰永恒花园",
				MediaURL:   "https://example.com/index.m3u8",
				DurationMs: &durationMs,
			},
		},
	}))

	request := httptest.NewRequest(
		http.MethodPost,
		"/rooms",
		strings.NewReader(`{"mediaItemId":"media_001"}`),
	)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateRoom(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	var response struct {
		Data createRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if response.Data.Room.RoomCode != "A7K2M9" {
		t.Fatalf("expected room code A7K2M9, got %q", response.Data.Room.RoomCode)
	}
	if response.Data.Room.HostUserID != "user_a" {
		t.Fatalf("expected host user user_a, got %q", response.Data.Room.HostUserID)
	}
	if response.Data.Media.ID != "media_001" {
		t.Fatalf("expected media media_001, got %q", response.Data.Media.ID)
	}
	if !strings.Contains(response.Data.Media.MediaURL, "/media/playback/media_001/master.m3u8") {
		t.Fatalf("expected signed playback mediaUrl, got %q", response.Data.Media.MediaURL)
	}
	if strings.Contains(response.Data.Media.MediaURL, "example.com/index.m3u8") {
		t.Fatalf("expected create room response not to expose raw HLS url, got %q", response.Data.Media.MediaURL)
	}
	if !response.Data.RoomState.Paused {
		t.Fatalf("expected new room paused=true")
	}
	if response.Data.RoomState.PlaybackRate != 1.0 {
		t.Fatalf("expected playbackRate 1.0, got %f", response.Data.RoomState.PlaybackRate)
	}
	if response.Data.RoomState.MediaDurationMs == nil || *response.Data.RoomState.MediaDurationMs != durationMs {
		t.Fatalf("expected roomState mediaDurationMs %d, got %v", durationMs, response.Data.RoomState.MediaDurationMs)
	}
}

func TestCreateRoomRequiresAccessToken(t *testing.T) {
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(&fakeRoomStore{}))
	request := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{"mediaItemId":"media_001"}`))
	recorder := httptest.NewRecorder()

	handler.CreateRoom(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestCreateRoomUsesRuntimeRegistryBoundary(t *testing.T) {
	durationMs := int64(1_458_000)
	runtime := &fakeRoomRuntime{
		state: room.State{
			RoomID:          "A7K2M9",
			MediaID:         "media_001",
			MediaDurationMs: &durationMs,
			HostUserID:      "user_a",
			Paused:          true,
			PlaybackRate:    1.0,
			Seq:             7,
		},
	}
	handler := newRoomHTTPHandlerWithRuntime(runtime, roomapi.NewService(&fakeRoomStore{
		createResult: roomapi.CreateRoomResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Media: roomapi.Media{
				ID:         "media_001",
				Title:      "Violet Evergarden",
				MediaURL:   "https://example.com/index.m3u8",
				DurationMs: &durationMs,
			},
		},
	}), nil, true, false)
	request := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{"mediaItemId":"media_001"}`))
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateRoom(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}
	if runtime.roomID != "A7K2M9" {
		t.Fatalf("expected runtime room A7K2M9, got %q", runtime.roomID)
	}
	if runtime.hostUserID != "user_a" {
		t.Fatalf("expected runtime host user_a, got %q", runtime.hostUserID)
	}
	if runtime.mediaID != "media_001" {
		t.Fatalf("expected runtime media media_001, got %q", runtime.mediaID)
	}
	if runtime.mediaDurationMs == nil || *runtime.mediaDurationMs != durationMs {
		t.Fatalf("expected runtime media duration %d, got %v", durationMs, runtime.mediaDurationMs)
	}

	var response struct {
		Data createRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Data.RoomState.Seq != 7 {
		t.Fatalf("expected roomState seq from runtime boundary, got %d", response.Data.RoomState.Seq)
	}
	if runtime.storeCalls != 1 {
		t.Fatalf("expected runtime snapshot store call, got %d", runtime.storeCalls)
	}
	if runtime.storedState.Seq != 7 {
		t.Fatalf("expected stored runtime seq 7, got %d", runtime.storedState.Seq)
	}
}

func TestCreateRoomGatewayClaimsAuthorityWithoutLocalRuntime(t *testing.T) {
	durationMs := int64(1_458_000)
	claimer := &fakeRoomAuthorityClaimer{}
	handler := NewRoomHTTPGatewayHandler(roomapi.NewService(&fakeRoomStore{
		createResult: roomapi.CreateRoomResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Media: roomapi.Media{
				ID:         "media_001",
				Title:      "Violet Evergarden",
				MediaURL:   "https://example.com/index.m3u8",
				DurationMs: &durationMs,
			},
		},
	}), nil)
	handler.SetRoomAuthorityClaimer("roomauthorityservice-1", claimer)
	request := httptest.NewRequest(http.MethodPost, "/rooms", strings.NewReader(`{"mediaItemId":"media_001"}`))
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateRoom(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}
	if claimer.roomID != "A7K2M9" || claimer.instanceID != "roomauthorityservice-1" {
		t.Fatalf("expected authority claim for A7K2M9 by roomauthorityservice-1, got room=%q instance=%q", claimer.roomID, claimer.instanceID)
	}
	var response struct {
		Data createRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if !response.Data.RoomState.Paused || response.Data.RoomState.PlaybackRate != 1 {
		t.Fatalf("expected gateway to return initial paused room state, got %+v", response.Data.RoomState)
	}
	if response.Data.RoomState.MediaDurationMs == nil || *response.Data.RoomState.MediaDurationMs != durationMs {
		t.Fatalf("expected roomState mediaDurationMs %d, got %v", durationMs, response.Data.RoomState.MediaDurationMs)
	}
}

func TestJoinRoomByCodeFlow(t *testing.T) {
	store := &fakeRoomStore{
		joinResult: roomapi.JoinRoomResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Member: roomapi.Member{
				UserID: "user_b",
				Role:   "member",
			},
		},
	}
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(store))
	request := httptest.NewRequest(http.MethodPost, "/rooms/A7K2M9/join", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_b"))
	recorder := httptest.NewRecorder()

	handler.JoinRoomByCode(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if store.lastJoin.RoomCode != "A7K2M9" {
		t.Fatalf("expected join room code A7K2M9, got %q", store.lastJoin.RoomCode)
	}
	if store.lastJoin.UserID != "user_b" {
		t.Fatalf("expected join user user_b, got %q", store.lastJoin.UserID)
	}

	var response struct {
		Data joinRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Data.Member.Role != "member" {
		t.Fatalf("expected member role, got %q", response.Data.Member.Role)
	}
	if response.Data.Room.RoomCode != "A7K2M9" {
		t.Fatalf("expected room code A7K2M9, got %q", response.Data.Room.RoomCode)
	}
}

func TestJoinRoomByCodeRejectsUnknownRoute(t *testing.T) {
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(&fakeRoomStore{}))
	request := httptest.NewRequest(http.MethodPost, "/rooms/A7K2M9/nope", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_b"))
	recorder := httptest.NewRecorder()

	handler.JoinRoomByCode(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestJoinRoomByCodeReturnsNotFoundWhenRoomMissing(t *testing.T) {
	store := &fakeRoomStore{err: roomapi.ErrRoomNotFound}
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(store))
	request := httptest.NewRequest(http.MethodPost, "/rooms/Z9X8Y7/join", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_b"))
	recorder := httptest.NewRecorder()

	handler.JoinRoomByCode(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "\"code\":\"NOT_FOUND\"") {
		t.Fatalf("expected NOT_FOUND error code, got body %s", recorder.Body.String())
	}
}

func TestRoomDetailFlow(t *testing.T) {
	avatarURL := "https://example.com/avatar.png"
	store := &fakeRoomStore{
		detailResult: roomapi.DetailResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Media: roomapi.Media{
				ID:       "media_001",
				Title:    "紫罗兰永恒花园",
				MediaURL: "https://example.com/index.m3u8",
			},
			Members: []roomapi.Member{
				{
					UserID:     "user_b",
					Nickname:   "Xingye",
					AvatarSeed: "xingye",
					AvatarURL:  &avatarURL,
					Role:       "member",
				},
			},
		},
	}
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(store))
	request := httptest.NewRequest(http.MethodGet, "/rooms/A7K2M9", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_b"))
	recorder := httptest.NewRecorder()

	handler.DetailByCode(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if store.lastDetailRoomCode != "A7K2M9" {
		t.Fatalf("expected room detail code A7K2M9, got %q", store.lastDetailRoomCode)
	}

	var response struct {
		Data roomDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Data.Media.ID != "media_001" {
		t.Fatalf("expected media_001, got %q", response.Data.Media.ID)
	}
	if !strings.Contains(response.Data.Media.MediaURL, "/media/playback/media_001/master.m3u8") {
		t.Fatalf("expected signed playback mediaUrl, got %q", response.Data.Media.MediaURL)
	}
	if len(response.Data.Members) != 1 {
		t.Fatalf("expected one member, got %d", len(response.Data.Members))
	}
	if response.Data.Members[0].Role != "member" {
		t.Fatalf("expected member role, got %q", response.Data.Members[0].Role)
	}
}

func TestRoomDetailRequiresAccessToken(t *testing.T) {
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(&fakeRoomStore{}))
	request := httptest.NewRequest(http.MethodGet, "/rooms/A7K2M9", nil)
	recorder := httptest.NewRecorder()

	handler.DetailByCode(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestRoomDetailRequiresActiveMembership(t *testing.T) {
	store := &fakeRoomStore{
		detailResult: roomapi.DetailResult{
			Room: roomapi.Room{
				ID:          "room_uuid",
				RoomCode:    "A7K2M9",
				HostUserID:  "user_a",
				MediaItemID: "media_001",
				Status:      "active",
			},
			Media: roomapi.Media{
				ID:       "media_001",
				Title:    "Violet Evergarden",
				MediaURL: "https://example.com/index.m3u8",
			},
			Members: []roomapi.Member{
				{
					UserID: "user_a",
					Role:   "host",
				},
			},
		},
	}
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(store))
	request := httptest.NewRequest(http.MethodGet, "/rooms/A7K2M9", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_b"))
	recorder := httptest.NewRecorder()

	handler.DetailByCode(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "room membership required") {
		t.Fatalf("expected membership error, got body %s", recorder.Body.String())
	}
}

type fakeRoomStore struct {
	createResult       roomapi.CreateRoomResult
	joinResult         roomapi.JoinRoomResult
	detailResult       roomapi.DetailResult
	lastCreate         roomapi.CreateRoomParams
	lastJoin           roomapi.JoinRoomParams
	lastDetailRoomCode string
	err                error
}

func (s *fakeRoomStore) CreateRoom(_ context.Context, params roomapi.CreateRoomParams) (roomapi.CreateRoomResult, error) {
	s.lastCreate = params
	if s.err != nil {
		return roomapi.CreateRoomResult{}, s.err
	}
	return s.createResult, nil
}

func (s *fakeRoomStore) JoinRoomByCode(_ context.Context, params roomapi.JoinRoomParams) (roomapi.JoinRoomResult, error) {
	s.lastJoin = params
	if s.err != nil {
		return roomapi.JoinRoomResult{}, s.err
	}
	return s.joinResult, nil
}

func (s *fakeRoomStore) LeaveRoomByCode(_ context.Context, params roomapi.LeaveRoomParams) error {
	s.lastJoin = roomapi.JoinRoomParams{
		RoomCode: params.RoomCode,
		UserID:   params.UserID,
	}
	return s.err
}

func (s *fakeRoomStore) GetRoomDetail(_ context.Context, roomCode string) (roomapi.DetailResult, error) {
	s.lastDetailRoomCode = roomCode
	if s.err != nil {
		return roomapi.DetailResult{}, s.err
	}
	return s.detailResult, nil
}

func (s *fakeRoomStore) IsActiveMemberByCode(_ context.Context, roomCode string, userID string) (bool, error) {
	for _, member := range s.detailResult.Members {
		if member.UserID == userID && s.detailResult.Room.RoomCode == roomCode {
			return true, nil
		}
	}
	return false, s.err
}

func (s *fakeRoomStore) GetRoomRuntimeBootstrap(_ context.Context, roomCode string) (roomapi.RuntimeBootstrapResult, error) {
	if s.err != nil {
		return roomapi.RuntimeBootstrapResult{}, s.err
	}
	return roomapi.RuntimeBootstrapResult{
		Room:  s.detailResult.Room,
		Media: s.detailResult.Media,
	}, nil
}

func (s *fakeRoomStore) ListRecoverableRoomCodes(context.Context, int) ([]string, error) {
	return nil, s.err
}

func (s *fakeRoomStore) MarkRoomGracePeriod(context.Context, string, time.Time, time.Time) error {
	return s.err
}

func (s *fakeRoomStore) MarkRoomActive(context.Context, string) error {
	return s.err
}

func (s *fakeRoomStore) DestroyRoom(context.Context, string) error {
	return s.err
}

func (s *fakeRoomStore) MarkAllActiveRoomsGracePeriod(context.Context, time.Time, time.Time) (int64, error) {
	return 0, s.err
}

func (s *fakeRoomStore) CleanupExpiredRoomCodes(context.Context, time.Time) ([]string, error) {
	return nil, s.err
}

type fakeRoomRuntime struct {
	roomID          string
	hostUserID      string
	mediaID         string
	mediaDurationMs *int64
	state           room.State
	storedState     room.State
	storeCalls      int
}

func (r *fakeRoomRuntime) RegisterCreatedRoomWithMedia(
	roomID string,
	hostUserID string,
	mediaID string,
	mediaDurationMs *int64,
) roomStateSnapshotter {
	r.roomID = roomID
	r.hostUserID = hostUserID
	r.mediaID = mediaID
	r.mediaDurationMs = mediaDurationMs
	return fakeRoomSnapshot{state: r.state}
}

func (r *fakeRoomRuntime) StoreLatestRoomState(_ context.Context, state room.State) error {
	r.storedState = state
	r.storeCalls++
	return nil
}

type fakeRoomSnapshot struct {
	state room.State
}

func (s fakeRoomSnapshot) StateSnapshot() room.State {
	return s.state
}

type fakeRoomAuthorityClaimer struct {
	roomID     string
	instanceID string
	err        error
	claimed    bool
}

func (c *fakeRoomAuthorityClaimer) ClaimAuthority(_ context.Context, roomID string, instanceID string) (cache.RoomAuthorityLease, bool, error) {
	c.roomID = roomID
	c.instanceID = instanceID
	if c.err != nil {
		return cache.RoomAuthorityLease{}, false, c.err
	}
	claimed := true
	if !c.claimed {
		c.claimed = true
	} else {
		claimed = c.claimed
	}
	return cache.RoomAuthorityLease{InstanceID: instanceID, Epoch: 1, Status: cache.RoomAuthorityStatusActive}, claimed, nil
}
