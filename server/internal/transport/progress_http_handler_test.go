package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watch_together/server/internal/progress"
)

func TestUpdateProgressFlow(t *testing.T) {
	lastWatchedAt := time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC)
	store := &fakeProgressStore{
		summary: progress.Summary{
			MediaItemID:         "media_001",
			LastPositionSeconds: 564,
			DurationSeconds:     1458,
			Completed:           false,
			LastWatchedAt:       lastWatchedAt,
		},
	}
	handler := NewProgressHTTPHandler(progress.NewService(store))
	request := httptest.NewRequest(
		http.MethodPut,
		"/me/media-progress/media_001",
		strings.NewReader(`{"lastPositionSeconds":564,"durationSeconds":1458,"completed":false}`),
	)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if store.lastParams.UserID != "user_a" {
		t.Fatalf("expected user_a, got %q", store.lastParams.UserID)
	}
	if store.lastParams.MediaItemID != "media_001" {
		t.Fatalf("expected media_001, got %q", store.lastParams.MediaItemID)
	}

	var response struct {
		Data progressResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Data.LastWatchedAt != "2026-04-27T10:00:00Z" {
		t.Fatalf("unexpected lastWatchedAt %q", response.Data.LastWatchedAt)
	}
}

func TestUpdateProgressRejectsInvalidProgress(t *testing.T) {
	handler := NewProgressHTTPHandler(progress.NewService(&fakeProgressStore{}))
	request := httptest.NewRequest(
		http.MethodPut,
		"/me/media-progress/media_001",
		strings.NewReader(`{"lastPositionSeconds":200,"durationSeconds":100,"completed":false}`),
	)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestUpdateProgressRequiresAccessToken(t *testing.T) {
	handler := NewProgressHTTPHandler(progress.NewService(&fakeProgressStore{}))
	request := httptest.NewRequest(
		http.MethodPut,
		"/me/media-progress/media_001",
		strings.NewReader(`{"lastPositionSeconds":10,"durationSeconds":100,"completed":false}`),
	)
	recorder := httptest.NewRecorder()

	handler.Update(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

type fakeProgressStore struct {
	summary    progress.Summary
	lastParams progress.UpdateParams
	err        error
}

func (s *fakeProgressStore) UpdateMediaProgress(_ context.Context, params progress.UpdateParams) (progress.Summary, error) {
	s.lastParams = params
	if s.err != nil {
		return progress.Summary{}, s.err
	}
	return s.summary, nil
}

func (s *fakeProgressStore) GetUserProgress(context.Context, string, string) (progress.Summary, bool, error) {
	if s.err != nil {
		return progress.Summary{}, false, s.err
	}
	return s.summary, true, nil
}

func (s *fakeProgressStore) BatchGetUserProgress(context.Context, string, []string) ([]progress.Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []progress.Summary{s.summary}, nil
}

func (s *fakeProgressStore) ListRecentUserProgress(context.Context, progress.RecentParams) ([]progress.Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []progress.Summary{s.summary}, nil
}
