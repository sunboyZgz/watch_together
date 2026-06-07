package progress

import (
	"context"
	"errors"
	"strings"
	"time"

	mediacatalog "watch_together/server/internal/media"
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

type MediaValidator interface {
	ValidatePlayableEpisode(ctx context.Context, episodeID string) (mediacatalog.PlayableEpisode, error)
}

type Service struct {
	store          Store
	mediaValidator MediaValidator
}

// NewService wires media progress writes to persistent storage.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// NewServiceWithMediaValidator validates media ownership through the media context boundary.
func NewServiceWithMediaValidator(store Store, mediaValidator MediaValidator) *Service {
	return &Service{store: store, mediaValidator: mediaValidator}
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
	if s != nil && s.mediaValidator != nil {
		episode, err := s.mediaValidator.ValidatePlayableEpisode(ctx, params.MediaItemID)
		if err != nil {
			if errors.Is(err, mediacatalog.ErrMediaNotFound) {
				return Summary{}, ErrMediaNotFound
			}
			return Summary{}, err
		}
		if !episode.Playable || strings.TrimSpace(episode.ID) == "" {
			return Summary{}, ErrMediaNotFound
		}
		params.MediaItemID = episode.ID
	}
	return s.store.UpdateMediaProgress(ctx, params)
}
