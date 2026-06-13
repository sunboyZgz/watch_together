package home

import (
	"context"
	"errors"
	"testing"
	"time"

	"watch_together/server/internal/auth"
	mediacatalog "watch_together/server/internal/media"
	progressapi "watch_together/server/internal/progress"
)

func TestSummaryEnrichesAndSkipsMissingMediaSummaries(t *testing.T) {
	coverURL := "https://example.com/cover.jpg"
	service := NewServiceWithComposition(
		serviceHomeUserProfiles{},
		serviceHomeProgress{
			last: []progressapi.Summary{
				{MediaItemID: "episode-missing", LastPositionSeconds: 12, DurationSeconds: 120, LastWatchedAt: time.Now()},
			},
			continueWatching: []progressapi.Summary{
				{MediaItemID: "episode-1", LastPositionSeconds: 30, DurationSeconds: 300},
				{MediaItemID: "episode-missing", LastPositionSeconds: 40, DurationSeconds: 400},
			},
		},
		&fakeMediaSummaryProvider{
			summaries: []mediacatalog.EpisodeSummary{
				{ID: "episode-1", Title: "Episode 1", CoverURL: &coverURL},
			},
		},
	)

	summary, err := service.Summary(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("summary: %v", err)
	}

	if summary.LastWatched != nil {
		t.Fatalf("expected missing last watched media to be skipped, got %+v", summary.LastWatched)
	}
	if len(summary.ContinueWatching) != 1 {
		t.Fatalf("expected one continue watching item, got %+v", summary.ContinueWatching)
	}
	item := summary.ContinueWatching[0]
	if item.MediaItemID != "episode-1" || item.Title != "Episode 1" || item.CoverURL == nil || *item.CoverURL != coverURL {
		t.Fatalf("expected enriched continue watching item, got %+v", item)
	}
}

func TestSummaryMapsMediaSummaryFailure(t *testing.T) {
	mediaErr := errors.New("media rpc unavailable")
	service := NewServiceWithComposition(
		serviceHomeUserProfiles{},
		serviceHomeProgress{
			last: []progressapi.Summary{
				{MediaItemID: "episode-1", LastPositionSeconds: 1, DurationSeconds: 100, LastWatchedAt: time.Now()},
			},
		},
		&fakeMediaSummaryProvider{err: mediaErr},
	)

	_, err := service.Summary(context.Background(), "user-1")
	if !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("expected ErrMediaUnavailable, got %v", err)
	}
}

type serviceHomeUserProfiles struct{}

func (serviceHomeUserProfiles) GetUserProfile(context.Context, string) (auth.User, error) {
	return auth.User{ID: "user-a", Nickname: "Xingye", AvatarSeed: "xingye"}, nil
}

type serviceHomeProgress struct {
	last             []progressapi.Summary
	continueWatching []progressapi.Summary
	err              error
}

func (p serviceHomeProgress) ListRecentUserProgress(_ context.Context, params progressapi.RecentParams) ([]progressapi.Summary, error) {
	if p.err != nil {
		return nil, p.err
	}
	if params.IncompleteOnly {
		return p.continueWatching, nil
	}
	return p.last, nil
}

type fakeMediaSummaryProvider struct {
	summaries []mediacatalog.EpisodeSummary
	err       error
}

func (p *fakeMediaSummaryProvider) BatchGetEpisodeSummaries(context.Context, []string) ([]mediacatalog.EpisodeSummary, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.summaries, nil
}
