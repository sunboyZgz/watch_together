package progress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watch_together/server/internal/auth"
	"watch_together/server/internal/internalrpc"
	mediacatalog "watch_together/server/internal/media"
	"watch_together/server/internal/servicekit"
)

func TestRPCClientMatchesLocalProgressService(t *testing.T) {
	watchedAt := time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)
	store := &fakeProgressServiceStore{
		summary: Summary{
			MediaItemID:         "episode-canonical",
			LastPositionSeconds: 42,
			DurationSeconds:     120,
			LastWatchedAt:       watchedAt,
		},
	}
	service := NewServiceWithBoundaries(
		store,
		fakeUserProfileProvider{},
		&fakeMediaValidator{episode: mediacatalog.PlayableEpisode{ID: "episode-canonical", Playable: true}},
	)

	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "/internal.rpc", "secret", service)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		PathPrefix: "/internal.rpc",
		Timeout:    time.Second,
		AuthToken:  "secret",
		Service:    servicekit.Config{ServiceName: "roomserver", ServiceVersion: "test"},
	})

	updated, err := client.Update(context.Background(), UpdateParams{
		UserID:              "user-a",
		MediaItemID:         "episode-alias",
		LastPositionSeconds: 42,
		DurationSeconds:     120,
	})
	if err != nil {
		t.Fatalf("rpc update progress: %v", err)
	}
	if updated.MediaItemID != "episode-canonical" || store.lastParams.MediaItemID != "episode-canonical" {
		t.Fatalf("expected canonical episode through rpc path, got summary=%+v params=%+v", updated, store.lastParams)
	}

	recent, err := client.ListRecentUserProgress(context.Background(), RecentParams{UserID: "user-a", Limit: 1})
	if err != nil {
		t.Fatalf("rpc list recent progress: %v", err)
	}
	if len(recent) != 1 || !recent[0].LastWatchedAt.Equal(watchedAt) {
		t.Fatalf("unexpected recent progress from rpc path: %+v", recent)
	}
}

type fakeUserProfileProvider struct{}

func (fakeUserProfileProvider) GetUserProfile(context.Context, string) (auth.User, error) {
	return auth.User{ID: "user-a", Nickname: "A", AvatarSeed: "a"}, nil
}
