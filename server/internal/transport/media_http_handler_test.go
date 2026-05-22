package transport

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watch_together/server/internal/media"
)

func TestMediaTagsReturnsFeaturedAndAllTags(t *testing.T) {
	handler := NewMediaHTTPHandler(media.NewService(&fakeMediaStore{
		tags: media.TagList{
			FeaturedTags: []media.Tag{{ID: "tag_1", Slug: "healing", Name: "Healing"}},
			AllTags:      []media.Tag{{ID: "tag_2", Slug: "campus", Name: "Campus"}},
		},
	}))

	recorder := httptest.NewRecorder()
	handler.Tags(recorder, httptest.NewRequest(http.MethodGet, "/media/tags", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Data mediaTagsResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.FeaturedTags) != 1 || response.Data.FeaturedTags[0].Slug != "healing" {
		t.Fatalf("unexpected featured tags: %+v", response.Data.FeaturedTags)
	}
	if len(response.Data.AllTags) != 1 || response.Data.AllTags[0].Slug != "campus" {
		t.Fatalf("unexpected all tags: %+v", response.Data.AllTags)
	}
}

func TestMediaItemsSearchesWithQueryTagAndPagination(t *testing.T) {
	durationMs := int64(1_458_000)
	store := &fakeMediaStore{
		items: []media.Item{
			{
				ID:         "media_1",
				Title:      "Violet Evergarden",
				MediaURL:   "http://127.0.0.1:9000/media/tmp/media/show/hls/master.m3u8",
				DurationMs: &durationMs,
				Tags:       []media.ItemTag{{Slug: "healing", Name: "Healing"}},
			},
			{ID: "media_2", Title: "Frieren"},
		},
	}
	handler := NewMediaHTTPHandler(media.NewService(store))

	request := httptest.NewRequest(http.MethodGet, "/media/items?query=violet&tag=healing&limit=1&cursor=2", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if store.lastSearch.Query != "violet" {
		t.Fatalf("expected query violet, got %q", store.lastSearch.Query)
	}
	if store.lastSearch.Tag != "healing" {
		t.Fatalf("expected tag healing, got %q", store.lastSearch.Tag)
	}
	if store.lastSearch.Limit != 2 {
		t.Fatalf("expected store limit 2, got %d", store.lastSearch.Limit)
	}
	if store.lastSearch.Offset != 2 {
		t.Fatalf("expected offset 2, got %d", store.lastSearch.Offset)
	}

	var response struct {
		Data mediaItemsResponse `json:"data"`
		Meta apiMeta            `json:"meta"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Items) != 1 {
		t.Fatalf("expected one returned item, got %d", len(response.Data.Items))
	}
	if response.Data.Items[0].ID != "media_1" {
		t.Fatalf("expected media_1, got %q", response.Data.Items[0].ID)
	}
	if !strings.Contains(response.Data.Items[0].MediaURL, "/media/playback/media_1/master.m3u8") {
		t.Fatalf("expected signed playback mediaUrl, got %q", response.Data.Items[0].MediaURL)
	}
	if strings.Contains(response.Data.Items[0].MediaURL, "media/tmp/media/show/hls/master.m3u8") {
		t.Fatalf("expected API not to expose raw HLS url, got %q", response.Data.Items[0].MediaURL)
	}
	if response.Meta.Page == nil || response.Meta.Page.NextCursor == nil || *response.Meta.Page.NextCursor != "3" {
		t.Fatalf("expected next cursor 3, got %+v", response.Meta.Page)
	}
}

func TestMediaPlaybackRedirectsWhenSignatureIsValid(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		playbackItem: media.PlaybackItem{
			ID:       "media_1",
			MediaURL: "https://cdn.example.com/media_1/hls/master.m3u8",
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			Now:           func() time.Time { return now },
		},
	)
	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	signedURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")

	request := httptest.NewRequest(http.MethodGet, signedURL, nil)
	recorder := httptest.NewRecorder()

	handler.Playback(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusFound, recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != store.playbackItem.MediaURL {
		t.Fatalf("expected redirect to raw media url, got %q", location)
	}
}

func TestMediaPlaybackRejectsExpiredSignature(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(&fakeMediaStore{}),
		nil,
		MediaPlaybackConfig{
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			Now:           func() time.Time { return now },
		},
	)
	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	signedURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")
	handler.playbackSigner.now = func() time.Time { return now.Add(2 * time.Hour) }

	request := httptest.NewRequest(http.MethodGet, signedURL, nil)
	recorder := httptest.NewRecorder()

	handler.Playback(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

func TestMediaItemsCanReturnNginxAuthRequestURL(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		items: []media.Item{
			{
				ID:       "media_1",
				Title:    "Violet Evergarden",
				MediaURL: "https://media.example.com/watch-together-media/media/show/hls/master.m3u8",
			},
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryNginxAuthRequest,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			PublicBaseURL: "https://media.example.com/watch-together-media",
			StorageBucket: "watch-together-media",
			Now:           func() time.Time { return now },
		},
	)
	request := httptest.NewRequest(http.MethodGet, "/media/items", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data mediaItemsResponse `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(response.Data.Items))
	}
	mediaURL := response.Data.Items[0].MediaURL
	if !strings.Contains(mediaURL, "/media/playback/media_1/master.m3u8?") {
		t.Fatalf("expected playback url, got %q", mediaURL)
	}
	if !strings.Contains(mediaURL, "expires=") || !strings.Contains(mediaURL, "sig=") {
		t.Fatalf("expected signed playback url, got %q", mediaURL)
	}
}

func TestNginxAuthAcceptsSignedMediaCookie(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(&fakeMediaStore{}),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryNginxAuthRequest,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			Now:           func() time.Time { return now },
		},
	)
	request := httptest.NewRequest(http.MethodGet, "/media/internal/auth", nil)
	request.Header.Set("X-Original-URI", "/media/show/hls/720p-fast/segment_00001.ts")
	request.AddCookie(&http.Cookie{
		Name:  nginxMediaCookieName,
		Value: handler.delivery.signer.SignedNginxCookieValue("media/show/hls"),
	})
	recorder := httptest.NewRecorder()

	handler.NginxAuth(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
	}
}

