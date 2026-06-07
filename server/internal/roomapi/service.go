package roomapi

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"

	mediacatalog "watch_together/server/internal/media"
)

var (
	ErrInvalidInput   = errors.New("invalid input")
	ErrMediaNotFound  = errors.New("media not found")
	ErrRoomCodeExists = errors.New("room code already exists")
	ErrRoomNotFound   = errors.New("room not found")
	ErrUnableToCreate = errors.New("unable to create room")
	ErrUserNotFound   = errors.New("user not found")
)

type Room struct {
	ID          string
	RoomCode    string
	HostUserID  string
	MediaItemID string
	Status      string
}

type Media struct {
	ID           string
	Title        string
	Subtitle     *string
	MediaURL     string
	DurationMs   *int64
	SeasonLabel  *string
	EpisodeLabel *string
}

type Member struct {
	UserID     string
	Nickname   string
	AvatarSeed string
	AvatarURL  *string
	Role       string
}

type CreateRoomResult struct {
	Room  Room
	Media Media
}

type JoinRoomResult struct {
	Room   Room
	Media  Media
	Member Member
}

type DetailResult struct {
	Room    Room
	Media   Media
	Members []Member
}

type Store interface {
	CreateRoom(ctx context.Context, params CreateRoomParams) (CreateRoomResult, error)
	JoinRoomByCode(ctx context.Context, params JoinRoomParams) (JoinRoomResult, error)
	LeaveRoomByCode(ctx context.Context, params LeaveRoomParams) error
	GetRoomDetail(ctx context.Context, roomCode string) (DetailResult, error)
	IsActiveMemberByCode(ctx context.Context, roomCode string, userID string) (bool, error)
}

type MediaDetailLookup interface {
	EpisodeDetail(ctx context.Context, episodeID string) (mediacatalog.EpisodeDetail, error)
}

type CreateRoomParams struct {
	RoomCode    string
	HostUserID  string
	MediaItemID string
}

type JoinRoomParams struct {
	RoomCode string
	UserID   string
}

type LeaveRoomParams struct {
	RoomCode string
	UserID   string
}

type Service struct {
	store       Store
	mediaLookup MediaDetailLookup
}

// NewService wires room business APIs to persistent storage.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// NewServiceWithMediaLookup wires room business APIs through the media context boundary.
func NewServiceWithMediaLookup(store Store, mediaLookup MediaDetailLookup) *Service {
	return &Service{store: store, mediaLookup: mediaLookup}
}

// CreateRoom validates input, generates a shareable room code, and persists the room.
func (s *Service) CreateRoom(ctx context.Context, hostUserID string, mediaItemID string) (CreateRoomResult, error) {
	hostUserID = strings.TrimSpace(hostUserID)
	mediaItemID = strings.TrimSpace(mediaItemID)
	if hostUserID == "" || mediaItemID == "" {
		return CreateRoomResult{}, ErrInvalidInput
	}

	mediaItem, err := s.lookupMedia(ctx, mediaItemID)
	if err != nil {
		return CreateRoomResult{}, err
	}
	if mediaItem.ID != "" {
		mediaItemID = mediaItem.ID
	}

	for range 10 {
		roomCode, err := generateRoomCode(6)
		if err != nil {
			return CreateRoomResult{}, err
		}
		result, err := s.store.CreateRoom(ctx, CreateRoomParams{
			RoomCode:    roomCode,
			HostUserID:  hostUserID,
			MediaItemID: mediaItemID,
		})
		if errors.Is(err, ErrRoomCodeExists) {
			continue
		}
		if err != nil {
			return CreateRoomResult{}, err
		}
		return s.withMedia(result, mediaItem)
	}

	return CreateRoomResult{}, ErrUnableToCreate
}

// JoinRoomByCode validates input and creates or restores the user's active membership.
func (s *Service) JoinRoomByCode(ctx context.Context, roomCode string, userID string) (JoinRoomResult, error) {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	userID = strings.TrimSpace(userID)
	if len(roomCode) != 6 || userID == "" {
		return JoinRoomResult{}, ErrInvalidInput
	}
	result, err := s.store.JoinRoomByCode(ctx, JoinRoomParams{
		RoomCode: roomCode,
		UserID:   userID,
	})
	if err != nil {
		return JoinRoomResult{}, err
	}
	mediaItem, err := s.lookupMedia(ctx, result.Room.MediaItemID)
	if err != nil {
		return JoinRoomResult{}, err
	}
	if mediaItem.ID == "" {
		mediaItem = result.Media
	}
	result.Media = mediaItem
	return result, nil
}

// LeaveRoomByCode marks the user's business membership inactive for an intentional leave.
func (s *Service) LeaveRoomByCode(ctx context.Context, roomCode string, userID string) error {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	userID = strings.TrimSpace(userID)
	if len(roomCode) != 6 || userID == "" {
		return ErrInvalidInput
	}
	return s.store.LeaveRoomByCode(ctx, LeaveRoomParams{
		RoomCode: roomCode,
		UserID:   userID,
	})
}

// IsActiveMemberByCode reports whether a user has an active business membership.
func (s *Service) IsActiveMemberByCode(ctx context.Context, roomCode string, userID string) (bool, error) {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	userID = strings.TrimSpace(userID)
	if len(roomCode) != 6 || userID == "" {
		return false, ErrInvalidInput
	}
	return s.store.IsActiveMemberByCode(ctx, roomCode, userID)
}

// DetailByCode returns the room business data needed to bootstrap the theater page.
func (s *Service) DetailByCode(ctx context.Context, roomCode string) (DetailResult, error) {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 {
		return DetailResult{}, ErrInvalidInput
	}
	result, err := s.store.GetRoomDetail(ctx, roomCode)
	if err != nil {
		return DetailResult{}, err
	}
	mediaItem, err := s.lookupMedia(ctx, result.Room.MediaItemID)
	if err != nil {
		return DetailResult{}, err
	}
	if mediaItem.ID == "" {
		mediaItem = result.Media
	}
	result.Media = mediaItem
	return result, nil
}

func (s *Service) lookupMedia(ctx context.Context, episodeID string) (Media, error) {
	if s == nil || s.mediaLookup == nil {
		return Media{}, nil
	}
	detail, err := s.mediaLookup.EpisodeDetail(ctx, episodeID)
	if err != nil {
		if errors.Is(err, mediacatalog.ErrMediaNotFound) {
			return Media{}, ErrMediaNotFound
		}
		return Media{}, err
	}
	return Media{
		ID:           detail.ID,
		Title:        detail.Title,
		Subtitle:     detail.Subtitle,
		MediaURL:     detail.MediaURL,
		DurationMs:   detail.DurationMs,
		SeasonLabel:  detail.SeasonLabel,
		EpisodeLabel: detail.EpisodeLabel,
	}, nil
}

func (s *Service) withMedia(result CreateRoomResult, mediaItem Media) (CreateRoomResult, error) {
	if mediaItem.ID == "" {
		return result, nil
	}
	if result.Room.MediaItemID == "" {
		result.Room.MediaItemID = mediaItem.ID
	}
	result.Media = mediaItem
	return result, nil
}

func generateRoomCode(length int) (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = chars[int(bytes[i])%len(chars)]
	}
	return string(bytes), nil
}
