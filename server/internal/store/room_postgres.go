package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"watch_together/server/internal/roomapi"
)

type PostgresRoomStore struct {
	db *sql.DB
}

// NewPostgresRoomStore creates the PostgreSQL-backed repository for room business APIs.
func NewPostgresRoomStore(db *sql.DB) *PostgresRoomStore {
	return &PostgresRoomStore{db: db}
}

// CreateRoom inserts room business data and the initial host membership in one transaction.
func (s *PostgresRoomStore) CreateRoom(ctx context.Context, params roomapi.CreateRoomParams) (roomapi.CreateRoomResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return roomapi.CreateRoomResult{}, fmt.Errorf("begin create room: %w", err)
	}
	defer rollbackTx(tx)

	if exists, err := rowExists(ctx, tx, `SELECT 1 FROM users WHERE id = $1`, params.HostUserID); err != nil {
		return roomapi.CreateRoomResult{}, fmt.Errorf("check host user: %w", err)
	} else if !exists {
		return roomapi.CreateRoomResult{}, roomapi.ErrUserNotFound
	}

	mediaItem, err := findRoomMedia(ctx, tx, params.MediaItemID)
	if err != nil {
		return roomapi.CreateRoomResult{}, err
	}

	const insertRoom = `
		INSERT INTO rooms (room_code, host_user_id, media_episode_id, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id::text, room_code, host_user_id::text, media_episode_id::text, status
	`
	var room roomapi.Room
	if err := tx.QueryRowContext(
		ctx,
		insertRoom,
		params.RoomCode,
		params.HostUserID,
		mediaItem.ID,
	).Scan(
		&room.ID,
		&room.RoomCode,
		&room.HostUserID,
		&room.MediaItemID,
		&room.Status,
	); err != nil {
		if isUniqueViolation(err) {
			return roomapi.CreateRoomResult{}, roomapi.ErrRoomCodeExists
		}
		return roomapi.CreateRoomResult{}, fmt.Errorf("insert room: %w", err)
	}

	if err := insertRoomMember(ctx, tx, room.ID, params.HostUserID, "host"); err != nil {
		return roomapi.CreateRoomResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return roomapi.CreateRoomResult{}, fmt.Errorf("commit create room: %w", err)
	}
	return roomapi.CreateRoomResult{
		Room:  room,
		Media: mediaItem,
	}, nil
}

// JoinRoomByCode creates or restores an active member row for a shareable room code.
func (s *PostgresRoomStore) JoinRoomByCode(ctx context.Context, params roomapi.JoinRoomParams) (roomapi.JoinRoomResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return roomapi.JoinRoomResult{}, fmt.Errorf("begin join room: %w", err)
	}
	defer rollbackTx(tx)

	if exists, err := rowExists(ctx, tx, `SELECT 1 FROM users WHERE id = $1`, params.UserID); err != nil {
		return roomapi.JoinRoomResult{}, fmt.Errorf("check join user: %w", err)
	} else if !exists {
		return roomapi.JoinRoomResult{}, roomapi.ErrUserNotFound
	}

	room, err := findActiveRoomByCode(ctx, tx, params.RoomCode)
	if err != nil {
		return roomapi.JoinRoomResult{}, err
	}

	member, err := findActiveMember(ctx, tx, room.ID, params.UserID)
	if err != nil {
		return roomapi.JoinRoomResult{}, err
	}
	if member.UserID == "" {
		if err := reactivateRoomMember(ctx, tx, room.ID, params.UserID); err != nil {
			return roomapi.JoinRoomResult{}, err
		}
		member = roomapi.Member{
			UserID: params.UserID,
			Role:   "member",
		}
	}

	if err := tx.Commit(); err != nil {
		return roomapi.JoinRoomResult{}, fmt.Errorf("commit join room: %w", err)
	}
	return roomapi.JoinRoomResult{
		Room:   room,
		Member: member,
	}, nil
}

