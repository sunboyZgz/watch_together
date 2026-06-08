package media

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

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
		detail: EpisodeDetail{
			ID:       "episode-1",
			Title:    "Episode 1",
			MediaURL: "episode-1/hls/master.m3u8",
		},
		playable:  PlayableEpisode{ID: "episode-1", Playable: true},
		summaries: []EpisodeSummary{{ID: "episode-1", Title: "Episode 1"}},
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
	authorized, err := rpcStore.AuthorizePlayback(context.Background(), "episode-1", "master.m3u8")
	if err != nil {
		t.Fatalf("authorize playback through rpc: %v", err)
	}
	if !authorized {
		t.Fatalf("expected playback authorization")
	}
	detail, err := rpcStore.FindEpisodeDetail(context.Background(), "episode-1")
	if err != nil {
		t.Fatalf("episode detail through rpc: %v", err)
	}
	if detail.ID != "episode-1" || detail.Title != "Episode 1" {
		t.Fatalf("unexpected episode detail: %+v", detail)
	}
	playable, err := rpcStore.ValidatePlayableEpisode(context.Background(), "episode-1")
	if err != nil {
		t.Fatalf("playable validation through rpc: %v", err)
	}
	if playable.ID != "episode-1" || !playable.Playable {
		t.Fatalf("unexpected playable episode: %+v", playable)
	}
	summaries, err := rpcStore.BatchFindEpisodeSummaries(context.Background(), []string{"episode-1"})
	if err != nil {
		t.Fatalf("episode summaries through rpc: %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != "episode-1" || summaries[0].Title != "Episode 1" {
		t.Fatalf("unexpected episode summaries: %+v", summaries)
	}
}

func TestRPCStoreMapsPlaybackNotFound(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "", NewService(fakeRPCMediaStore{
		playbackErr: ErrMediaNotFound,
		detailErr:   ErrMediaNotFound,
		playableErr: ErrMediaNotFound,
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	rpcStore := NewRPCStore(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
	_, err := rpcStore.FindPlaybackItem(context.Background(), "missing")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
	_, err = rpcStore.FindEpisodeDetail(context.Background(), "missing")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound for detail, got %v", err)
	}
	_, err = rpcStore.ValidatePlayableEpisode(context.Background(), "missing")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound for playable validation, got %v", err)
	}
}

func TestRPCStoreRejectsInvalidAuthToken(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", NewService(fakeRPCMediaStore{}))
	server := httptest.NewServer(mux)
	defer server.Close()

	rpcStore := NewRPCStore(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "wrong",
	})
	_, err := rpcStore.ListTags(context.Background(), 20)
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated connect error, got %v", err)
	}
}

type fakeRPCMediaStore struct {
	tags        TagList
	items       []Item
	playback    PlaybackItem
	detail      EpisodeDetail
	playable    PlayableEpisode
	playbackErr error
	detailErr   error
	playableErr error
	summaries   []EpisodeSummary
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

func (s fakeRPCMediaStore) FindEpisodeDetail(context.Context, string) (EpisodeDetail, error) {
	if s.detailErr != nil {
		return EpisodeDetail{}, s.detailErr
	}
	return s.detail, nil
}

func (s fakeRPCMediaStore) ValidatePlayableEpisode(context.Context, string) (PlayableEpisode, error) {
	if s.playableErr != nil {
		return PlayableEpisode{}, s.playableErr
	}
	return s.playable, nil
}

func (s fakeRPCMediaStore) BatchFindEpisodeSummaries(context.Context, []string) ([]EpisodeSummary, error) {
	return s.summaries, nil
}
