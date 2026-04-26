package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrAccountExists      = errors.New("account already exists")
	ErrInvalidCredentials = errors.New("invalid account or password")
	ErrInvalidInput       = errors.New("invalid input")
	ErrUserNotFound       = errors.New("user not found")
)

type User struct {
	ID           string
	Account      string
	PasswordHash string
	Nickname     string
	AvatarSeed   string
	AvatarURL    *string
	Bio          *string
}

type UserStore interface {
	CreateUser(ctx context.Context, params CreateUserParams) (User, error)
	FindUserByAccount(ctx context.Context, account string) (User, error)
}

type CreateUserParams struct {
	Account      string
	PasswordHash string
	Nickname     string
	AvatarSeed   string
}

type Service struct {
	store UserStore
}

type AuthResult struct {
	User        User
	AccessToken string
}

// NewService builds the auth service around a persistent user store.
func NewService(store UserStore) *Service {
	return &Service{store: store}
}

// Register validates account input, hashes the password, and creates a user.
func (s *Service) Register(ctx context.Context, account string, password string, nickname string) (AuthResult, error) {
	account = normalizeAccount(account)
	nickname = strings.TrimSpace(nickname)
	if account == "" || strings.TrimSpace(password) == "" || nickname == "" {
		return AuthResult{}, ErrInvalidInput
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, CreateUserParams{
		Account:      account,
		PasswordHash: string(passwordHash),
		Nickname:     nickname,
		AvatarSeed:   account,
	})
	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:        user,
		AccessToken: devAccessToken(user.ID),
	}, nil
}

// Login verifies the account and password and returns the minimum auth result.
func (s *Service) Login(ctx context.Context, account string, password string) (AuthResult, error) {
	account = normalizeAccount(account)
	if account == "" || strings.TrimSpace(password) == "" {
		return AuthResult{}, ErrInvalidInput
	}

	user, err := s.store.FindUserByAccount(ctx, account)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	return AuthResult{
		User:        user,
		AccessToken: devAccessToken(user.ID),
	}, nil
}

func normalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

func devAccessToken(userID string) string {
	return "dev_" + userID
}
