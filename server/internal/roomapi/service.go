package roomapi

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"watch_together/server/internal/auth"
	mediacatalog "watch_together/server/internal/media"
)

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrMediaNotFound       = errors.New("media not found")
	ErrRoomCodeExists      = errors.New("room code already exists")
	ErrRoomNotFound        = errors.New("room not found")
	ErrUnableToCreate      = errors.New("unable to create room")
	ErrUserNotFound        = errors.New("user not found")
	ErrIdentityUnavailable = errors.New("identity service is unavailable")
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

type RuntimeBootstrapResult struct {
	Room  Room
	Media Media
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
	GetRoomRuntimeBootstrap(ctx context.Context, roomCode string) (RuntimeBootstrapResult, error)
	ListRecoverableRoomCodes(ctx context.Context, limit int) ([]string, error)
	MarkRoomGracePeriod(ctx context.Context, roomCode string, lastEmptyAt time.Time, destroyAfter time.Time) error
	MarkRoomActive(ctx context.Context, roomCode string) error
	DestroyRoom(ctx context.Context, roomCode string) error
	MarkAllActiveRoomsGracePeriod(ctx context.Context, lastEmptyAt time.Time, destroyAfter time.Time) (int64, error)
	CleanupExpiredRoomCodes(ctx context.Context, now time.Time) ([]string, error)
}

type BusinessService interface {
	CreateRoom(ctx context.Context, hostUserID string, mediaItemID string) (CreateRoomResult, error)
	JoinRoomByCode(ctx context.Context, roomCode string, userID string) (JoinRoomResult, error)
	LeaveRoomByCode(ctx context.Context, roomCode string, userID string) error
	DetailByCode(ctx context.Context, roomCode string) (DetailResult, error)
	IsActiveMemberByCode(ctx context.Context, roomCode string, userID string) (bool, error)
	RuntimeBootstrapByCode(ctx context.Context, roomCode string) (RuntimeBootstrapResult, error)
	ListRecoverableRoomCodes(ctx context.Context, limit int) ([]string, error)
	MarkRoomGracePeriod(ctx context.Context, roomCode string, lastEmptyAt time.Time, destroyAfter time.Time) error
	MarkRoomActive(ctx context.Context, roomCode string) error
	DestroyRoom(ctx context.Context, roomCode string) error
	MarkAllActiveRoomsGracePeriod(ctx context.Context, lastEmptyAt time.Time, destroyAfter time.Time) (int64, error)
	CleanupExpiredRoomCodes(ctx context.Context, now time.Time) ([]string, error)
}

type MediaDetailLookup interface {
	EpisodeDetail(ctx context.Context, episodeID string) (mediacatalog.EpisodeDetail, error)
}

type UserProfileLookup interface {
	GetUserProfile(ctx context.Context, userID string) (auth.User, error)
	BatchGetUserProfiles(ctx context.Context, userIDs []string) ([]auth.User, error)
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
	userLookup  UserProfileLookup
}

// NewService wires room business APIs to persistent storage.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// NewServiceWithMediaLookup wires room business APIs through the media context boundary.
func NewServiceWithMediaLookup(store Store, mediaLookup MediaDetailLookup) *Service {
	return &Service{store: store, mediaLookup: mediaLookup}
}

// NewServiceWithBoundaries wires room business APIs through identity and media context boundaries.
func NewServiceWithBoundaries(store Store, mediaLookup MediaDetailLookup, userLookup UserProfileLookup) *Service {
	return &Service{store: store, mediaLookup: mediaLookup, userLookup: userLookup}
}

