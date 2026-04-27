package progress

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrMediaNotFound = errors.New("media not found")
	ErrUserNotFound  = errors.New("user not found")
)

type UpdateParams struct {
	UserID              string
	MediaItemID         string
	LastPositionSeconds int
	DurationSeconds     int
	Completed           bool
	CompletionSource    *string
}

type Summary struct {
	MediaItemID         string
	LastPositionSeconds int
	DurationSeconds     int
	Completed           bool
	LastWatchedAt       time.Time
}

type Store interface {
	UpdateMediaProgress(ctx context.Context, params UpdateParams) (Summary, error)
}

type Service struct {
	store Store
}

// NewService wires media progress writes to persistent storage.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Update validates and persists the current user's low-frequency media progress.
func (s *Service) Update(ctx context.Context, params UpdateParams) (Summary, error) {
	params.UserID = strings.TrimSpace(params.UserID)
	params.MediaItemID = strings.TrimSpace(params.MediaItemID)
	if params.UserID == "" ||
		params.MediaItemID == "" ||
		params.DurationSeconds <= 0 ||
		params.LastPositionSeconds < 0 ||
		params.LastPositionSeconds > params.DurationSeconds {
		return Summary{}, ErrInvalidInput
	}
	if params.CompletionSource != nil {
		source := strings.TrimSpace(*params.CompletionSource)
		if source == "" {
			params.CompletionSource = nil
		} else if source != "ended" && source != "manual_mark" && source != "threshold_auto" {
			return Summary{}, ErrInvalidInput
		} else {
			params.CompletionSource = &source
		}
	}
	return s.store.UpdateMediaProgress(ctx, params)
}
