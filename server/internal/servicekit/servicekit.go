package servicekit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderRequestID      = "X-Request-Id"
	HeaderServiceName    = "X-Service-Name"
	HeaderServiceVersion = "X-Service-Version"
	HeaderTraceParent    = "Traceparent"

	DefaultServiceVersion = "dev"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	InstanceID     string
}

func (c Config) Normalized(fallbackName string) Config {
	c.ServiceName = strings.TrimSpace(c.ServiceName)
	if c.ServiceName == "" {
		c.ServiceName = strings.TrimSpace(fallbackName)
	}
	c.ServiceVersion = strings.TrimSpace(c.ServiceVersion)
	if c.ServiceVersion == "" {
		c.ServiceVersion = DefaultServiceVersion
	}
	c.InstanceID = strings.TrimSpace(c.InstanceID)
	return c
}

type contextKey string

const requestIDContextKey contextKey = "request_id"

func EnsureRequestID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID, ok := ctx.Value(requestIDContextKey).(string); ok && strings.TrimSpace(requestID) != "" {
		return ctx, requestID
	}
	requestID := NewRequestID()
	return context.WithValue(ctx, requestIDContextKey, requestID), requestID
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return strings.TrimSpace(requestID)
}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = NewRequestID()
	}
	return context.WithValue(ctx, requestIDContextKey, requestID)
}

func NewRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "req-" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(bytes[:])
}

func InjectHeaders(header http.Header, config Config, requestID string) {
	if header == nil {
		return
	}
	if strings.TrimSpace(requestID) == "" {
		requestID = NewRequestID()
	}
	header.Set(HeaderRequestID, requestID)
	if strings.TrimSpace(config.ServiceName) != "" {
		header.Set(HeaderServiceName, config.ServiceName)
	}
	if strings.TrimSpace(config.ServiceVersion) != "" {
		header.Set(HeaderServiceVersion, config.ServiceVersion)
	}
}

func RequestIDFromHeaders(header http.Header) string {
	if header == nil {
		return ""
	}
	return strings.TrimSpace(header.Get(HeaderRequestID))
}

func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func BearerToken(header http.Header) string {
	if header == nil {
		return ""
	}
	raw := strings.TrimSpace(header.Get("Authorization"))
	if raw == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(strings.ToLower(raw), strings.ToLower(prefix)) {
		return ""
	}
	return strings.TrimSpace(raw[len(prefix):])
}

func SetBearerToken(header http.Header, token string) {
	if header == nil || strings.TrimSpace(token) == "" {
		return
	}
	header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
}

var ErrUnauthorized = errors.New("unauthorized internal service request")
