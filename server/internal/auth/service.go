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
	FindUserByID(ctx context.Context, userID string) (User, error)
}

type BatchUserStore interface {
	FindUsersByIDs(ctx context.Context, userIDs []string) ([]User, error)
}

type CreateUserParams struct {
	Account      string
	PasswordHash string
	Nickname     string
	AvatarSeed   string
}

type Service struct {
	store        UserStore
	tokenManager *TokenManager
}

type AuthResult struct {
	User        User
	AccessToken string
}

type IdentityService interface {
	Register(ctx context.Context, account string, password string, nickname string) (AuthResult, error)
	Login(ctx context.Context, account string, password string) (AuthResult, error)
	VerifyAccessToken(rawToken string) (TokenClaims, error)
	GetUserProfile(ctx context.Context, userID string) (User, error)
	BatchGetUserProfiles(ctx context.Context, userIDs []string) ([]User, error)
}

// NewService builds the auth service around a persistent user store.
func NewService(store UserStore) *Service {
	return NewServiceWithTokenManager(store, NewTokenManager(DefaultTokenConfig()))
}

func NewServiceWithTokenManager(store UserStore, tokenManager *TokenManager) *Service {
	if tokenManager == nil {
		tokenManager = NewTokenManager(DefaultTokenConfig())
	}
	return &Service{
		store:        store,
		tokenManager: tokenManager,
	}
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

	return s.authResult(user)
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

	return s.authResult(user)
}

func (s *Service) VerifyAccessToken(rawToken string) (TokenClaims, error) {
	if s == nil {
		return TokenClaims{}, ErrInvalidToken
	}
	return s.tokenManager.VerifyAccessToken(rawToken)
}

func (s *Service) GetUserProfile(ctx context.Context, userID string) (User, error) {
	if s == nil || s.store == nil {
		return User{}, ErrUserNotFound
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, ErrInvalidInput
	}
	return s.store.FindUserByID(ctx, userID)
}

func (s *Service) BatchGetUserProfiles(ctx context.Context, userIDs []string) ([]User, error) {
	if s == nil || s.store == nil {
		return nil, ErrUserNotFound
	}
	userIDs = normalizeUserIDs(userIDs)
	if len(userIDs) == 0 {
		return nil, nil
	}
	if batchStore, ok := s.store.(BatchUserStore); ok {
		return batchStore.FindUsersByIDs(ctx, userIDs)
	}
	users := make([]User, 0, len(userIDs))
	for _, userID := range userIDs {
		user, err := s.store.FindUserByID(ctx, userID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				continue
			}
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func normalizeAccount(account string) string {
	return strings.ToLower(strings.TrimSpace(account))
}

func normalizeUserIDs(userIDs []string) []string {
	result := make([]string, 0, len(userIDs))
	seen := map[string]struct{}{}
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		result = append(result, userID)
	}
	return result
}

func (s *Service) authResult(user User) (AuthResult, error) {
	accessToken, err := s.tokenManager.IssueAccessToken(user)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{
		User:        user,
		AccessToken: accessToken,
	}, nil
}
