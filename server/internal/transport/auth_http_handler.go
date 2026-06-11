package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"watch_together/server/internal/auth"
)

type AuthHTTPHandler struct {
	authService authHTTPService
}

type authHTTPService interface {
	Register(ctx context.Context, account string, password string, nickname string) (auth.AuthResult, error)
	Login(ctx context.Context, account string, password string) (auth.AuthResult, error)
}

type authRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Nickname string `json:"nickname,omitempty"`
}

type authResponse struct {
	User        userResponse `json:"user"`
	AccessToken string       `json:"accessToken"`
}

type userResponse struct {
	ID         string  `json:"id"`
	Account    string  `json:"account"`
	Nickname   string  `json:"nickname"`
	AvatarSeed string  `json:"avatarSeed"`
	AvatarURL  *string `json:"avatarUrl"`
}

// NewAuthHTTPHandler builds the HTTP entrypoint for account login and registration.
func NewAuthHTTPHandler(authService authHTTPService) *AuthHTTPHandler {
	return &AuthHTTPHandler{authService: authService}
}

// Login handles POST /auth/login with the shared HTTP API envelope.
func (h *AuthHTTPHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body", nil)
		return
	}

	result, err := h.authService.Login(r.Context(), request.Account, request.Password)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	writeAPISuccess(w, http.StatusOK, authResultResponse(result))
}

// Register handles POST /auth/register with bcrypt password hashing.
func (h *AuthHTTPHandler) Register(w http.ResponseWriter, r *http.Request) {
	if !h.ensureReady(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeAPIError(w, http.StatusMethodNotAllowed, "VALIDATION_ERROR", "method not allowed", nil)
		return
	}

	var request authRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body", nil)
		return
	}

	result, err := h.authService.Register(r.Context(), request.Account, request.Password, request.Nickname)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}
	writeAPISuccess(w, http.StatusCreated, authResultResponse(result))
}

func (h *AuthHTTPHandler) ensureReady(w http.ResponseWriter) bool {
	if h == nil || h.authService == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "INTERNAL_ERROR", "auth service is unavailable", nil)
		return false
	}
	return true
}

func (h *AuthHTTPHandler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeAPIError(w, http.StatusBadRequest, "VALIDATION_ERROR", "account, password, and nickname are required when applicable", nil)
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeAPIError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid account or password", nil)
	case errors.Is(err, auth.ErrAccountExists):
		writeAPIError(w, http.StatusConflict, "CONFLICT", "account already exists", map[string]any{"field": "account"})
	default:
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "auth request failed", nil)
	}
}

func authResultResponse(result auth.AuthResult) authResponse {
	return authResponse{
		User: userResponse{
			ID:         result.User.ID,
			Account:    result.User.Account,
			Nickname:   result.User.Nickname,
			AvatarSeed: result.User.AvatarSeed,
			AvatarURL:  result.User.AvatarURL,
		},
		AccessToken: result.AccessToken,
	}
}
