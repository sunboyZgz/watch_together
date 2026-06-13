package roomapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
	mediacatalog "watch_together/server/internal/media"
)

func TestRPCClientMatchesLocalRoomService(t *testing.T) {
	durationMs := int64(1458000)
	localStore := &fakeRPCRoomStore{
		mediaID:    "episode-1",
		durationMs: &durationMs,
		active:     true,
	}
	local := NewServiceWithMediaLookup(localStore, fakeRPCRoomMediaLookup{
		detail: mediacatalog.EpisodeDetail{
			ID:         "episode-1",
			Title:      "Episode 1",
			MediaURL:   "episode-1/hls/master.m3u8",
			DurationMs: &durationMs,
		},
	})
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", local)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})
	created, err := client.CreateRoom(context.Background(), "user-host", "episode-1")
	if err != nil {
		t.Fatalf("create room through rpc: %v", err)
	}
	if created.Room.RoomCode == "" || created.Room.HostUserID != "user-host" || created.Media.ID != "episode-1" {
		t.Fatalf("unexpected create result: %+v", created)
	}

	joined, err := client.JoinRoomByCode(context.Background(), created.Room.RoomCode, "user-viewer")
	if err != nil {
		t.Fatalf("join room through rpc: %v", err)
	}
	if joined.Member.UserID != "user-viewer" || joined.Room.RoomCode != created.Room.RoomCode {
		t.Fatalf("unexpected join result: %+v", joined)
	}

	detail, err := client.DetailByCode(context.Background(), created.Room.RoomCode)
	if err != nil {
		t.Fatalf("detail through rpc: %v", err)
	}
	if detail.Room.RoomCode != created.Room.RoomCode || len(detail.Members) != 2 {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	active, err := client.IsActiveMemberByCode(context.Background(), created.Room.RoomCode, "user-viewer")
	if err != nil {
		t.Fatalf("active member through rpc: %v", err)
	}
	if !active {
		t.Fatalf("expected viewer active")
	}

	bootstrap, err := client.RuntimeBootstrapByCode(context.Background(), created.Room.RoomCode)
	if err != nil {
		t.Fatalf("runtime bootstrap through rpc: %v", err)
	}
	if bootstrap.Room.RoomCode != created.Room.RoomCode || bootstrap.Media.ID != "episode-1" {
		t.Fatalf("unexpected bootstrap: %+v", bootstrap)
	}

	recoverable, err := client.ListRecoverableRoomCodes(context.Background(), 10)
	if err != nil {
		t.Fatalf("recoverable rooms through rpc: %v", err)
	}
	if len(recoverable) != 1 || recoverable[0] != created.Room.RoomCode {
		t.Fatalf("unexpected recoverable rooms: %+v", recoverable)
	}

	if err := client.LeaveRoomByCode(context.Background(), created.Room.RoomCode, "user-viewer"); err != nil {
		t.Fatalf("leave through rpc: %v", err)
	}
	if localStore.lastLeave.UserID != "user-viewer" {
		t.Fatalf("expected leave user-viewer, got %+v", localStore.lastLeave)
	}

	now := time.Now()
	if err := client.MarkRoomGracePeriod(context.Background(), created.Room.RoomCode, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("mark grace period through rpc: %v", err)
	}
	if err := client.MarkRoomActive(context.Background(), created.Room.RoomCode); err != nil {
		t.Fatalf("mark active through rpc: %v", err)
	}
	if count, err := client.MarkAllActiveRoomsGracePeriod(context.Background(), now, now.Add(time.Minute)); err != nil || count != 1 {
		t.Fatalf("mark all active through rpc count=%d err=%v", count, err)
	}
	expired, err := client.CleanupExpiredRoomCodes(context.Background(), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("cleanup expired through rpc: %v", err)
	}
	if len(expired) != 1 || expired[0] != created.Room.RoomCode {
		t.Fatalf("unexpected expired rooms: %+v", expired)
	}
	if err := client.DestroyRoom(context.Background(), created.Room.RoomCode); err != nil {
		t.Fatalf("destroy room through rpc: %v", err)
	}
}

func TestRPCClientMapsRoomErrors(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "", NewServiceWithMediaLookup(
		&fakeRPCRoomStore{err: ErrRoomNotFound},
		fakeRPCRoomMediaLookup{detail: mediacatalog.EpisodeDetail{ID: "episode-1"}},
	))
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
	if _, err := client.JoinRoomByCode(context.Background(), "ABC123", "user-a"); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}
	if _, err := client.DetailByCode(context.Background(), "ABC123"); !errors.Is(err, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound for detail, got %v", err)
	}

	mux = http.NewServeMux()
	RegisterInternalRPC(mux, "", "", NewServiceWithMediaLookup(
		&fakeRPCRoomStore{},
		fakeRPCRoomMediaLookup{err: mediacatalog.ErrMediaNotFound},
	))
	mediaServer := httptest.NewServer(mux)
	defer mediaServer.Close()
	mediaClient := NewRPCClient(mediaServer.URL, internalrpc.ClientConfig{Timeout: time.Second})
	if _, err := mediaClient.CreateRoom(context.Background(), "user-a", "missing"); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expected ErrMediaNotFound, got %v", err)
	}
}