func TestNginxAuthRequestPlaybackSetsCookieAndRedirects(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		playbackItem: media.PlaybackItem{
			ID:       "media_1",
			MediaURL: "https://media.example.com/watch-together-media/media/show/hls/master.m3u8",
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryNginxAuthRequest,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			PublicBaseURL: "https://media.example.com/watch-together-media",
			StorageBucket: "watch-together-media",
			Now:           func() time.Time { return now },
		},
	)
	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	signedURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")
	request := httptest.NewRequest(http.MethodGet, signedURL, nil)
	recorder := httptest.NewRecorder()

	handler.Playback(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusFound, recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); !strings.HasPrefix(location, "https://media.example.com/watch-together-media/media/show/hls/master.m3u8?") {
		t.Fatalf("expected nginx redirect location, got %q", location)
	}
	foundCookie := false
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == nginxMediaCookieName && cookie.Value != "" {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Fatalf("expected %s cookie to be set", nginxMediaCookieName)
	}
}

func TestNginxAuthRequestPlaybackRequiresPublicBaseURL(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		playbackItem: media.PlaybackItem{
			ID:       "media_1",
			MediaURL: "https://media.example.com/watch-together-media/media/show/hls/master.m3u8",
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryNginxAuthRequest,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			StorageBucket: "watch-together-media",
			Now:           func() time.Time { return now },
		},
	)
	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	signedURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")
	request := httptest.NewRequest(http.MethodGet, signedURL, nil)
	recorder := httptest.NewRecorder()

	handler.Playback(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	}
}

