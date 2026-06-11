package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	"watch_together/server/internal/internalrpc"
)

func TestRPCClientMatchesLocalIdentityService(t *testing.T) {
	local := NewService(newFakeIdentityUserStore())
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", local)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "secret",
	})

	registered, err := client.Register(context.Background(), " Xingye ", "secret", "Xingye")
	if err != nil {
		t.Fatalf("register through rpc: %v", err)
	}
	if registered.User.Account != "xingye" || registered.AccessToken == "" {
		t.Fatalf("unexpected register result: %+v", registered)
	}

	loggedIn, err := client.Login(context.Background(), "xingye", "secret")
	if err != nil {
		t.Fatalf("login through rpc: %v", err)
	}
	if loggedIn.User.ID != registered.User.ID || loggedIn.AccessToken == "" {
		t.Fatalf("unexpected login result: %+v", loggedIn)
	}

	claims, err := client.VerifyAccessToken(loggedIn.AccessToken)
	if err != nil {
		t.Fatalf("verify token through rpc: %v", err)
	}
	if claims.UserID != registered.User.ID {
		t.Fatalf("expected token user %q, got %q", registered.User.ID, claims.UserID)
	}

	profile, err := client.GetUserProfile(context.Background(), registered.User.ID)
	if err != nil {
		t.Fatalf("profile through rpc: %v", err)
	}
	if profile.ID != registered.User.ID || profile.Nickname != "Xingye" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestRPCClientMapsIdentityErrors(t *testing.T) {
	local := NewService(newFakeIdentityUserStore())
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "", local)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{Timeout: time.Second})
	if _, err := client.Register(context.Background(), "", "secret", "X"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if _, err := client.Login(context.Background(), "missing", "secret"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	if _, err := client.VerifyAccessToken("bad-token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestRPCClientRejectsInvalidAuthToken(t *testing.T) {
	mux := http.NewServeMux()
	RegisterInternalRPC(mux, "", "secret", NewService(newFakeIdentityUserStore()))
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewRPCClient(server.URL, internalrpc.ClientConfig{
		Timeout:   time.Second,
		AuthToken: "wrong",
	})
	_, err := client.Login(context.Background(), "xingye", "secret")
	var connectErr *connect.Error
	if !errors.As(err, &connectErr) || connectErr.Code() != connect.CodeUnauthenticated {
		t.Fatalf("expected unauthenticated connect error, got %v", err)
	}
}

type fakeIdentityUserStore struct {
	usersByAccount map[string]User
	nextID         int
}

func newFakeIdentityUserStore() *fakeIdentityUserStore {
	return &fakeIdentityUserStore{usersByAccount: make(map[string]User)}
}

func (s *fakeIdentityUserStore) CreateUser(_ context.Context, params CreateUserParams) (User, error) {
	if _, exists := s.usersByAccount[params.Account]; exists {
		return User{}, ErrAccountExists
	}
	s.nextID++
	user := User{
		ID:           "user_rpc_test",
		Account:      params.Account,
		PasswordHash: params.PasswordHash,
		Nickname:     params.Nickname,
		AvatarSeed:   params.AvatarSeed,
	}
	if s.nextID > 1 {
		user.ID = user.ID + "_" + string(rune('0'+s.nextID))
	}
	s.usersByAccount[user.Account] = user
	return user, nil
}

func (s *fakeIdentityUserStore) FindUserByAccount(_ context.Context, account string) (User, error) {
	user, ok := s.usersByAccount[account]
	if !ok {
		return User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *fakeIdentityUserStore) FindUserByID(_ context.Context, userID string) (User, error) {
	for _, user := range s.usersByAccount {
		if user.ID == userID {
			return user, nil
		}
	}
	return User{}, ErrUserNotFound
}
