package transport

import (
	"errors"
	"net/http"

	"watch_together/server/internal/home"
)

type HomeHTTPHandler struct {
	homeService   *home.Service
	tokenVerifier accessTokenVerifier
}

type homeSummaryResponse struct {
	User             homeUserResponse        `json:"user"`
	LastWatched      *watchProgressResponse  `json:"lastWatched"`
	ContinueWatching []watchProgressResponse `json:"continueWatching"`
}

type homeUserResponse struct {
	Nickname   string  `json:"nickname"`
	AvatarSeed string  `json:"avatarSeed"`
	AvatarURL  *string `json:"avatarUrl"`
}

type watchProgressResponse struct {
	MediaItemID         string  `json:"mediaItemId"`
	Title               string  `json:"title"`
	CoverURL            *string `json:"coverUrl"`
	LastPositionSeconds int     `json:"lastPositionSeconds"`
	DurationSeconds     int     `json:"durationSeconds"`
}

// NewHomeHTTPHandler builds the HTTP entrypoint for the Android home page.
func NewHomeHTTPHandler(homeService *home.Service) *HomeHTTPHandler {
	return NewHomeHTTPHandlerWithTokenVerifier(homeService, nil)
}

func NewHomeHTTPHandlerWithTokenVerifier(
	homeService *home.Service,
	tokenVerifier accessTokenVerifier,
) *HomeHTTPHandler {
	return &HomeHTTPHandler{
		homeService:   homeService,
		tokenVerifier: tokenVerifier,
	}
}

// Summary handles GET /home/summary using the current dev access token.
func (h *HomeHTTPHandler) Summary(w http.ResponseWriter, r *http.Request) {
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

	summary, err := h.homeService.Summary(r.Context(), userID)
	if err != nil {
		h.writeHomeError(w, err)
		return
	}
	writeAPISuccess(w, http.StatusOK, homeSummaryToResponse(summary))
}

func (h *HomeHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.homeService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "home service is unavailable", nil)
		return false
	}
	return true
}

func (h *HomeHTTPHandler) writeHomeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, home.ErrInvalidUserID):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
	case errors.Is(err, home.ErrUserNotFound):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found", nil)
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "home summary request failed", nil)
	}
}

func homeSummaryToResponse(summary home.Summary) homeSummaryResponse {
	return homeSummaryResponse{
		User: homeUserResponse{
			Nickname:   summary.User.Nickname,
			AvatarSeed: summary.User.AvatarSeed,
			AvatarURL:  summary.User.AvatarURL,
		},
		LastWatched:      watchProgressToResponsePtr(summary.LastWatched),
		ContinueWatching: watchProgressListToResponse(summary.ContinueWatching),
	}
}

func watchProgressToResponsePtr(progress *home.WatchProgressSummary) *watchProgressResponse {
	if progress == nil {
		return nil
	}
	response := watchProgressToResponse(*progress)
	return &response
}

func watchProgressListToResponse(items []home.WatchProgressSummary) []watchProgressResponse {
	responses := make([]watchProgressResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, watchProgressToResponse(item))
	}
	return responses
}

func watchProgressToResponse(progress home.WatchProgressSummary) watchProgressResponse {
	return watchProgressResponse{
		MediaItemID:         progress.MediaItemID,
		Title:               progress.Title,
		CoverURL:            progress.CoverURL,
		LastPositionSeconds: progress.LastPositionSeconds,
		DurationSeconds:     progress.DurationSeconds,
	}
}
