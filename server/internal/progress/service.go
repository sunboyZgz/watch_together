package progress

import (
	"context"
	"errors"
	"strings"
	"time"

	"watch_together/server/internal/auth"
	mediacatalog "watch_together/server/internal/media"
)

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrMediaNotFound    = errors.New("media not found")
	ErrProgressNotFound = errors.New("progress not found")
	ErrUserNotFound     = errors.New("user not found")
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

type RecentParams struct {
	UserID         string
	Limit          int
	IncompleteOnly bool
}

type Store interface {
	UpdateMediaProgress(ctx context.Context, params UpdateParams) (Summary, error)
	GetUserProgress(ctx context.Context, userID string, mediaItemID string) (Summary, bool, error)
	BatchGetUserProgress(ctx context.Context, userID string, mediaItemIDs []string) ([]Summary, error)
	ListRecentUserProgress(ctx context.Context, params RecentParams) ([]Summary, error)
}

type BusinessService interface {
	Update(ctx context.Context, params UpdateParams) (Summary, error)
	GetUserProgress(ctx context.Context, userID string, mediaItemID string) (Summary, bool, error)
	BatchGetUserProgress(ctx context.Context, userID string, mediaItemIDs []string) ([]Summary, error)
	ListRecentUserProgress(ctx context.Context, params RecentParams) ([]Summary, error)
}

type MediaValidator interface {
	ValidatePlayableEpisode(ctx context.Context, episodeID string) (mediacatalog.PlayableEpisode, error)
}

type UserProfileProvider interface {
	GetUserProfile(ctx context.Context, userID string) (auth.User, error)
}

type Service struct {
	store          Store
	userProfiles   UserProfileProvider
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

// NewServiceWithBoundaries validates users and media through their owning contexts.
func NewServiceWithBoundaries(
	store Store,
	userProfiles UserProfileProvider,
	mediaValidator MediaValidator,
) *Service {
	return &Service{store: store, userProfiles: userProfiles, mediaValidator: mediaValidator}
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
	if err := s.validateUser(ctx, params.UserID); err != nil {
		return Summary{}, err
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

func (s *Service) GetUserProgress(ctx context.Context, userID string, mediaItemID string) (Summary, bool, error) {
	userID = strings.TrimSpace(userID)
	mediaItemID = strings.TrimSpace(mediaItemID)
	if userID == "" || mediaItemID == "" {
		return Summary{}, false, ErrInvalidInput
	}
	if err := s.validateUser(ctx, userID); err != nil {
		return Summary{}, false, err
	}
	if s == nil || s.store == nil {
		return Summary{}, false, ErrProgressNotFound
	}
	return s.store.GetUserProgress(ctx, userID, mediaItemID)
}

func (s *Service) BatchGetUserProgress(ctx context.Context, userID string, mediaItemIDs []string) ([]Summary, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalidInput
	}
	if err := s.validateUser(ctx, userID); err != nil {
		return nil, err
	}
	cleaned := cleanMediaItemIDs(mediaItemIDs)
	if len(cleaned) == 0 {
		return nil, nil
	}
	if s == nil || s.store == nil {
		return nil, ErrProgressNotFound
	}
	return s.store.BatchGetUserProgress(ctx, userID, cleaned)
}

func (s *Service) ListRecentUserProgress(ctx context.Context, params RecentParams) ([]Summary, error) {
	params.UserID = strings.TrimSpace(params.UserID)
	if params.UserID == "" {
		return nil, ErrInvalidInput
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 50 {
		params.Limit = 50
	}
	if err := s.validateUser(ctx, params.UserID); err != nil {
		return nil, err
	}
	if s == nil || s.store == nil {
		return nil, ErrProgressNotFound
	}
	return s.store.ListRecentUserProgress(ctx, params)
}

func (s *Service) validateUser(ctx context.Context, userID string) error {
	if s == nil || s.userProfiles == nil {
		return nil
	}
	if _, err := s.userProfiles.GetUserProfile(ctx, userID); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrInvalidInput) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

func cleanMediaItemIDs(mediaItemIDs []string) []string {
	cleaned := make([]string, 0, len(mediaItemIDs))
	seen := make(map[string]struct{}, len(mediaItemIDs))
	for _, mediaItemID := range mediaItemIDs {
		mediaItemID = strings.TrimSpace(mediaItemID)
		if mediaItemID == "" {
			continue
		}
		if _, ok := seen[mediaItemID]; ok {
			continue
		}
		seen[mediaItemID] = struct{}{}
		cleaned = append(cleaned, mediaItemID)
	}
	return cleaned
}
