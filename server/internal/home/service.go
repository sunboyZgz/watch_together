package home

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrInvalidUserID = errors.New("invalid user id")
	ErrUserNotFound  = errors.New("user not found")
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

type Service struct {
	store SummaryStore
}

// NewService wires home-page summary reads to a persistent store.
func NewService(store SummaryStore) *Service {
	return &Service{store: store}
}

// Summary loads the user profile plus recent watching progress for the home page.
func (s *Service) Summary(ctx context.Context, userID string) (Summary, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Summary{}, ErrInvalidUserID
	}
	return s.store.GetHomeSummary(ctx, userID)
}
