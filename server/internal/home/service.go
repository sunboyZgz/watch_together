package home

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"watch_together/server/internal/auth"
	mediacatalog "watch_together/server/internal/media"
	progressapi "watch_together/server/internal/progress"
)

var (
	ErrInvalidUserID       = errors.New("invalid user id")
	ErrUserNotFound        = errors.New("user not found")
	ErrIdentityUnavailable = errors.New("identity service is unavailable")
	ErrProgressUnavailable = errors.New("progress service is unavailable")
	ErrMediaUnavailable    = errors.New("media service is unavailable")
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

type BusinessService interface {
	Summary(ctx context.Context, userID string) (Summary, error)
}

type UserProfileProvider interface {
	GetUserProfile(ctx context.Context, userID string) (auth.User, error)
}

type ProgressProvider interface {
	ListRecentUserProgress(ctx context.Context, params progressapi.RecentParams) ([]progressapi.Summary, error)
}

type MediaSummaryProvider interface {
	BatchGetEpisodeSummaries(ctx context.Context, episodeIDs []string) ([]mediacatalog.EpisodeSummary, error)
}

type Service struct {
	store          SummaryStore
	userProfiles   UserProfileProvider
	progress       ProgressProvider
	mediaSummaries MediaSummaryProvider
}

// NewService wires home-page summary reads to a persistent store.
func NewService(store SummaryStore) *Service {
	return &Service{store: store}
}

func NewServiceWithMediaSummaries(store SummaryStore, mediaSummaries MediaSummaryProvider) *Service {
	return &Service{store: store, mediaSummaries: mediaSummaries}
}

func NewServiceWithComposition(
	userProfiles UserProfileProvider,
	progress ProgressProvider,
	mediaSummaries MediaSummaryProvider,
) *Service {
	return &Service{
		userProfiles:   userProfiles,
		progress:       progress,
		mediaSummaries: mediaSummaries,
	}
}

// Summary loads the user profile plus recent watching progress for the home page.
func (s *Service) Summary(ctx context.Context, userID string) (Summary, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Summary{}, ErrInvalidUserID
	}
	if s == nil {
		return Summary{}, ErrIdentityUnavailable
	}
	if s.store == nil {
		return s.composedSummary(ctx, userID)
	}
	summary, err := s.store.GetHomeSummary(ctx, userID)
	if err != nil || s.mediaSummaries == nil {
		return summary, err
	}
	return s.enrichMedia(ctx, summary)
}

func (s *Service) composedSummary(ctx context.Context, userID string) (Summary, error) {
	if s.userProfiles == nil || s.progress == nil || s.mediaSummaries == nil {
		return Summary{}, ErrProgressUnavailable
	}
	user, err := s.userProfiles.GetUserProfile(ctx, userID)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrInvalidInput) {
			return Summary{}, ErrUserNotFound
		}
		return Summary{}, fmt.Errorf("%w: %v", ErrIdentityUnavailable, err)
	}
	lastWatched, err := s.progress.ListRecentUserProgress(ctx, progressapi.RecentParams{
		UserID: userID,
		Limit:  1,
	})
	if err != nil {
		if errors.Is(err, progressapi.ErrUserNotFound) {
			return Summary{}, ErrUserNotFound
		}
		return Summary{}, fmt.Errorf("%w: %v", ErrProgressUnavailable, err)
	}
	continueWatching, err := s.progress.ListRecentUserProgress(ctx, progressapi.RecentParams{
		UserID:         userID,
		Limit:          2,
		IncompleteOnly: true,
	})
	if err != nil {
		if errors.Is(err, progressapi.ErrUserNotFound) {
			return Summary{}, ErrUserNotFound
		}
		return Summary{}, fmt.Errorf("%w: %v", ErrProgressUnavailable, err)
	}
	summary := Summary{
		User: UserSummary{
			Nickname:   user.Nickname,
			AvatarSeed: user.AvatarSeed,
			AvatarURL:  user.AvatarURL,
		},
		ContinueWatching: progressSummariesToWatchItems(continueWatching),
	}
	if len(lastWatched) > 0 {
		item := progressSummaryToWatchItem(lastWatched[0])
		summary.LastWatched = &item
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

func progressSummariesToWatchItems(summaries []progressapi.Summary) []WatchProgressSummary {
	items := make([]WatchProgressSummary, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, progressSummaryToWatchItem(summary))
	}
	return items
}

func progressSummaryToWatchItem(summary progressapi.Summary) WatchProgressSummary {
	return WatchProgressSummary{
		MediaItemID:         summary.MediaItemID,
		LastPositionSeconds: summary.LastPositionSeconds,
		DurationSeconds:     summary.DurationSeconds,
	}
}
