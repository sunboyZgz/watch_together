package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"watch_together/server/internal/progress"
)

type ProgressHTTPHandler struct {
	progressService progress.BusinessService
	tokenVerifier   accessTokenVerifier
}

type updateProgressRequest struct {
	LastPositionSeconds int     `json:"lastPositionSeconds"`
	DurationSeconds     int     `json:"durationSeconds"`
	Completed           bool    `json:"completed"`
	CompletionSource    *string `json:"completionSource,omitempty"`
}

type progressResponse struct {
	MediaItemID         string `json:"mediaItemId"`
	LastPositionSeconds int    `json:"lastPositionSeconds"`
	DurationSeconds     int    `json:"durationSeconds"`
	Completed           bool   `json:"completed"`
	LastWatchedAt       string `json:"lastWatchedAt"`
}

// NewProgressHTTPHandler builds the HTTP entrypoint for user media progress writes.
func NewProgressHTTPHandler(progressService progress.BusinessService) *ProgressHTTPHandler {
	return NewProgressHTTPHandlerWithTokenVerifier(progressService, nil)
}

func NewProgressHTTPHandlerWithTokenVerifier(
	progressService progress.BusinessService,
	tokenVerifier accessTokenVerifier,
) *ProgressHTTPHandler {
	return &ProgressHTTPHandler{
		progressService: progressService,
		tokenVerifier:   tokenVerifier,
	}
}

// Update handles PUT /me/media-progress/{mediaItemId}.
func (h *ProgressHTTPHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodPut {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	userID, ok := userIDFromAuthorization(r.Header.Get("Authorization"), h.tokenVerifier)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
		return
	}

	mediaItemID, ok := mediaItemIDFromProgressPath(r.URL.Path)
	if !ok {
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "progress route not found", nil)
		return
	}

	var request updateProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body", nil)
		return
	}

	summary, err := h.progressService.Update(r.Context(), progress.UpdateParams{
		UserID:              userID,
		MediaItemID:         mediaItemID,
		LastPositionSeconds: request.LastPositionSeconds,
		DurationSeconds:     request.DurationSeconds,
		Completed:           request.Completed,
		CompletionSource:    request.CompletionSource,
	})
	if err != nil {
		h.writeProgressError(w, err)
		return
	}

	writeAPISuccess(w, http.StatusOK, progressSummaryToResponse(summary))
}

func (h *ProgressHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.progressService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "progress service is unavailable", nil)
		return false
	}
	return true
}

func (h *ProgressHTTPHandler) writeProgressError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, progress.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "media progress request is invalid", nil)
	case errors.Is(err, progress.ErrUserNotFound):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found", nil)
	case errors.Is(err, progress.ErrMediaNotFound):
		writeAPIError(w, http.StatusNotFound, "NOT_FOUND", "media item not found", nil)
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "media progress request failed", nil)
	}
}

func mediaItemIDFromProgressPath(path string) (string, bool) {
	const prefix = "/me/media-progress/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	mediaItemID := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	return mediaItemID, mediaItemID != ""
}

func progressSummaryToResponse(summary progress.Summary) progressResponse {
	return progressResponse{
		MediaItemID:         summary.MediaItemID,
		LastPositionSeconds: summary.LastPositionSeconds,
		DurationSeconds:     summary.DurationSeconds,
		Completed:           summary.Completed,
		LastWatchedAt:       summary.LastWatchedAt.UTC().Format(time.RFC3339),
	}
}
