package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"watch_together/server/internal/room"
	"watch_together/server/internal/transport"
)

func TestGinRouterHealthz(t *testing.T) {
	router := newTestGinRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if strings.TrimSpace(recorder.Body.String()) != "ok" {
		t.Fatalf("expected health body ok, got %q", recorder.Body.String())
	}
}

func TestGinRouterKeepsProgressWildcardRoute(t *testing.T) {
	router := newTestGinRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/me/media-progress/media_001", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected wrapped progress handler status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "progress service is unavailable") {
		t.Fatalf("expected wrapped progress handler response, got %q", recorder.Body.String())
	}
}

func TestGinRouterKeepsRoomWildcardRoute(t *testing.T) {
	router := newTestGinRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/rooms/A7K2M9/nope", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected wrapped room handler status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "room service is unavailable") {
		t.Fatalf("expected wrapped room handler response, got %q", recorder.Body.String())
	}
}

func TestNewServerKeepsRedisOptionalWhenUnconfigured(t *testing.T) {
	server := NewServer(Config{
		Host: "127.0.0.1",
		Port: "0",
	})

	if server.RedisClient() != nil {
		t.Fatalf("expected nil redis client when REDIS_ADDR is not configured")
	}
}

func newTestGinRouter() http.Handler {
	roomManager := room.NewManager()
	return newGinRouter(
		roomManager,
		false,
		transport.NewRoomHTTPHandler(roomManager, nil),
		transport.NewAuthHTTPHandler(nil),
		transport.NewHomeHTTPHandler(nil),
		transport.NewMediaHTTPHandler(nil),
		transport.NewProgressHTTPHandler(nil),
	)
}
