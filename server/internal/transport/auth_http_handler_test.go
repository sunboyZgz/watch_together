package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watch_together/server/internal/auth"
)

func TestRegisterCreatesUserWithEnvelope(t *testing.T) {
	handler := NewAuthHTTPHandler(auth.NewService(newFakeUserStore()))
	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/register",
		strings.NewReader(`{"account":"Xingye","password":"secret","nickname":"Xingye"}`),
	)
	recorder := httptest.NewRecorder()

	handler.Register(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data authResponse `json:"data"`
		Meta apiMeta      `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Data.User.Account != "xingye" {
		t.Fatalf("expected normalized account, got %q", response.Data.User.Account)
	}
	if response.Data.User.Nickname != "Xingye" {
		t.Fatalf("expected nickname Xingye, got %q", response.Data.User.Nickname)
	}
	if !strings.HasPrefix(response.Data.AccessToken, "dev_") {
		t.Fatalf("expected dev access token, got %q", response.Data.AccessToken)
	}
	if response.Meta.RequestID == "" {
		t.Fatalf("expected requestId in meta")
	}
}

func TestLoginRejectsInvalidPassword(t *testing.T) {
	store := newFakeUserStore()
	handler := NewAuthHTTPHandler(auth.NewService(store))
	registerRequest := httptest.NewRequest(
		http.MethodPost,
		"/auth/register",
		strings.NewReader(`{"account":"xingye","password":"secret","nickname":"Xingye"}`),
	)
	handler.Register(httptest.NewRecorder(), registerRequest)

	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		strings.NewReader(`{"account":"xingye","password":"wrong"}`),
	)
	recorder := httptest.NewRecorder()

	handler.Login(recorder, loginRequest)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	var response apiErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("expected UNAUTHORIZED, got %q", response.Error.Code)
	}
}

func TestRegisterRejectsDuplicateAccount(t *testing.T) {
	store := newFakeUserStore()
	handler := NewAuthHTTPHandler(auth.NewService(store))
	firstRequest := httptest.NewRequest(
		http.MethodPost,
		"/auth/register",
		strings.NewReader(`{"account":"xingye","password":"secret","nickname":"Xingye"}`),
	)
	handler.Register(httptest.NewRecorder(), firstRequest)
	secondRequest := httptest.NewRequest(
		http.MethodPost,
		"/auth/register",
		strings.NewReader(`{"account":"xingye","password":"secret","nickname":"Xingye 2"}`),
	)
	recorder := httptest.NewRecorder()

	handler.Register(recorder, secondRequest)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, recorder.Code)
	}
}

type fakeUserStore struct {
	usersByAccount map[string]auth.User
	nextID         int
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{usersByAccount: make(map[string]auth.User)}
}

func (s *fakeUserStore) CreateUser(_ context.Context, params auth.CreateUserParams) (auth.User, error) {
	if _, exists := s.usersByAccount[params.Account]; exists {
		return auth.User{}, auth.ErrAccountExists
	}
	s.nextID++
	user := auth.User{
		ID:           fmt.Sprintf("user_test_%d", s.nextID),
		Account:      params.Account,
		PasswordHash: params.PasswordHash,
		Nickname:     params.Nickname,
		AvatarSeed:   params.AvatarSeed,
	}
	s.usersByAccount[user.Account] = user
	return user, nil
}

func (s *fakeUserStore) FindUserByAccount(_ context.Context, account string) (auth.User, error) {
	user, exists := s.usersByAccount[account]
	if !exists {
		return auth.User{}, auth.ErrUserNotFound
	}
	return user, nil
}