func TestRPCClientRejectsInvalidRoomAuthToken(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", NewServiceWithMediaLookup(
		&fakeRPCRoomStore{},
		fakeRPCRoomMediaLookup{detail: mediacatalog.EpisodeDetail{ID: "episode-1"}},
	))
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "wrong",
	})
	_, err := client.CreateRoom(context.Background(), "user-a", "episode-1")
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated connect error, got %v", err)
	}
}

type fakeRPCRoomStore struct {
	mediaID      string
	durationMs   *int64
	active       bool
	err          error
	lastLeave    LeaveRoomParams
	lastRoomCode string
}

func (s *fakeRPCRoomStore) CreateRoom(_ context.Context, params CreateRoomParams) (CreateRoomResult, error) {
	if s.err != nil {
		return CreateRoomResult{}, s.err
	}
	s.lastRoomCode = params.RoomCode
	return CreateRoomResult{Room: s.room(params.RoomCode, params.HostUserID, params.MediaItemID)}, nil
}

func (s *fakeRPCRoomStore) JoinRoomByCode(_ context.Context, params JoinRoomParams) (JoinRoomResult, error) {
	if s.err != nil {
		return JoinRoomResult{}, s.err
	}
	return JoinRoomResult{
		Room:   s.room(params.RoomCode, "user-host", s.mediaIDOrDefault()),
		Member: Member{UserID: params.UserID, Role: "member"},
	}, nil
}

func (s *fakeRPCRoomStore) LeaveRoomByCode(_ context.Context, params LeaveRoomParams) error {
	s.lastLeave = params
	return s.err
}

func (s *fakeRPCRoomStore) GetRoomDetail(_ context.Context, roomCode string) (DetailResult, error) {
	if s.err != nil {
		return DetailResult{}, s.err
	}
	return DetailResult{
		Room: s.room(roomCode, "user-host", s.mediaIDOrDefault()),
		Members: []Member{
			{UserID: "user-host", Role: "host"},
			{UserID: "user-viewer", Role: "member"},
		},
	}, nil
}

func (s *fakeRPCRoomStore) GetRoomRuntimeBootstrap(_ context.Context, roomCode string) (RuntimeBootstrapResult, error) {
	if s.err != nil {
		return RuntimeBootstrapResult{}, s.err
	}
	return RuntimeBootstrapResult{Room: s.room(roomCode, "user-host", s.mediaIDOrDefault())}, nil
}

func (s *fakeRPCRoomStore) IsActiveMemberByCode(context.Context, string, string) (bool, error) {
	return s.active, s.err
}

func (s *fakeRPCRoomStore) ListRecoverableRoomCodes(context.Context, int) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []string{s.lastRoomCode}, nil
}

func (s *fakeRPCRoomStore) MarkRoomGracePeriod(context.Context, string, time.Time, time.Time) error {
	return s.err
}

func (s *fakeRPCRoomStore) MarkRoomActive(context.Context, string) error {
	return s.err
}

func (s *fakeRPCRoomStore) DestroyRoom(context.Context, string) error {
	return s.err
}

func (s *fakeRPCRoomStore) MarkAllActiveRoomsGracePeriod(context.Context, time.Time, time.Time) (int64, error) {
	return 1, s.err
}

func (s *fakeRPCRoomStore) CleanupExpiredRoomCodes(context.Context, time.Time) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []string{s.lastRoomCode}, nil
}

func (s *fakeRPCRoomStore) room(roomCode string, hostUserID string, mediaItemID string) Room {
	return Room{
		ID:          "room-id",
		RoomCode:    roomCode,
		HostUserID:  hostUserID,
		MediaItemID: mediaItemID,
		Status:      "active",
	}
}

func (s *fakeRPCRoomStore) mediaIDOrDefault() string {
	if s.mediaID != "" {
		return s.mediaID
	}
	return "episode-1"
}

type fakeRPCRoomMediaLookup struct {
	detail mediacatalog.EpisodeDetail
	err    error
}

func (l fakeRPCRoomMediaLookup) EpisodeDetail(context.Context, string) (mediacatalog.EpisodeDetail, error) {
	if l.err != nil {
		return mediacatalog.EpisodeDetail{}, l.err
	}
	return l.detail, nil
}
