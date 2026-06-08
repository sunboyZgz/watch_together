package home

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mediacatalog "watch_together/server/internal/media"
)

var (
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrUserNotFound     = errors.New("user not found")
	ErrMediaUnavailable = errors.New("media service is unavailable")
)

type UserSummary struct {
	Nickname   string
	AvatarSeed string
	AvatarURL  *string
}

type WatchProgressSummary struct {
	MediaItemID         string
	Title               string
	CoverURL            *string
	LastPositionSeconds int
	DurationSeconds     int
}

type Summary struct {
	User             UserSummary
	LastWatched      *WatchProgressSummary
	ContinueWatching []WatchProgressSummary
}

type SummaryStore interface {
	GetHomeSummary(ctx context.Context, userID string) (Summary, error)
}

type MediaSummaryProvider interface {
	BatchGetEpisodeSummaries(ctx context.Context, episodeIDs []string) ([]mediacatalog.EpisodeSummary, error)
}

type Service struct {
	store          SummaryStore
	mediaSummaries MediaSummaryProvider
}

// NewService wires home-page summary reads to a persistent store.
func NewService(store SummaryStore) *Service {
	return &Service{store: store}
}

func NewServiceWithMediaSummaries(store SummaryStore, mediaSummaries MediaSummaryProvider) *Service {
	return &Service{store: store, mediaSummaries: mediaSummaries}
}

// Summary loads the user profile plus recent watching progress for the home page.
func (s *Service) Summary(ctx context.Context, userID string) (Summary, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Summary{}, ErrInvalidUserID
	}
	summary, err := s.store.GetHomeSummary(ctx, userID)
	if err != nil || s.mediaSummaries == nil {
		return summary, err
	}
	return s.enrichMedia(ctx, summary)
}

func (s *Service) enrichMedia(ctx context.Context, summary Summary) (Summary, error) {
	episodeIDs := collectSummaryEpisodeIDs(summary)
	if len(episodeIDs) == 0 {
		return summary, nil
	}
	summaries, err := s.mediaSummaries.BatchGetEpisodeSummaries(ctx, episodeIDs)
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
	}
	byID := make(map[string]mediacatalog.EpisodeSummary, len(summaries))
	for _, episode := range summaries {
		if strings.TrimSpace(episode.ID) == "" {
			continue
		}
		byID[episode.ID] = episode
	}
	if summary.LastWatched != nil {
		if enriched, ok := enrichWatchProgress(*summary.LastWatched, byID); ok {
			summary.LastWatched = &enriched
		} else {
			summary.LastWatched = nil
		}
	}
	remaining := make([]WatchProgressSummary, 0, len(summary.ContinueWatching))
	for _, item := range summary.ContinueWatching {
		if enriched, ok := enrichWatchProgress(item, byID); ok {
			remaining = append(remaining, enriched)
		}
	}
	summary.ContinueWatching = remaining
	return summary, nil
}

func collectSummaryEpisodeIDs(summary Summary) []string {
	ids := make([]string, 0, 1+len(summary.ContinueWatching))
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if summary.LastWatched != nil {
		add(summary.LastWatched.MediaItemID)
	}
	for _, item := range summary.ContinueWatching {
		add(item.MediaItemID)
	}
	return ids
}

func enrichWatchProgress(
	item WatchProgressSummary,
	summaries map[string]mediacatalog.EpisodeSummary,
) (WatchProgressSummary, bool) {
	summary, ok := summaries[item.MediaItemID]
	if !ok {
		return WatchProgressSummary{}, false
	}
	item.Title = summary.Title
	item.CoverURL = summary.CoverURL
	return item, true
}
