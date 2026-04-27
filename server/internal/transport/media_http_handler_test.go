package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"watch_together/server/internal/media"
)

func TestMediaTagsReturnsFeaturedAndAllTags(t *testing.T) {
	handler := NewMediaHTTPHandler(media.NewService(&fakeMediaStore{
		tags: media.TagList{
			FeaturedTags: []media.Tag{{ID: "tag_1", Slug: "healing", Name: "治愈"}},
			AllTags:      []media.Tag{{ID: "tag_2", Slug: "campus", Name: "校园"}},
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
				Title:      "紫罗兰永恒花园",
				DurationMs: &durationMs,
				Tags:       []media.ItemTag{{Slug: "healing", Name: "治愈"}},
			},
			{ID: "media_2", Title: "葬送的芙莉莲"},
		},
	}
	handler := NewMediaHTTPHandler(media.NewService(store))

	request := httptest.NewRequest(http.MethodGet, "/media/items?query=紫罗兰&tag=healing&limit=1&cursor=2", nil)
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if store.lastSearch.Query != "紫罗兰" {
		t.Fatalf("expected query 紫罗兰, got %q", store.lastSearch.Query)
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
	if response.Meta.Page == nil || response.Meta.Page.NextCursor == nil || *response.Meta.Page.NextCursor != "3" {
		t.Fatalf("expected next cursor 3, got %+v", response.Meta.Page)
	}
}

func TestMediaItemsRejectsInvalidCursor(t *testing.T) {
	handler := NewMediaHTTPHandler(media.NewService(&fakeMediaStore{}))
	request := httptest.NewRequest(http.MethodGet, "/media/items?cursor=nope", nil)
	recorder := httptest.NewRecorder()

	handler.Items(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

type fakeMediaStore struct {
	tags       media.TagList
	items      []media.Item
	lastSearch media.StoreSearchParams
}

func (s *fakeMediaStore) ListTags(_ context.Context, _ int) (media.TagList, error) {
	return s.tags, nil
}

func (s *fakeMediaStore) SearchItems(_ context.Context, params media.StoreSearchParams) ([]media.Item, error) {
	s.lastSearch = params
	return s.items, nil
}
