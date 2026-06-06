package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watch_together/server/internal/internalrpc"
)

func TestRPCStoreMatchesLocalMediaStore(t *testing.T) {
	localStore := fakeRPCMediaStore{
		tags: TagList{FeaturedTags: []Tag{{Slug: "featured", Name: "Featured"}}},
		items: []Item{{
			ID:       "episode-1",
			Title:    "Episode 1",
			MediaURL: "episode-1/hls/master.m3u8",
		}},
		playback: PlaybackItem{ID: "episode-1", MediaURL: "episode-1/hls/master.m3u8"},
	}
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", NewService(localStore))
	server := httptest.NewServer(mux)
	defer server.Close()

	rpcStore := NewRPCStore(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})
	tags, err := rpcStore.ListTags(context.Background(), 20)
	if err != nil {
		t.Fatalf("list tags through rpc: %v", err)
	}
	if len(tags.FeaturedTags) != 1 || tags.FeaturedTags[0].Slug != "featured" {
		t.Fatalf("unexpected tags: %+v", tags)
	}
	items, err := rpcStore.SearchItems(context.Background(), StoreSearchParams{Limit: 10})
	if err != nil {
		t.Fatalf("search through rpc: %v", err)
	}
	if len(items) != 1 || items[0].ID != "episode-1" {
		t.Fatalf("unexpected items: %+v", items)
	}
	playback, err := rpcStore.FindPlaybackItem(context.Background(), "episode-1")
	if err != nil {
		t.Fatalf("playback through rpc: %v", err)
	}
	if playback.ID != "episode-1" {
		t.Fatalf("unexpected playback: %+v", playback)
	}
}

func TestRPCStoreMapsPlaybackNotFound(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "", NewService(fakeRPCMediaStore{playbackErr: ErrMediaNotFound}))
	server := httptest.NewServer(mux)
	defer server.Close()

	rpcStore := NewRPCStore(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
	_, err := rpcStore.FindPlaybackItem(context.Background(), "missing")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
}

type fakeRPCMediaStore struct {
	tags        TagList
	items       []Item
	playback    PlaybackItem
	playbackErr error
}

func (s fakeRPCMediaStore) ListTags(context.Context, int) (TagList, error) {
	return s.tags, nil
}

func (s fakeRPCMediaStore) SearchItems(context.Context, StoreSearchParams) ([]Item, error) {
	return s.items, nil
}

func (s fakeRPCMediaStore) FindPlaybackItem(context.Context, string) (PlaybackItem, error) {
	if s.playbackErr != nil {
		return PlaybackItem{}, s.playbackErr
	}
	return s.playback, nil
}
