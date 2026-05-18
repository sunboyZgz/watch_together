package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
					UserID:     "user_a",
					Nickname:   "Xingye",
					AvatarSeed: "xingye",
					AvatarURL:  &avatarURL,
					Role:       "host",
				},
			},
		},
	}
	handler := NewRoomHTTPHandler(room.NewManager(), roomapi.NewService(store))
	request := httptest.NewRequest(http.MethodGet, "/rooms/A7K2M9", nil)
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
	if len(response.Data.Members) != 1 {
		t.Fatalf("expected one member, got %d", len(response.Data.Members))
	}
	if response.Data.Members[0].Role != "host" {
		t.Fatalf("expected host role, got %q", response.Data.Members[0].Role)
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
