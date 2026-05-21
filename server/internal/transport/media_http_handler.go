package transport

import (
	"errors"
	"net/http"
	"strconv"

	"watch_together/server/internal/media"
)

type MediaHTTPHandler struct {
	mediaService  *media.Service
	tokenVerifier accessTokenVerifier
}

type mediaTagsResponse struct {
	FeaturedTags []mediaTagResponse `json:"featuredTags"`
	AllTags      []mediaTagResponse `json:"allTags"`
}

type mediaTagResponse struct {
	ID   string `json:"id,omitempty"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type mediaItemsResponse struct {
	Items []mediaItemResponse `json:"items"`
}

type mediaItemResponse struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Subtitle     *string            `json:"subtitle"`
	Description  *string            `json:"description"`
	CoverURL     *string            `json:"coverUrl"`
	MediaURL     string             `json:"mediaUrl,omitempty"`
	DurationMs   *int64             `json:"durationMs"`
	SeasonLabel  *string            `json:"seasonLabel"`
	EpisodeLabel *string            `json:"episodeLabel"`
	Tags         []mediaTagResponse `json:"tags"`
}

// NewMediaHTTPHandler builds the HTTP entrypoint for media catalog APIs.
func NewMediaHTTPHandler(mediaService *media.Service) *MediaHTTPHandler {
	return NewMediaHTTPHandlerWithTokenVerifier(mediaService, nil)
}

func NewMediaHTTPHandlerWithTokenVerifier(
	mediaService *media.Service,
	tokenVerifier accessTokenVerifier,
) *MediaHTTPHandler {
	return &MediaHTTPHandler{
		mediaService:  mediaService,
		tokenVerifier: tokenVerifier,
	}
}

// Tags handles GET /media/tags for the video-selection tag row and expanded panel.
func (h *MediaHTTPHandler) Tags(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	tags, err := h.mediaService.Tags(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "media tags request failed", nil)
		return
	}
	writeAPISuccess(w, http.StatusOK, mediaTagsToResponse(tags))
}

// Items handles GET /media/items with optional search, tag filtering, and cursor paging.
func (h *MediaHTTPHandler) Items(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}
	if _, ok := userIDFromAuthorization(r.Header.Get("Authorization"), h.tokenVerifier); !ok {
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid access token", nil)
		return
	}

	limit, err := limitFromQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be a positive integer", map[string]any{"field": "limit"})
		return
	}

	result, err := h.mediaService.Search(r.Context(), media.SearchParams{
		Query:  r.URL.Query().Get("query"),
		Tag:    r.URL.Query().Get("tag"),
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		if errors.Is(err, media.ErrInvalidPagination) {
			writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "cursor is invalid", map[string]any{"field": "cursor"})
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "media search request failed", nil)
		return
	}

	writeAPISuccessWithPage(w, http.StatusOK, mediaItemsResponse{
		Items: mediaItemsToResponse(result.Items),
	}, apiPage{
		Limit:      result.Limit,
		NextCursor: result.NextCursor,
	})
}

func (h *MediaHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.mediaService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "media service is unavailable", nil)
		return false
	}
	return true
}

func limitFromQuery(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}

func mediaTagsToResponse(tags media.TagList) mediaTagsResponse {
	return mediaTagsResponse{
		FeaturedTags: mediaTagListToResponse(tags.FeaturedTags),
		AllTags:      mediaTagListToResponse(tags.AllTags),
	}
}

func mediaTagListToResponse(tags []media.Tag) []mediaTagResponse {
	responses := make([]mediaTagResponse, 0, len(tags))
	for _, tag := range tags {
		responses = append(responses, mediaTagResponse{
			ID:   tag.ID,
			Slug: tag.Slug,
			Name: tag.Name,
		})
	}
	return responses
}

func mediaItemsToResponse(items []media.Item) []mediaItemResponse {
	responses := make([]mediaItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, mediaItemToResponse(item))
	}
	return responses
}

func mediaItemToResponse(item media.Item) mediaItemResponse {
	return mediaItemResponse{
		ID:           item.ID,
		Title:        item.Title,
		Subtitle:     item.Subtitle,
		Description:  item.Description,
		CoverURL:     item.CoverURL,
		MediaURL:     item.MediaURL,
		DurationMs:   item.DurationMs,
		SeasonLabel:  item.SeasonLabel,
		EpisodeLabel: item.EpisodeLabel,
		Tags:         itemTagsToResponse(item.Tags),
	}
}

func itemTagsToResponse(tags []media.ItemTag) []mediaTagResponse {
	responses := make([]mediaTagResponse, 0, len(tags))
	for _, tag := range tags {
		responses = append(responses, mediaTagResponse{
			Slug: tag.Slug,
			Name: tag.Name,
		})
	}
	return responses
}