// GetRoomDetail loads the persisted room, media, and active members for the theater page.
func (s *PostgresRoomStore) GetRoomDetail(ctx context.Context, roomCode string) (roomapi.DetailResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return roomapi.DetailResult{}, fmt.Errorf("begin room detail: %w", err)
	}
	defer rollbackTx(tx)

	room, err := findActiveRoomByCode(ctx, tx, roomCode)
	if err != nil {
		return roomapi.DetailResult{}, err
	}

	mediaItem, err := findRoomMedia(ctx, tx, room.MediaItemID)
	if err != nil {
		return roomapi.DetailResult{}, err
	}

	members, err := findActiveMembers(ctx, tx, room.ID)
	if err != nil {
		return roomapi.DetailResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return roomapi.DetailResult{}, fmt.Errorf("commit room detail: %w", err)
	}
	return roomapi.DetailResult{
		Room:    room,
		Media:   mediaItem,
		Members: members,
	}, nil
}

func findRoomMedia(ctx context.Context, tx *sql.Tx, mediaItemID string) (roomapi.Media, error) {
	const query = `
		SELECT
			episode.id::text,
			season.title,
			episode.subtitle,
			episode.media_url,
			episode.duration_ms,
			season.season_label,
			episode.episode_label
		FROM media_episodes AS episode
		INNER JOIN media_seasons AS season ON season.id = episode.season_id
		WHERE episode.id = $1
			AND episode.status = 'active'
			AND season.status = 'active'
		LIMIT 1
	`
	var mediaItem roomapi.Media
	var subtitle sql.NullString
	var durationMs sql.NullInt64
	var seasonLabel sql.NullString
	var episodeLabel sql.NullString
	if err := tx.QueryRowContext(ctx, query, mediaItemID).Scan(
		&mediaItem.ID,
		&mediaItem.Title,
		&subtitle,
		&mediaItem.MediaURL,
		&durationMs,
		&seasonLabel,
		&episodeLabel,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return roomapi.Media{}, roomapi.ErrMediaNotFound
		}
		return roomapi.Media{}, fmt.Errorf("find room media: %w", err)
	}
	mediaItem.Subtitle = nullableStringPtr(subtitle)
	if durationMs.Valid {
		mediaItem.DurationMs = &durationMs.Int64
	}
	mediaItem.SeasonLabel = nullableStringPtr(seasonLabel)
	mediaItem.EpisodeLabel = nullableStringPtr(episodeLabel)
	return mediaItem, nil
}

func findActiveMembers(ctx context.Context, tx *sql.Tx, roomID string) ([]roomapi.Member, error) {
	const query = `
		SELECT users.id::text, users.nickname, users.avatar_seed, users.avatar_url, room_members.role
		FROM room_members
		INNER JOIN users ON users.id = room_members.user_id
		WHERE room_members.room_id = $1 AND room_members.is_active = true
		ORDER BY
			CASE room_members.role WHEN 'host' THEN 0 ELSE 1 END,
			room_members.joined_at ASC
	`
	rows, err := tx.QueryContext(ctx, query, roomID)
	if err != nil {
		return nil, fmt.Errorf("find active members: %w", err)
	}
	defer rows.Close()

	members := make([]roomapi.Member, 0)
	for rows.Next() {
		var member roomapi.Member
		var avatarURL sql.NullString
		if err := rows.Scan(
			&member.UserID,
			&member.Nickname,
			&member.AvatarSeed,
			&avatarURL,
			&member.Role,
		); err != nil {
			return nil, fmt.Errorf("scan active member: %w", err)
		}
		member.AvatarURL = nullableStringPtr(avatarURL)
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active members: %w", err)
	}
	return members, nil
}

