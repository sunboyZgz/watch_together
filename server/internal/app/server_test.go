package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"watch_together/server/internal/cache"
	"watch_together/server/internal/eventbus"
	"watch_together/server/internal/observability"
	"watch_together/server/internal/room"
	"watch_together/server/internal/transport"
)

func TestGinRouterHealthz(t *testing.T) {
	cases := []struct {
		name        string
		runtime     runtimeBoundary
		wantInst    string
		wantRuntime string
	}{
		{
			name:        "default runtime",
			runtime:     runtimeBoundary{},
			wantRuntime: "local_process",
		},
		{
			name: "explicit runtime boundary",
			runtime: runtimeBoundary{
				InstanceID:      "roomserver-a",
				RoomRuntimeMode: "local_process",
			},
			wantInst:    "roomserver-a",
			wantRuntime: "local_process",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newTestGinRouterWithRuntime(tc.runtime)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
			}
			if strings.TrimSpace(recorder.Body.String()) != "ok" {
				t.Fatalf("expected health body ok, got %q", recorder.Body.String())
			}
			if recorder.Header().Get("X-Watch-Together-Instance-ID") != tc.wantInst {
				t.Fatalf("expected instance health header %q, got %q", tc.wantInst, recorder.Header().Get("X-Watch-Together-Instance-ID"))
			}
			if recorder.Header().Get("X-Watch-Together-Room-Runtime") != tc.wantRuntime {
				t.Fatalf("expected room runtime health header %q, got %q", tc.wantRuntime, recorder.Header().Get("X-Watch-Together-Room-Runtime"))
			}
		})
	}
}

func TestGinRouterReadyz(t *testing.T) {
	router := newTestGinRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d body=%s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected ready response, got %s", recorder.Body.String())
	}
}

func TestGinRouterMetricsEnabled(t *testing.T) {
	router := newTestGinRouter()
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "watch_together_websocket_connections_current") {
		t.Fatalf("expected prometheus metrics, got %s", recorder.Body.String())
	}
}

func TestGinRouterMetricsDisabled(t *testing.T) {
	roomManager := room.NewManager()
	router := newGinRouter(
		context.Background(),
		roomManager,
		false,
		WebSocketRuntimeConfig{},
		nil,
		nil,
		transport.NewRoomHTTPHandler(roomManager, nil),
		transport.NewAuthHTTPHandler(nil),
		transport.NewHomeHTTPHandler(nil),
		transport.NewMediaHTTPHandler(nil),
		transport.NewProgressHTTPHandler(nil),
		runtimeBoundary{},
		testReadinessSnapshot("", "local_process"),
		observability.Config{MetricsEnabled: false}.Normalized(),
		observability.NewMetrics(),
		"",
		eventbus.NewDisabledRoomBroadcastBus(),
		eventbus.NewDisabledRoomControlBus(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected metrics disabled status %d, got %d", http.StatusNotFound, recorder.Code)
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

func TestRoomLifecycleHookDeletesRoomStateCacheOnDestroy(t *testing.T) {
	roomManager := room.NewManager()
	jsonStore := &fakeAppJSONStore{}
	roomStateCache := cache.NewRoomStateCache(jsonStore, 0)
	installRoomLifecycleHooks(roomManager, nil, roomStateCache)

	registeredRoom := roomManager.RegisterCreatedRoom("ROOM01", "user-1", "media-1")
	client := room.NewClientConnection(nil)
	client.SetIdentity("user-1", "ROOM01")
	registeredRoom.Join(client)
	roomManager.MarkRoomActive("ROOM01")

	result := roomManager.LeaveClient(client)

	if !result.RoomRemoved {
		t.Fatalf("expected intentional leave to destroy empty room")
	}
	if len(jsonStore.deletedKeys) != 1 {
		t.Fatalf("expected one deleted redis key, got %d", len(jsonStore.deletedKeys))
	}
	if jsonStore.deletedKeys[0] != "wt:room:state:ROOM01:v1" {
		t.Fatalf("unexpected deleted key %q", jsonStore.deletedKeys[0])
	}
}

func newTestGinRouter() http.Handler {
	return newTestGinRouterWithRuntime(runtimeBoundary{})
}

func newTestGinRouterWithRuntime(runtime runtimeBoundary) http.Handler {
	roomManager := room.NewManager()
	return newGinRouter(
		context.Background(),
		roomManager,
		false,
		WebSocketRuntimeConfig{},
		nil,
		nil,
		transport.NewRoomHTTPHandler(roomManager, nil),
		transport.NewAuthHTTPHandler(nil),
		transport.NewHomeHTTPHandler(nil),
		transport.NewMediaHTTPHandler(nil),
		transport.NewProgressHTTPHandler(nil),
		runtime,
		testReadinessSnapshot(runtime.InstanceID, normalizeRoomRuntimeMode(runtime.RoomRuntimeMode)),
		observability.Config{MetricsEnabled: true}.Normalized(),
		observability.NewMetrics(),
		"",
		eventbus.NewDisabledRoomBroadcastBus(),
		eventbus.NewDisabledRoomControlBus(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func testReadinessSnapshot(instanceID string, runtimeMode string) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot("test", instanceID, runtimeMode, nil)
}

type fakeAppJSONStore struct {
	deletedKeys []string
}

func (s *fakeAppJSONStore) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	return false, nil
}

func (s *fakeAppJSONStore) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}

func (s *fakeAppJSONStore) Delete(ctx context.Context, keys ...string) error {
	s.deletedKeys = append(s.deletedKeys, keys...)
	return nil
}
