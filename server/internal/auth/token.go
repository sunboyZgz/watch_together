package auth

import (
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid access token")

const (
	defaultJWTIssuer      = "watch_together"
	defaultAccessTokenTTL = 24 * time.Hour
	defaultLocalJWTSecret = "watch_together_local_dev_jwt_secret_change_me"
)

type TokenConfig struct {
	JWTSecret      string
	Issuer         string
	AccessTokenTTL time.Duration
}

type TokenClaims struct {
	UserID string
}

type accessTokenClaims struct {
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret         []byte
	issuer         string
	accessTokenTTL time.Duration
	now            func() time.Time
}

func DefaultTokenConfig() TokenConfig {
	return TokenConfig{
		JWTSecret:      defaultLocalJWTSecret,
		Issuer:         defaultJWTIssuer,
		AccessTokenTTL: defaultAccessTokenTTL,
	}
}

func NewTokenManager(config TokenConfig) *TokenManager {
	if strings.TrimSpace(config.JWTSecret) == "" {
		config.JWTSecret = defaultLocalJWTSecret
	}
	if strings.TrimSpace(config.Issuer) == "" {
		config.Issuer = defaultJWTIssuer
	}
	if config.AccessTokenTTL <= 0 {
		config.AccessTokenTTL = defaultAccessTokenTTL
	}
	return &TokenManager{
		secret:         []byte(config.JWTSecret),
		issuer:         strings.TrimSpace(config.Issuer),
		accessTokenTTL: config.AccessTokenTTL,
		now:            time.Now,
	}
}

func (m *TokenManager) IssueAccessToken(user User) (string, error) {
	if m == nil {
		m = NewTokenManager(DefaultTokenConfig())
	}
	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		return "", ErrInvalidInput
	}
	now := m.now().UTC()
	claims := accessTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTokenTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", err
	}
	return signed, nil
}

func (m *TokenManager) VerifyAccessToken(rawToken string) (TokenClaims, error) {
	if m == nil {
		m = NewTokenManager(DefaultTokenConfig())
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return TokenClaims{}, ErrInvalidToken
	}

	var claims accessTokenClaims
	token, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}
			return m.secret, nil
		},
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil || token == nil || !token.Valid {
		return TokenClaims{}, ErrInvalidToken
	}
	userID := strings.TrimSpace(claims.Subject)
	if userID == "" {
		return TokenClaims{}, ErrInvalidToken
	}
	return TokenClaims{UserID: userID}, nil
}
