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
	"watch_together/server/internal/servicekit"
	"watch_together/server/internal/transport"

	"gorm.io/gorm"
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
		func(context.Context) observability.ReadinessSnapshot {
			return testReadinessSnapshot("", "local_process")
		},
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
		nil,
	)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected metrics disabled status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestReadinessSnapshotReportsMediaPostgresBoundary(t *testing.T) {
	cases := []struct {
		name       string
		config     Config
		mediaDB    *gorm.DB
		wantStatus string
		wantReq    bool
	}{
		{
			name: "local media database missing",
			config: Config{
				AppEnv:           "test",
				RoomRuntimeMode:  roomRuntimeModeLocalProcess,
				MediaDatabaseURL: "postgres://media-db",
				ServiceClients:   ServiceClientsConfig{MediaMode: "local"},
			},
			wantStatus: "unavailable",
			wantReq:    true,
		},
		{
			name: "local media database connected",
			config: Config{
				AppEnv:           "test",
				RoomRuntimeMode:  roomRuntimeModeLocalProcess,
				MediaDatabaseURL: "postgres://media-db",
				ServiceClients:   ServiceClientsConfig{MediaMode: "local"},
			},
			mediaDB:    &gorm.DB{},
			wantStatus: "ok",
			wantReq:    true,
		},
		{
			name: "rpc media mode does not require roomserver media database",
			config: Config{
				AppEnv:           "test",
				RoomRuntimeMode:  roomRuntimeModeLocalProcess,
				MediaDatabaseURL: "postgres://media-db",
				ServiceClients: ServiceClientsConfig{
					MediaMode: "rpc",
					MediaAddr: "http://mediaservice:8090",
				},
			},
			wantStatus: "disabled",
			wantReq:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := readinessSnapshotFromConfig(
				context.Background(),
				tc.config,
				nil,
				nil,
				tc.mediaDB,
				nil,
				nil,
				eventbus.NewDisabledRoomBroadcastBus(),
				eventbus.NewDisabledRoomControlBus(),
				nil,
				nil,
			)

			dependency, ok := dependencyByName(snapshot.Dependencies, "media_postgres")
			if !ok {
				t.Fatalf("expected media_postgres dependency in readiness snapshot")
			}
			if dependency.Status != tc.wantStatus || dependency.Required != tc.wantReq {
				t.Fatalf("media_postgres = status %q required %t, want status %q required %t",
					dependency.Status,
					dependency.Required,
					tc.wantStatus,
					tc.wantReq,
				)
			}
		})
	}
}

func TestReadinessSnapshotReportsTimelinePostgresBoundary(t *testing.T) {
	cases := []struct {
		name       string
		config     Config
		timelineDB *gorm.DB
		wantStatus string
		wantReq    bool
	}{
		{
			name: "local timeline database missing",
			config: Config{
				AppEnv:              "test",
				RoomRuntimeMode:     roomRuntimeModeDistributedAuthority,
				TimelineDatabaseURL: "postgres://timeline-db",
				ServiceClients:      ServiceClientsConfig{TimelineMode: "local"},
			},
			wantStatus: "unavailable",
			wantReq:    true,
		},
		{
			name: "local timeline database connected",
			config: Config{
				AppEnv:              "test",
				RoomRuntimeMode:     roomRuntimeModeDistributedAuthority,
				TimelineDatabaseURL: "postgres://timeline-db",
				ServiceClients:      ServiceClientsConfig{TimelineMode: "local"},
			},
			timelineDB: &gorm.DB{},
			wantStatus: "ok",
			wantReq:    true,
		},
		{
			name: "rpc timeline mode does not require roomserver timeline database",
			config: Config{
				AppEnv:              "test",
				RoomRuntimeMode:     roomRuntimeModeDistributedAuthority,
				TimelineDatabaseURL: "postgres://timeline-db",
				ServiceClients: ServiceClientsConfig{
					TimelineMode: "rpc",
					TimelineAddr: "http://timelineservice:8090",
				},
			},
			wantStatus: "disabled",
			wantReq:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := readinessSnapshotFromConfig(
				context.Background(),
				tc.config,
				nil,
				nil,
				nil,
				tc.timelineDB,
				nil,
				eventbus.NewDisabledRoomBroadcastBus(),
				eventbus.NewDisabledRoomControlBus(),
				nil,
				nil,
			)

			dependency, ok := dependencyByName(snapshot.Dependencies, "timeline_postgres")
			if !ok {
				t.Fatalf("expected timeline_postgres dependency in readiness snapshot")
			}
			if dependency.Status != tc.wantStatus || dependency.Required != tc.wantReq {
				t.Fatalf("timeline_postgres = status %q required %t, want status %q required %t",
					dependency.Status,
					dependency.Required,
					tc.wantStatus,
					tc.wantReq,
				)
			}
		})
	}
}

