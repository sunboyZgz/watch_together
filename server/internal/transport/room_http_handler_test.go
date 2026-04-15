package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watch_together/server/internal/protocol"
	"watch_together/server/internal/room"
)

func TestCreateRoomFlow(t *testing.T) {
	handler := NewRoomHTTPHandler(room.NewManager())

	request := httptest.NewRequest(
		http.MethodPost,
		"/rooms",
		strings.NewReader(`{"userId":"user_a","mediaId":"sample_001"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateRoom(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	var response protocol.CreateRoomResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(response.RoomID) != 6 {
		t.Fatalf("expected 6-char room id, got %q", response.RoomID)
	}
	if response.RoomState.RoomID != response.RoomID {
		t.Fatalf("expected roomState.roomId to match roomId")
	}
	if response.RoomState.HostUserID != "user_a" {
		t.Fatalf("expected host user user_a, got %q", response.RoomState.HostUserID)
	}
	if response.RoomState.MediaID != "sample_001" {
		t.Fatalf("expected mediaId sample_001, got %q", response.RoomState.MediaID)
	}
	if !response.RoomState.Paused {
		t.Fatalf("expected new room paused=true")
	}
	if response.RoomState.PositionMs != 0 {
		t.Fatalf("expected position 0, got %d", response.RoomState.PositionMs)
	}
	if response.RoomState.PlaybackRate != 1.0 {
		t.Fatalf("expected playbackRate 1.0, got %f", response.RoomState.PlaybackRate)
	}
	if response.RoomState.Seq != 1 {
		t.Fatalf("expected seq 1, got %d", response.RoomState.Seq)
	}
}