// CreateRoom validates input, generates a shareable room code, and persists the room.
func (s *Service) CreateRoom(ctx context.Context, hostUserID string, mediaItemID string) (CreateRoomResult, error) {
	hostUserID = strings.TrimSpace(hostUserID)
	mediaItemID = strings.TrimSpace(mediaItemID)
	if hostUserID == "" || mediaItemID == "" {
		return CreateRoomResult{}, ErrInvalidInput
	}
	if err := s.requireUser(ctx, hostUserID); err != nil {
		return CreateRoomResult{}, err
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
	if err := s.requireUser(ctx, userID); err != nil {
		return JoinRoomResult{}, err
	}
	result, err := s.store.JoinRoomByCode(ctx, JoinRoomParams{
		RoomCode: roomCode,
		UserID:   userID,
	})
	if err != nil {
		return JoinRoomResult{}, err
	}
	result.Member = s.enrichMember(ctx, result.Member)
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
	members, err := s.enrichMembers(ctx, result.Members)
	if err != nil {
		return DetailResult{}, err
	}
	result.Members = members
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

func (s *Service) RuntimeBootstrapByCode(ctx context.Context, roomCode string) (RuntimeBootstrapResult, error) {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 {
		return RuntimeBootstrapResult{}, ErrInvalidInput
	}
	result, err := s.store.GetRoomRuntimeBootstrap(ctx, roomCode)
	if err != nil {
		return RuntimeBootstrapResult{}, err
	}
	mediaItem, err := s.lookupMedia(ctx, result.Room.MediaItemID)
	if err != nil {
		return RuntimeBootstrapResult{}, err
	}
	if mediaItem.ID != "" {
		result.Media = mediaItem
	}
	return result, nil
}

func (s *Service) ListRecoverableRoomCodes(ctx context.Context, limit int) ([]string, error) {
	return s.store.ListRecoverableRoomCodes(ctx, limit)
}

func (s *Service) MarkRoomGracePeriod(ctx context.Context, roomCode string, lastEmptyAt time.Time, destroyAfter time.Time) error {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 || lastEmptyAt.IsZero() || destroyAfter.IsZero() {
		return ErrInvalidInput
	}
	return s.store.MarkRoomGracePeriod(ctx, roomCode, lastEmptyAt, destroyAfter)
}

func (s *Service) MarkRoomActive(ctx context.Context, roomCode string) error {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 {
		return ErrInvalidInput
	}
	return s.store.MarkRoomActive(ctx, roomCode)
}

func (s *Service) DestroyRoom(ctx context.Context, roomCode string) error {
	roomCode = strings.ToUpper(strings.TrimSpace(roomCode))
	if len(roomCode) != 6 {
		return ErrInvalidInput
	}
	return s.store.DestroyRoom(ctx, roomCode)
}

func (s *Service) MarkAllActiveRoomsGracePeriod(ctx context.Context, lastEmptyAt time.Time, destroyAfter time.Time) (int64, error) {
	if lastEmptyAt.IsZero() || destroyAfter.IsZero() {
		return 0, ErrInvalidInput
	}
	return s.store.MarkAllActiveRoomsGracePeriod(ctx, lastEmptyAt, destroyAfter)
}

func (s *Service) CleanupExpiredRoomCodes(ctx context.Context, now time.Time) ([]string, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return s.store.CleanupExpiredRoomCodes(ctx, now)
}

func (s *Service) requireUser(ctx context.Context, userID string) error {
	if s == nil || s.userLookup == nil {
		return nil
	}
	if _, err := s.userLookup.GetUserProfile(ctx, userID); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) || errors.Is(err, auth.ErrInvalidInput) {
			return ErrUserNotFound
		}
		return ErrIdentityUnavailable
	}
	return nil
}

func (s *Service) enrichMember(ctx context.Context, member Member) Member {
	members, err := s.enrichMembers(ctx, []Member{member})
	if err != nil {
		return member
	}
	if len(members) == 0 {
		return member
	}
	return members[0]
}

func (s *Service) enrichMembers(ctx context.Context, members []Member) ([]Member, error) {
	if s == nil || s.userLookup == nil || len(members) == 0 {
		return members, nil
	}
	userIDs := make([]string, 0, len(members))
	for _, member := range members {
		userIDs = append(userIDs, member.UserID)
	}
	users, err := s.userLookup.BatchGetUserProfiles(ctx, userIDs)
	if err != nil {
		return nil, ErrIdentityUnavailable
	}
	byID := make(map[string]auth.User, len(users))
	for _, user := range users {
		byID[user.ID] = user
	}
	enriched := make([]Member, 0, len(members))
	for _, member := range members {
		if user, ok := byID[member.UserID]; ok {
			member.Nickname = user.Nickname
			member.AvatarSeed = user.AvatarSeed
			member.AvatarURL = user.AvatarURL
		}
		enriched = append(enriched, member)
	}
	return enriched, nil
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
