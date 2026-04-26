package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"

	"watch_together/server/internal/auth"
)

type PostgresUserStore struct {
	db *sql.DB
}

// OpenPostgres opens and verifies a PostgreSQL connection for API handlers.
func OpenPostgres(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// NewPostgresUserStore creates the PostgreSQL-backed user repository.
func NewPostgresUserStore(db *sql.DB) *PostgresUserStore {
	return &PostgresUserStore{db: db}
}

// CreateUser inserts a new account and returns the persisted public user data.
func (s *PostgresUserStore) CreateUser(ctx context.Context, params auth.CreateUserParams) (auth.User, error) {
	const query = `
		INSERT INTO users (account, password_hash, nickname, avatar_seed)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, account, password_hash, nickname, avatar_seed, avatar_url, bio
	`

	user, err := scanUser(
		s.db.QueryRowContext(
			ctx,
			query,
			params.Account,
			params.PasswordHash,
			params.Nickname,
			params.AvatarSeed,
		),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return auth.User{}, auth.ErrAccountExists
		}
		return auth.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// FindUserByAccount loads a user by account for login verification.
func (s *PostgresUserStore) FindUserByAccount(ctx context.Context, account string) (auth.User, error) {
	const query = `
		SELECT id::text, account, password_hash, nickname, avatar_seed, avatar_url, bio
		FROM users
		WHERE account = $1
	`

	user, err := scanUser(s.db.QueryRowContext(ctx, query, account))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("find user by account: %w", err)
	}
	return user, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (auth.User, error) {
	var user auth.User
	var avatarURL sql.NullString
	var bio sql.NullString

	if err := row.Scan(
		&user.ID,
		&user.Account,
		&user.PasswordHash,
		&user.Nickname,
		&user.AvatarSeed,
		&avatarURL,
		&bio,
	); err != nil {
		return auth.User{}, err
	}
	if avatarURL.Valid {
		user.AvatarURL = &avatarURL.String
	}
	if bio.Valid {
		user.Bio = &bio.String
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
