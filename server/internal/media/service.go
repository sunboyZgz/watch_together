package media

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	defaultLimit = 20
	maxLimit     = 50
	tagListLimit = 20
)

var ErrInvalidPagination = errors.New("invalid pagination")
var ErrMediaNotFound = errors.New("media not found")

type Tag struct {
	ID   string
	Slug string
	Name string
}

type ItemTag struct {
	Slug string
	Name string
}

type Item struct {
	ID           string
	Title        string
	Subtitle     *string
	Description  *string
	CoverURL     *string
	MediaURL     string
	DurationMs   *int64
	SeasonLabel  *string
	EpisodeLabel *string
	Tags         []ItemTag
}

type PlaybackItem struct {
	ID       string
	MediaURL string
}

type EpisodeDetail struct {
	ID           string
	Title        string
	Subtitle     *string
	MediaURL     string
	DurationMs   *int64
	SeasonLabel  *string
	EpisodeLabel *string
}

type PlayableEpisode struct {
	ID       string
	Playable bool
}

type TagList struct {
	FeaturedTags []Tag
	AllTags      []Tag
}

type ItemSearchResult struct {
	Items      []Item
	Limit      int
	NextCursor *string
}

type SearchParams struct {
	Query  string
	Tag    string
	Limit  int
	Cursor string
}

type Store interface {
	ListTags(ctx context.Context, allLimit int) (TagList, error)
	SearchItems(ctx context.Context, params StoreSearchParams) ([]Item, error)
	FindPlaybackItem(ctx context.Context, episodeID string) (PlaybackItem, error)
	FindEpisodeDetail(ctx context.Context, episodeID string) (EpisodeDetail, error)
	ValidatePlayableEpisode(ctx context.Context, episodeID string) (PlayableEpisode, error)
}

type StoreSearchParams struct {
	Query  string
	Tag    string
	Limit  int
	Offset int
}

type Service struct {
	store Store
}

// NewService wires media catalog reads to a persistent store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Tags returns featured tags for the default row and active tags for the expanded list.
func (s *Service) Tags(ctx context.Context) (TagList, error) {
	return s.store.ListTags(ctx, tagListLimit)
}

// Search validates query params and returns one page of media items.
func (s *Service) Search(ctx context.Context, params SearchParams) (ItemSearchResult, error) {
	limit := normalizeLimit(params.Limit)
	offset, err := parseCursor(params.Cursor)
	if err != nil {
		return ItemSearchResult{}, err
	}

	// Fetch one extra row so the API can tell Android whether another page exists.
	items, err := s.store.SearchItems(ctx, StoreSearchParams{
		Query:  strings.TrimSpace(params.Query),
		Tag:    strings.TrimSpace(params.Tag),
		Limit:  limit + 1,
		Offset: offset,
	})
	if err != nil {
		return ItemSearchResult{}, err
	}

	var nextCursor *string
	if len(items) > limit {
		items = items[:limit]
		cursor := strconv.Itoa(offset + limit)
		nextCursor = &cursor
	}

	return ItemSearchResult{
		Items:      items,
		Limit:      limit,
		NextCursor: nextCursor,
	}, nil
}

func (s *Service) PlaybackItem(ctx context.Context, episodeID string) (PlaybackItem, error) {
	episodeID = strings.TrimSpace(episodeID)
	if episodeID == "" {
		return PlaybackItem{}, ErrMediaNotFound
	}
	return s.store.FindPlaybackItem(ctx, episodeID)
}

func (s *Service) EpisodeDetail(ctx context.Context, episodeID string) (EpisodeDetail, error) {
	episodeID = strings.TrimSpace(episodeID)
	if episodeID == "" {
		return EpisodeDetail{}, ErrMediaNotFound
	}
	return s.store.FindEpisodeDetail(ctx, episodeID)
}

func (s *Service) ValidatePlayableEpisode(ctx context.Context, episodeID string) (PlayableEpisode, error) {
	episodeID = strings.TrimSpace(episodeID)
	if episodeID == "" {
		return PlayableEpisode{}, ErrMediaNotFound
	}
	episode, err := s.store.ValidatePlayableEpisode(ctx, episodeID)
	if err != nil {
		return PlayableEpisode{}, err
	}
	if !episode.Playable || strings.TrimSpace(episode.ID) == "" {
		return PlayableEpisode{}, ErrMediaNotFound
	}
	return episode, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func parseCursor(cursor string) (int, error) {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, ErrInvalidPagination
	}
	return offset, nil
}
