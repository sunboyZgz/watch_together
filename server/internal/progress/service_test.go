package progress

import (
	"context"
	"errors"
	"testing"
	"time"

	mediacatalog "watch_together/server/internal/media"
)

func TestServiceUpdateValidatesPlayableEpisodeThroughMediaBoundary(t *testing.T) {
	validator := &fakeMediaValidator{
		episode: mediacatalog.PlayableEpisode{ID: "episode-canonical", Playable: true},
	}
	store := &fakeProgressServiceStore{
		summary: Summary{
			MediaItemID:         "episode-canonical",
			LastPositionSeconds: 10,
			DurationSeconds:     100,
			LastWatchedAt:       time.Date(2026, 6, 7, 1, 2, 3, 0, time.UTC),
		},
	}
	service := NewServiceWithMediaValidator(store, validator)

	_, err := service.Update(context.Background(), UpdateParams{
		UserID:              "user-a",
		MediaItemID:         "episode-alias",
		LastPositionSeconds: 10,
		DurationSeconds:     100,
	})
	if err != nil {
		t.Fatalf("update progress: %v", err)
	}

	if validator.lastEpisodeID != "episode-alias" {
		t.Fatalf("expected media validator episode-alias, got %q", validator.lastEpisodeID)
	}
	if store.lastParams.MediaItemID != "episode-canonical" {
		t.Fatalf("expected canonical episode id in progress store, got %q", store.lastParams.MediaItemID)
	}
}

func TestServiceUpdateMapsMediaNotFound(t *testing.T) {
	service := NewServiceWithMediaValidator(
		&fakeProgressServiceStore{},
		&fakeMediaValidator{err: mediacatalog.ErrMediaNotFound},
	)

	_, err := service.Update(context.Background(), UpdateParams{
		UserID:              "user-a",
		MediaItemID:         "missing",
		LastPositionSeconds: 10,
		DurationSeconds:     100,
	})
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
}

func TestServiceUpdateDoesNotWriteWhenMediaValidatorFails(t *testing.T) {
	validatorErr := errors.New("media rpc unavailable")
	store := &fakeProgressServiceStore{}
	service := NewServiceWithMediaValidator(store, &fakeMediaValidator{err: validatorErr})

	_, err := service.Update(context.Background(), UpdateParams{
		UserID:              "user-a",
		MediaItemID:         "episode-1",
		LastPositionSeconds: 10,
		DurationSeconds:     100,
	})
	if !errors.Is(err, validatorErr) {
		t.Fatalf("expected validator error, got %v", err)
	}
	if store.lastParams.MediaItemID != "" {
		t.Fatalf("expected progress store not to be called, got %+v", store.lastParams)
	}
}

type fakeMediaValidator struct {
	episode       mediacatalog.PlayableEpisode
	err           error
	lastEpisodeID string
}

func (v *fakeMediaValidator) ValidatePlayableEpisode(_ context.Context, episodeID string) (mediacatalog.PlayableEpisode, error) {
	v.lastEpisodeID = episodeID
	if v.err != nil {
		return mediacatalog.PlayableEpisode{}, v.err
	}
	return v.episode, nil
}

type fakeProgressServiceStore struct {
	summary    Summary
	lastParams UpdateParams
	err        error
}

func (s *fakeProgressServiceStore) UpdateMediaProgress(_ context.Context, params UpdateParams) (Summary, error) {
	s.lastParams = params
	if s.err != nil {
		return Summary{}, s.err
	}
	return s.summary, nil
}

func (s *fakeProgressServiceStore) GetUserProgress(context.Context, string, string) (Summary, bool, error) {
	if s.err != nil {
		return Summary{}, false, s.err
	}
	return s.summary, true, nil
}

func (s *fakeProgressServiceStore) BatchGetUserProgress(context.Context, string, []string) ([]Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Summary{s.summary}, nil
}

func (s *fakeProgressServiceStore) ListRecentUserProgress(context.Context, RecentParams) ([]Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []Summary{s.summary}, nil
}
