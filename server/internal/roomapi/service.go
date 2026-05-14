package roomapi

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
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
	GetRoomDetail(ctx context.Context, roomCode string) (DetailResult, error)
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

type Service struct {
	store Store
}

// NewService wires room business APIs to persistent storage.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CreateRoom validates input, generates a shareable room code, and persists the room.
func (s *Service) CreateRoom(ctx context.Context, hostUserID string, mediaItemID string) (CreateRoomResult, error) {
	hostUserID = strings.TrimSpace(hostUserID)
	mediaItemID = strings.TrimSpace(mediaItemID)
	if hostUserID == "" || mediaItemID == "" {
		return CreateRoomResult{}, ErrInvalidInput
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
		return result, err
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
	return s.store.JoinRoomByCode(ctx, JoinRoomParams{
		RoomCode: roomCode,
		UserID:   userID,
	})
}

// DetailByCode returns the room business data needed to bootstrap the theater page.
func (s *Service) DetailByCode(ctx context.Context, roomCode string) (DetailResult, error) {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 {
		return DetailResult{}, ErrInvalidInput
	}
	return s.store.GetRoomDetail(ctx, roomCode)
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