func TestMinIOPresignModeFailsClosedWhenStorageIsMissing(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		playbackItem: media.PlaybackItem{
			ID:       "media_1",
			MediaURL: "https://media.example.com/watch-together-media/media/show/hls/master.m3u8",
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryMinIOPresign,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			Now:           func() time.Time { return now },
		},
	)
	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	signedURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")
	request := httptest.NewRequest(http.MethodGet, signedURL, nil)
	recorder := httptest.NewRecorder()

	handler.Playback(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestMinIOPresignModeUsesObjectStoreAdapter(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	store := &fakeMediaStore{
		playbackItem: media.PlaybackItem{
			ID:       "media_1",
			MediaURL: "https://media.example.com/watch-together-media/media/show/hls/master.m3u8",
		},
	}
	handler := NewMediaHTTPHandlerWithTokenVerifier(
		media.NewService(store),
		nil,
		MediaPlaybackConfig{
			DeliveryMode:  MediaDeliveryMinIOPresign,
			SigningSecret: "test-media-secret",
			URLTTL:        time.Hour,
			PublicBaseURL: "https://media.example.com/watch-together-media",
			StorageBucket: "watch-together-media",
			Now:           func() time.Time { return now },
		},
	)
	objectStore := &fakeObjectStore{
		objects: map[string]string{
			"media/show/hls/master.m3u8":          "#EXTM3U\n720p-fast/index.m3u8\n",
			"media/show/hls/720p-fast/index.m3u8": "#EXTM3U\n#EXTINF:6.000,\nsegment_00001.ts\n",
		},
	}
	handler.delivery.initErr = nil
	handler.delivery.objectStore = objectStore

	signingRequest := httptest.NewRequest(http.MethodGet, "http://api.example.com/media/items", nil)
	masterURL := handler.playbackSigner.SignedPlaybackURL(signingRequest, "media_1")
	masterRequest := httptest.NewRequest(http.MethodGet, masterURL, nil)
	masterRecorder := httptest.NewRecorder()

	handler.Playback(masterRecorder, masterRequest)

	if masterRecorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, masterRecorder.Code, masterRecorder.Body.String())
	}
	if !strings.Contains(masterRecorder.Body.String(), "/media/playback/media_1/720p-fast/index.m3u8") {
		t.Fatalf("expected rewritten variant playback URL, got body %s", masterRecorder.Body.String())
	}

	variantURL := handler.playbackSigner.SignedAssetPlaybackURL(signingRequest, "media_1", "720p-fast/index.m3u8")
	variantRequest := httptest.NewRequest(http.MethodGet, variantURL, nil)
	variantRecorder := httptest.NewRecorder()

	handler.Playback(variantRecorder, variantRequest)

	if variantRecorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, variantRecorder.Code, variantRecorder.Body.String())
	}
	wantSegmentURL := "https://minio.example.com/media/show/hls/720p-fast/segment_00001.ts?presigned=1"
	if !strings.Contains(variantRecorder.Body.String(), wantSegmentURL) {
		t.Fatalf("expected presigned segment URL %q, got body %s", wantSegmentURL, variantRecorder.Body.String())
	}
}

func TestMediaItemsRejectsInvalidCursor(t *testing.T) {
	handler := NewMediaHTTPHandler(media.NewService(&fakeMediaStore{}))
	request := httptest.NewRequest(http.MethodGet, "/media/items?cursor=nope", nil)
	request.Header.Set("Authorization", testAuthorizationHeader("user_a"))
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestMediaItemsRequiresAccessToken(t *testing.T) {
	handler := NewMediaHTTPHandler(media.NewService(&fakeMediaStore{}))
	request := httptest.NewRequest(http.MethodGet, "/media/items", nil)
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}
}

type fakeMediaStore struct {
	tags         media.TagList
	items        []media.Item
	playbackItem media.PlaybackItem
	lastSearch   media.StoreSearchParams
}

func (s *fakeMediaStore) ListTags(_ context.Context, _ int) (media.TagList, error) {
	return s.tags, nil
}

func (s *fakeMediaStore) SearchItems(_ context.Context, params media.StoreSearchParams) ([]media.Item, error) {
	s.lastSearch = params
	return s.items, nil
}

func (s *fakeMediaStore) FindPlaybackItem(_ context.Context, episodeID string) (media.PlaybackItem, error) {
	if s.playbackItem.ID == episodeID {
		return s.playbackItem, nil
	}
	return media.PlaybackItem{}, media.ErrMediaNotFound
}

type fakeObjectStore struct {
	objects map[string]string
}

func (s *fakeObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.objects[key])), nil
}

func (s *fakeObjectStore) PresignGetObject(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://minio.example.com/" + key + "?presigned=1", nil
}
