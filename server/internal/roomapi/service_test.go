package roomapi

import (
	"context"
	"errors"
	"testing"
	"time"

	mediacatalog "watch_together/server/internal/media"
)

func TestServiceCreateRoomUsesMediaLookupBoundary(t *testing.T) {
	lookup := &fakeMediaLookup{
		detail: mediacatalog.EpisodeDetail{
			ID:       "episode-canonical",
			Title:    "Episode 1",
			MediaURL: "episode-1/hls/master.m3u8",
		},
	}
	store := &fakeRoomServiceStore{
		createResult: CreateRoomResult{
			Room: Room{
				ID:         "room-id",
				RoomCode:   "A7K2M9",
				HostUserID: "user-a",
				Status:     "active",
			},
		},
	}
	service := NewServiceWithMediaLookup(store, lookup)

	result, err := service.CreateRoom(context.Background(), "user-a", "episode-alias")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	if lookup.lastEpisodeID != "episode-alias" {
		t.Fatalf("expected media lookup for episode-alias, got %q", lookup.lastEpisodeID)
	}
	if store.lastCreate.MediaItemID != "episode-canonical" {
		t.Fatalf("expected canonical media id in room store, got %q", store.lastCreate.MediaItemID)
	}
	if result.Media.ID != "episode-canonical" || result.Media.Title != "Episode 1" {
		t.Fatalf("expected media from lookup, got %+v", result.Media)
	}
}

func TestServiceDetailByCodeUsesMediaLookupBoundary(t *testing.T) {
	lookup := &fakeMediaLookup{
		detail: mediacatalog.EpisodeDetail{
			ID:       "episode-1",
			Title:    "Episode 1",
			MediaURL: "episode-1/hls/master.m3u8",
		},
	}
	store := &fakeRoomServiceStore{
		detailResult: DetailResult{
			Room: Room{
				ID:          "room-id",
				RoomCode:    "A7K2M9",
				MediaItemID: "episode-1",
				Status:      "active",
			},
			Members: []Member{{UserID: "user-a", Role: "host"}},
		},
	}
	service := NewServiceWithMediaLookup(store, lookup)

	result, err := service.DetailByCode(context.Background(), "a7k2m9")
	if err != nil {
		t.Fatalf("detail room: %v", err)
	}
	if lookup.lastEpisodeID != "episode-1" {
		t.Fatalf("expected media lookup for episode-1, got %q", lookup.lastEpisodeID)
	}
	if result.Media.Title != "Episode 1" {
		t.Fatalf("expected media title from lookup, got %+v", result.Media)
	}
}

func TestServiceMapsMediaLookupNotFound(t *testing.T) {
	service := NewServiceWithMediaLookup(
		&fakeRoomServiceStore{},
		&fakeMediaLookup{err: mediacatalog.ErrMediaNotFound},
	)

	_, err := service.CreateRoom(context.Background(), "user-a", "missing")
	if !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
}

func TestServiceDoesNotCreateRoomWhenMediaLookupFails(t *testing.T) {
	lookupErr := errors.New("media rpc unavailable")
	store := &fakeRoomServiceStore{}
	service := NewServiceWithMediaLookup(store, &fakeMediaLookup{err: lookupErr})

	_, err := service.CreateRoom(context.Background(), "user-a", "episode-1")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected media lookup error, got %v", err)
	}
	if store.lastCreate.MediaItemID != "" {
		t.Fatalf("expected room store not to be called, got %+v", store.lastCreate)
	}
}

type fakeMediaLookup struct {
	detail        mediacatalog.EpisodeDetail
	err           error
	lastEpisodeID string
}

func (l *fakeMediaLookup) EpisodeDetail(_ context.Context, episodeID string) (mediacatalog.EpisodeDetail, error) {
	l.lastEpisodeID = episodeID
	if l.err != nil {
		return mediacatalog.EpisodeDetail{}, l.err
	}
	return l.detail, nil
}

type fakeRoomServiceStore struct {
	createResult       CreateRoomResult
	joinResult         JoinRoomResult
	detailResult       DetailResult
	lastCreate         CreateRoomParams
	lastJoin           JoinRoomParams
	lastDetailRoomCode string
	err                error
}

func (s *fakeRoomServiceStore) CreateRoom(_ context.Context, params CreateRoomParams) (CreateRoomResult, error) {
	s.lastCreate = params
	if s.err != nil {
		return CreateRoomResult{}, s.err
	}
	return s.createResult, nil
}

func (s *fakeRoomServiceStore) JoinRoomByCode(_ context.Context, params JoinRoomParams) (JoinRoomResult, error) {
	s.lastJoin = params
	if s.err != nil {
		return JoinRoomResult{}, s.err
	}
	return s.joinResult, nil
}

func (s *fakeRoomServiceStore) LeaveRoomByCode(context.Context, LeaveRoomParams) error {
	return s.err
}

func (s *fakeRoomServiceStore) GetRoomDetail(_ context.Context, roomCode string) (DetailResult, error) {
	s.lastDetailRoomCode = roomCode
	if s.err != nil {
		return DetailResult{}, s.err
	}
	return s.detailResult, nil
}

func (s *fakeRoomServiceStore) IsActiveMemberByCode(context.Context, string, string) (bool, error) {
	return false, s.err
}

func (s *fakeRoomServiceStore) GetRoomRuntimeBootstrap(_ context.Context, roomCode string) (RuntimeBootstrapResult, error) {
	if s.err != nil {
		return RuntimeBootstrapResult{}, s.err
	}
	return RuntimeBootstrapResult{Room: s.detailResult.Room}, nil
}

func (s *fakeRoomServiceStore) ListRecoverableRoomCodes(context.Context, int) ([]string, error) {
	return []string{"A7K2M9"}, s.err
}

func (s *fakeRoomServiceStore) MarkRoomGracePeriod(context.Context, string, time.Time, time.Time) error {
	return s.err
}

func (s *fakeRoomServiceStore) MarkRoomActive(context.Context, string) error {
	return s.err
}

func (s *fakeRoomServiceStore) DestroyRoom(context.Context, string) error {
	return s.err
}

func (s *fakeRoomServiceStore) MarkAllActiveRoomsGracePeriod(context.Context, time.Time, time.Time) (int64, error) {
	return 1, s.err
}

func (s *fakeRoomServiceStore) CleanupExpiredRoomCodes(context.Context, time.Time) ([]string, error) {
	return []string{"A7K2M9"}, s.err
}
