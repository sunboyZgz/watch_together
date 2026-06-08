package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watch_together/server/internal/home"
)

func TestHomeSummaryReturnsUserAndWatchingProgress(t *testing.T) {
	coverURL := "https://example.com/cover.jpg"
	handler := NewHomeHTTPHandler(home.NewService(&fakeHomeStore{
		summary: home.Summary{
			User: home.UserSummary{
				Nickname:   "Xingye",
				AvatarSeed: "xingye",
			},
			LastWatched: &home.WatchProgressSummary{
				MediaItemID:         "media_001",
				Title:               "紫罗兰永恒花园",
				CoverURL:            &coverURL,
				LastPositionSeconds: 564,
				DurationSeconds:     1458,
			},
			ContinueWatching: []home.WatchProgressSummary{
				{
					MediaItemID:         "media_002",
					Title:               "孤独摇滚!",
					LastPositionSeconds: 120,
					DurationSeconds:     1440,
				},
			},
		},
	}))

	request := httptest.NewRequest(http.MethodGet, "/home/summary", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_001"))
	recorder := httptest.NewRecorder()

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Data homeSummaryResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Data.User.Nickname != "Xingye" {
		t.Fatalf("expected nickname Xingye, got %q", response.Data.User.Nickname)
	}
	if response.Data.LastWatched == nil {
		t.Fatal("expected lastWatched to be present")
	}
	if response.Data.LastWatched.MediaItemID != "media_001" {
		t.Fatalf("expected media_001, got %q", response.Data.LastWatched.MediaItemID)
	}
	if len(response.Data.ContinueWatching) != 1 {
		t.Fatalf("expected 1 continue watching item, got %d", len(response.Data.ContinueWatching))
	}
}

func TestHomeSummaryAllowsEmptyWatchingProgress(t *testing.T) {
	handler := NewHomeHTTPHandler(home.NewService(&fakeHomeStore{
		summary: home.Summary{
			User: home.UserSummary{
				Nickname:   "Xingye",
				AvatarSeed: "xingye",
			},
		},
	}))

	request := httptest.NewRequest(http.MethodGet, "/home/summary", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_001"))
	recorder := httptest.NewRecorder()

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Data homeSummaryResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.LastWatched != nil {
		t.Fatal("expected empty lastWatched")
	}
	if len(response.Data.ContinueWatching) != 0 {
		t.Fatalf("expected empty continueWatching, got %d", len(response.Data.ContinueWatching))
	}
}

func TestHomeSummaryRequiresDevAccessToken(t *testing.T) {
	handler := NewHomeHTTPHandler(home.NewService(&fakeHomeStore{}))
	request := httptest.NewRequest(http.MethodGet, "/home/summary", nil)
	recorder := httptest.NewRecorder()

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestHomeSummaryReturnsServiceUnavailableWhenMediaBoundaryFails(t *testing.T) {
	handler := NewHomeHTTPHandler(home.NewService(&fakeHomeStore{err: home.ErrMediaUnavailable}))
	request := httptest.NewRequest(http.MethodGet, "/home/summary", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_001"))
	recorder := httptest.NewRecorder()

	handler.Summary(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "home service is unavailable") {
		t.Fatalf("expected stable unavailable response, got %s", recorder.Body.String())
	}
}

type fakeHomeStore struct {
	summary home.Summary
	err     error
}

func (s *fakeHomeStore) GetHomeSummary(_ context.Context, userID string) (home.Summary, error) {
	if userID != "user_001" {
		return home.Summary{}, home.ErrUserNotFound
	}
	if s.err != nil {
		return home.Summary{}, s.err
	}
	return s.summary, nil
}
