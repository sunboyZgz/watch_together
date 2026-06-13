package home

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"watch_together/server/internal/auth"
	"watch_together/server/internal/internalrpc"
	mediacatalog "watch_together/server/internal/media"
	progressapi "watch_together/server/internal/progress"
	"watch_together/server/internal/servicekit"
)

func TestRPCClientMatchesComposedHomeSummary(t *testing.T) {
	coverURL := "https://example.com/cover.jpg"
	service := NewServiceWithComposition(
		fakeHomeUserProfiles{},
		fakeHomeProgress{
			last: []progressapi.Summary{
				{MediaItemID: "episode-1", LastPositionSeconds: 10, DurationSeconds: 100, LastWatchedAt: time.Now()},
			},
			continueWatching: []progressapi.Summary{
				{MediaItemID: "episode-1", LastPositionSeconds: 10, DurationSeconds: 100},
				{MediaItemID: "episode-missing", LastPositionSeconds: 20, DurationSeconds: 100},
			},
		},
		&fakeMediaSummaryProvider{summaries: []mediacatalog.EpisodeSummary{
			{ID: "episode-1", Title: "Episode 1", CoverURL: &coverURL},
		}},
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

	summary, err := client.Summary(context.Background(), "user-a")
	if err != nil {
		t.Fatalf("rpc home summary: %v", err)
	}
	if summary.User.Nickname != "Xingye" {
		t.Fatalf("unexpected user summary: %+v", summary.User)
	}
	if summary.LastWatched == nil || summary.LastWatched.Title != "Episode 1" {
		t.Fatalf("expected enriched last watched, got %+v", summary.LastWatched)
	}
	if len(summary.ContinueWatching) != 1 || summary.ContinueWatching[0].MediaItemID != "episode-1" {
		t.Fatalf("expected missing media progress to be skipped, got %+v", summary.ContinueWatching)
	}
}

type fakeHomeUserProfiles struct{}

func (fakeHomeUserProfiles) GetUserProfile(context.Context, string) (auth.User, error) {
	return auth.User{ID: "user-a", Nickname: "Xingye", AvatarSeed: "xingye"}, nil
}

type fakeHomeProgress struct {
	last             []progressapi.Summary
	continueWatching []progressapi.Summary
}

func (p fakeHomeProgress) ListRecentUserProgress(_ context.Context, params progressapi.RecentParams) ([]progressapi.Summary, error) {
	if params.IncompleteOnly {
		return p.continueWatching, nil
	}
	return p.last, nil
}
