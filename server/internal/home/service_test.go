package home

import (
	"context"
	"errors"
	"testing"

	mediacatalog "watch_together/server/internal/media"
)

func TestSummaryEnrichesAndSkipsMissingMediaSummaries(t *testing.T) {
	coverURL := "https://example.com/cover.jpg"
	service := NewServiceWithMediaSummaries(
		&fakeSummaryStore{
			summary: Summary{
				User: UserSummary{Nickname: "Xingye", AvatarSeed: "xingye"},
				LastWatched: &WatchProgressSummary{
					MediaItemID:         "episode-missing",
					LastPositionSeconds: 12,
					DurationSeconds:     120,
				},
				ContinueWatching: []WatchProgressSummary{
					{MediaItemID: "episode-1", LastPositionSeconds: 30, DurationSeconds: 300},
					{MediaItemID: "episode-missing", LastPositionSeconds: 40, DurationSeconds: 400},
				},
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
	service := NewServiceWithMediaSummaries(
		&fakeSummaryStore{
			summary: Summary{
				User:        UserSummary{Nickname: "Xingye", AvatarSeed: "xingye"},
				LastWatched: &WatchProgressSummary{MediaItemID: "episode-1"},
			},
		},
		&fakeMediaSummaryProvider{err: mediaErr},
	)

	_, err := service.Summary(context.Background(), "user-1")
	if !errors.Is(err, ErrMediaUnavailable) {
		t.Fatalf("expected ErrMediaUnavailable, got %v", err)
	}
}

type fakeSummaryStore struct {
	summary Summary
	err     error
}

func (s *fakeSummaryStore) GetHomeSummary(context.Context, string) (Summary, error) {
	if s.err != nil {
		return Summary{}, s.err
	}
	return s.summary, nil
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
