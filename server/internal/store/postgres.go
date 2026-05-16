package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"watch_together/server/internal/auth"
	"watch_together/server/internal/model"
)

type PostgresUserStore struct {
	db *gorm.DB
}

const (
	defaultMaxOpenConns    = 20
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
)

// OpenPostgres opens and verifies a GORM-backed PostgreSQL connection for API handlers.
func OpenPostgres(ctx context.Context, databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(defaultMaxOpenConns)
	sqlDB.SetMaxIdleConns(defaultMaxIdleConns)
	sqlDB.SetConnMaxLifetime(defaultConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(defaultConnMaxIdleTime)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// NewPostgresUserStore creates the PostgreSQL-backed user repository.
func NewPostgresUserStore(db *gorm.DB) *PostgresUserStore {
	return &PostgresUserStore{db: db}
}

// CreateUser inserts a new account and returns the persisted public user data.
func (s *PostgresUserStore) CreateUser(ctx context.Context, params auth.CreateUserParams) (auth.User, error) {
	dbUser := model.User{
		Account:      params.Account,
		PasswordHash: params.PasswordHash,
		Nickname:     params.Nickname,
		AvatarSeed:   params.AvatarSeed,
	}
	if err := s.db.WithContext(ctx).Create(&dbUser).Error; err != nil {
		if isUniqueViolation(err) || errors.Is(err, gorm.ErrDuplicatedKey) {
			return auth.User{}, auth.ErrAccountExists
		}
		return auth.User{}, fmt.Errorf("create user: %w", err)
	}
	return userFromModel(dbUser), nil
}

// FindUserByAccount loads a user by account for login verification.
func (s *PostgresUserStore) FindUserByAccount(ctx context.Context, account string) (auth.User, error) {
	var dbUser model.User
	err := s.db.WithContext(ctx).Where("account = ?", account).First(&dbUser).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return auth.User{}, auth.ErrUserNotFound
		}
		return auth.User{}, fmt.Errorf("find user by account: %w", err)
	}
	return userFromModel(dbUser), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func userFromModel(dbUser model.User) auth.User {
	return auth.User{
		ID:           dbUser.ID,
		Account:      dbUser.Account,
		PasswordHash: dbUser.PasswordHash,
		Nickname:     dbUser.Nickname,
		AvatarSeed:   dbUser.AvatarSeed,
		AvatarURL:    dbUser.AvatarURL,
		Bio:          dbUser.Bio,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