func TestReadinessSnapshotProbesRPCDependencies(t *testing.T) {
	readyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer readyServer.Close()
	notReadyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not_ready"}`))
	}))
	defer notReadyServer.Close()

	baseConfig := Config{
		AppEnv:          "test",
		RoomRuntimeMode: roomRuntimeModeLocalProcess,
		Observability:   observability.Config{ReadinessPath: "/readyz"},
		ServiceClients: ServiceClientsConfig{
			IdentityMode:  "rpc",
			IdentityAddr:  readyServer.URL,
			MediaMode:     "rpc",
			MediaAddr:     readyServer.URL,
			TimelineMode:  "rpc",
			TimelineAddr:  readyServer.URL,
			AuthorityMode: "rpc",
			AuthorityAddr: readyServer.URL,
		},
	}
	snapshot := readinessSnapshotFromConfig(
		context.Background(),
		baseConfig,
		nil,
		nil,
		nil,
		nil,
		nil,
		eventbus.NewDisabledRoomBroadcastBus(),
		eventbus.NewDisabledRoomControlBus(),
		nil,
		nil,
	)
	if snapshot.Status != "ready" {
		t.Fatalf("expected ready RPC dependencies to make snapshot ready, got %+v", snapshot)
	}
	for _, name := range []string{"identity_rpc", "media_rpc", "timeline_rpc", "authority_rpc"} {
		dependency, ok := dependencyByName(snapshot.Dependencies, name)
		if !ok || dependency.Status != "ok" || !dependency.Required {
			t.Fatalf("expected %s to be required and ok, got %+v", name, dependency)
		}
	}

	baseConfig.ServiceClients.AuthorityAddr = notReadyServer.URL
	snapshot = readinessSnapshotFromConfig(
		context.Background(),
		baseConfig,
		nil,
		nil,
		nil,
		nil,
		nil,
		eventbus.NewDisabledRoomBroadcastBus(),
		eventbus.NewDisabledRoomControlBus(),
		nil,
		nil,
	)
	dependency, ok := dependencyByName(snapshot.Dependencies, "authority_rpc")
	if !ok || dependency.Status != "unavailable" || snapshot.Status != "not_ready" {
		t.Fatalf("expected unavailable authority RPC to make snapshot not ready, dependency=%+v snapshot=%+v", dependency, snapshot)
	}
}

func TestNewTimelineOutboxStoreDoesNotFallbackWhenConfiguredTimelineDatabaseIsUnavailable(t *testing.T) {
	mainDB := &gorm.DB{}
	store := newTimelineOutboxStore(
		mainDB,
		nil,
		Config{
			TimelineDatabaseURL: "postgres://timeline-db",
			ServiceClients:      ServiceClientsConfig{TimelineMode: "local"},
		},
	)
	if store != nil {
		t.Fatalf("expected nil timeline outbox store when TIMELINE_DATABASE_URL is configured but timeline database is unavailable")
	}

	store = newTimelineOutboxStore(
		mainDB,
		nil,
		Config{ServiceClients: ServiceClientsConfig{TimelineMode: "local"}},
	)
	if store == nil {
		t.Fatalf("expected main database fallback when TIMELINE_DATABASE_URL is empty")
	}
}

func TestNewMediaServiceDoesNotFallbackWhenConfiguredMediaDatabaseIsUnavailable(t *testing.T) {
	mainDB := &gorm.DB{}
	service := newMediaService(
		mainDB,
		nil,
		Config{
			MediaDatabaseURL: "postgres://media-db",
			ServiceClients:   ServiceClientsConfig{MediaMode: "local"},
		},
		servicekit.Config{},
	)
	if service != nil {
		t.Fatalf("expected nil media service when MEDIA_DATABASE_URL is configured but media database is unavailable")
	}

	service = newMediaService(
		mainDB,
		nil,
		Config{ServiceClients: ServiceClientsConfig{MediaMode: "local"}},
		servicekit.Config{},
	)
	if service == nil {
		t.Fatalf("expected main database fallback when MEDIA_DATABASE_URL is empty")
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
		func(context.Context) observability.ReadinessSnapshot {
			return testReadinessSnapshot(runtime.InstanceID, normalizeRoomRuntimeMode(runtime.RoomRuntimeMode))
		},
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
		nil,
	)
}

func testReadinessSnapshot(instanceID string, runtimeMode string) observability.ReadinessSnapshot {
	return observability.NewReadinessSnapshot("test", instanceID, runtimeMode, nil)
}

func dependencyByName(dependencies []observability.DependencyStatus, name string) (observability.DependencyStatus, bool) {
	for _, dependency := range dependencies {
		if dependency.Name == name {
			return dependency, true
		}
	}
	return observability.DependencyStatus{}, false
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
