package transport

import (
	"strings"

	"watch_together/server/internal/auth"
)

type accessTokenVerifier interface {
	VerifyAccessToken(rawToken string) (auth.TokenClaims, error)
}

var defaultAccessTokenVerifier = auth.NewTokenManager(auth.DefaultTokenConfig())

func userIDFromAuthorization(header string, verifier accessTokenVerifier) (string, bool) {
	const bearerPrefix = "Bearer "

	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	if token == "" {
		return "", false
	}
	if verifier == nil {
		verifier = defaultAccessTokenVerifier
	}
	claims, err := verifier.VerifyAccessToken(token)
	if err != nil {
		return "", false
	}
	userID := strings.TrimSpace(claims.UserID)
	return userID, userID != ""
}

func bearerTokenFromRequestHeader(header string) (string, bool) {
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(header, bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
	return token, token != ""
}