func findActiveRoomByCode(ctx context.Context, tx *sql.Tx, roomCode string) (roomapi.Room, error) {
	const query = `
		SELECT id::text, room_code, host_user_id::text, media_episode_id::text, status
		FROM rooms
		WHERE room_code = $1 AND status IN ('active', 'grace_period')
	`
	var room roomapi.Room
	if err := tx.QueryRowContext(ctx, query, roomCode).Scan(
		&room.ID,
		&room.RoomCode,
		&room.HostUserID,
		&room.MediaItemID,
		&room.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return roomapi.Room{}, roomapi.ErrRoomNotFound
		}
		return roomapi.Room{}, fmt.Errorf("find active room by code: %w", err)
	}
	return room, nil
}

func findActiveMember(ctx context.Context, tx *sql.Tx, roomID string, userID string) (roomapi.Member, error) {
	const query = `
		SELECT user_id::text, role
		FROM room_members
		WHERE room_id = $1 AND user_id = $2 AND is_active = true
		LIMIT 1
	`
	var member roomapi.Member
	if err := tx.QueryRowContext(ctx, query, roomID, userID).Scan(&member.UserID, &member.Role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return roomapi.Member{}, nil
		}
		return roomapi.Member{}, fmt.Errorf("find active member: %w", err)
	}
	return member, nil
}

func reactivateRoomMember(ctx context.Context, tx *sql.Tx, roomID string, userID string) error {
	const updateExisting = `
		UPDATE room_members
		SET is_active = true, left_at = NULL, joined_at = NOW(), role = 'member'
		WHERE id = (
			SELECT id
			FROM room_members
			WHERE room_id = $1 AND user_id = $2 AND is_active = false
			ORDER BY joined_at DESC
			LIMIT 1
		)
		RETURNING user_id
	`
	var restoredUserID string
	if err := tx.QueryRowContext(ctx, updateExisting, roomID, userID).Scan(&restoredUserID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("reactivate room member: %w", err)
		}
		if err := insertRoomMember(ctx, tx, roomID, userID, "member"); err != nil {
			return err
		}
	}
	return nil
}

func insertRoomMember(ctx context.Context, tx *sql.Tx, roomID string, userID string, role string) error {
	const query = `
		INSERT INTO room_members (room_id, user_id, role, is_active)
		VALUES ($1, $2, $3, true)
	`
	if _, err := tx.ExecContext(ctx, query, roomID, userID, role); err != nil {
		if isUniqueViolation(err) {
			return nil
		}
		return fmt.Errorf("insert room member: %w", err)
	}
	return nil
}

func rowExists(ctx context.Context, tx *sql.Tx, query string, args ...any) (bool, error) {
	var marker int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&marker); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func rollbackTx(tx *sql.Tx) {
	_ = tx.Rollback()
}

func (s *PostgresRoomStore) MarkRoomGracePeriod(
	ctx context.Context,
	roomCode string,
	lastEmptyAt time.Time,
	destroyAfter time.Time,
) error {
	const query = `
		UPDATE rooms
		SET
			status = 'grace_period',
			last_empty_at = $2,
			destroy_after = $3,
			updated_at = NOW()
		WHERE room_code = $1 AND status IN ('active', 'grace_period')
	`
	if _, err := s.db.ExecContext(ctx, query, roomCode, lastEmptyAt, destroyAfter); err != nil {
		return fmt.Errorf("mark room grace_period: %w", err)
	}
	return nil
}

func (s *PostgresRoomStore) MarkRoomActive(ctx context.Context, roomCode string) error {
	const query = `
		UPDATE rooms
		SET
			status = 'active',
			last_empty_at = NULL,
			destroy_after = NULL,
			updated_at = NOW()
		WHERE room_code = $1 AND status IN ('active', 'grace_period')
	`
	if _, err := s.db.ExecContext(ctx, query, roomCode); err != nil {
		return fmt.Errorf("mark room active: %w", err)
	}
	return nil
}

func (s *PostgresRoomStore) DestroyRoom(ctx context.Context, roomCode string) error {
	const query = `DELETE FROM rooms WHERE room_code = $1`
	if _, err := s.db.ExecContext(ctx, query, roomCode); err != nil {
		return fmt.Errorf("destroy room: %w", err)
	}
	return nil
}
