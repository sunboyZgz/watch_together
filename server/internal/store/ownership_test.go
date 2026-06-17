package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type ownershipRegistry struct {
	Version            int                       `yaml:"version"`
	StoreFiles         map[string]string         `yaml:"store_files"`
	Tables             map[string]tableOwnership `yaml:"tables"`
	CrossContextAccess []crossContextAccess      `yaml:"cross_context_access"`
}

type tableOwnership struct {
	Owner   string   `yaml:"owner"`
	Writers []string `yaml:"writers"`
	Readers []string `yaml:"readers"`
	Status  string   `yaml:"status"`
}

type crossContextAccess struct {
	Caller string   `yaml:"caller"`
	Owner  string   `yaml:"owner"`
	Tables []string `yaml:"tables"`
	Access string   `yaml:"access"`
	Path   string   `yaml:"path"`
	Reason string   `yaml:"reason"`
}

type composeDocument struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Profiles    []string       `yaml:"profiles"`
	Image       string         `yaml:"image"`
	User        string         `yaml:"user"`
	DependsOn   any            `yaml:"depends_on"`
	Environment map[string]any `yaml:"environment"`
	Ports       []string       `yaml:"ports"`
	Volumes     []string       `yaml:"volumes"`
	Healthcheck map[string]any `yaml:"healthcheck"`
}

func TestDatabaseOwnershipRegistryCoversCoreTables(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	required := []string{
		"users",
		"media_tags",
		"media_seasons",
		"media_episodes",
		"media_episode_variants",
		"media_season_tags",
		"rooms",
		"room_members",
		"user_media_progress",
		"room_timeline_outbox",
	}
	for _, table := range required {
		ownership, ok := registry.Tables[table]
		if !ok {
			t.Fatalf("expected ownership registry to include table %s", table)
		}
		if strings.TrimSpace(ownership.Owner) == "" {
			t.Fatalf("expected table %s to declare an owner", table)
		}
		if len(ownership.Writers) == 0 {
			t.Fatalf("expected table %s to declare writer contexts", table)
		}
	}
}

func TestDatabaseOwnershipDocumentReferencesRegistryAndMediaDatabaseBoundary(t *testing.T) {
	content, err := os.ReadFile("../../../docs/database-ownership.md")
	if err != nil {
		t.Fatalf("read database ownership document: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"server/internal/store/db_ownership.yaml",
		"MEDIA_DATABASE_URL",
		"independent media database",
		"home-composition",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected database ownership document to mention %q", expected)
		}
	}
}

func TestComposeDefaultsUseTimelineRPCBoundary(t *testing.T) {
	tests := []struct {
		name                       string
		path                       string
		wantTimelineServiceProfile string
		wantTimelineServiceDefault bool
	}{
		{
			name:                       "local app compose",
			path:                       "../../compose.yaml",
			wantTimelineServiceProfile: "app",
		},
		{
			name:                       "prod compose",
			path:                       "../../compose.prod.yaml",
			wantTimelineServiceDefault: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compose := readCompose(t, tc.path)
			roomserver := requireComposeService(t, compose, "roomserver")
			timelineService := requireComposeService(t, compose, "timelineservice")

			if !dependsOnService(roomserver.DependsOn, "timelineservice") {
				t.Fatalf("roomserver must depend on timelineservice in %s", filepath.Base(tc.path))
			}
			timelineMode := envString(roomserver.Environment, "TIMELINE_SERVICE_MODE")
			if !strings.Contains(timelineMode, "rpc") {
				t.Fatalf("roomserver TIMELINE_SERVICE_MODE = %q, want RPC default", timelineMode)
			}
			timelineDatabaseURL := envString(roomserver.Environment, "TIMELINE_DATABASE_URL")
			if strings.Contains(timelineDatabaseURL, "${TIMELINE_DATABASE_URL") {
				t.Fatalf("roomserver must not receive the timeline-owned TIMELINE_DATABASE_URL directly, got %q", timelineDatabaseURL)
			}
			if timelineDatabaseURL != "" && !strings.Contains(timelineDatabaseURL, "ROOMSERVER_TIMELINE_DATABASE_URL") {
				t.Fatalf("roomserver timeline DB override must be explicit, got %q", timelineDatabaseURL)
			}
			if tc.wantTimelineServiceProfile != "" && !containsContext(timelineService.Profiles, tc.wantTimelineServiceProfile) {
				t.Fatalf("timelineservice profiles = %v, want profile %q", timelineService.Profiles, tc.wantTimelineServiceProfile)
			}
			if tc.wantTimelineServiceDefault && len(timelineService.Profiles) != 0 {
				t.Fatalf("timelineservice should be a default prod service, got profiles %v", timelineService.Profiles)
			}
		})
	}
}

func TestComposeAppAndProdUseFullRPCBoundary(t *testing.T) {
	localCompose := readCompose(t, "../../compose.yaml")
	localRoomserver := requireComposeService(t, localCompose, "roomserver")
	localIdentity := requireComposeService(t, localCompose, "identityservice")
	localRoom := requireComposeService(t, localCompose, "roomservice")
	localMedia := requireComposeService(t, localCompose, "mediaservice")
	localProgress := requireComposeService(t, localCompose, "progressservice")
	localHome := requireComposeService(t, localCompose, "homecompositionservice")
	localTimeline := requireComposeService(t, localCompose, "timelineservice")
	localAuthority := requireComposeService(t, localCompose, "roomauthorityservice")
	localGateway := requireComposeService(t, localCompose, "apigateway")
	localOutbox := requireComposeService(t, localCompose, "outboxworker")
	rollingRoomserver := requireComposeService(t, localCompose, "roomserver-rolling")
	rollingNginx := requireComposeService(t, localCompose, "nginx-rolling")

	for _, serviceName := range []string{"identityservice", "roomservice", "mediaservice", "progressservice", "homecompositionservice", "timelineservice", "roomauthorityservice"} {
		if !dependsOnService(localRoomserver.DependsOn, serviceName) {
			t.Fatalf("app roomserver must depend on %s by default", serviceName)
		}
	}
	if mode := envString(localRoomserver.Environment, "SERVER_EDGE_MODE"); !strings.Contains(mode, "session_gateway") {
		t.Fatalf("app roomserver SERVER_EDGE_MODE = %q, want session_gateway default", mode)
	}
	if databaseURL := envString(localRoomserver.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("app roomserver must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localGateway.Profiles, "app") {
		t.Fatalf("apigateway profiles = %v, want app profile", localGateway.Profiles)
	}
	for _, serviceName := range []string{"identityservice", "roomservice", "mediaservice", "progressservice", "homecompositionservice", "roomauthorityservice"} {
		if !dependsOnService(localGateway.DependsOn, serviceName) {
			t.Fatalf("app apigateway must depend on %s", serviceName)
		}
	}
	if mode := envString(localGateway.Environment, "SERVER_EDGE_MODE"); mode != "api_gateway" {
		t.Fatalf("app apigateway SERVER_EDGE_MODE = %q, want api_gateway", mode)
	}
	if mode := envString(localGateway.Environment, "ROOM_RUNTIME_MODE"); mode != "local_process" {
		t.Fatalf("app apigateway ROOM_RUNTIME_MODE = %q, want local_process", mode)
	}
	if enabled := envString(localGateway.Environment, "WS_CROSS_INSTANCE_BROADCAST_ENABLED"); enabled != "false" {
		t.Fatalf("app apigateway must not enable websocket broadcast, got %q", enabled)
	}
	if natsURL := envString(localGateway.Environment, "NATS_URL"); natsURL != "" {
		t.Fatalf("app apigateway must not receive NATS_URL, got %q", natsURL)
	}
	for _, key := range []string{"DATABASE_URL", "IDENTITY_DATABASE_URL", "ROOM_DATABASE_URL", "MEDIA_DATABASE_URL", "PROGRESS_DATABASE_URL", "TIMELINE_DATABASE_URL"} {
		if value := envString(localGateway.Environment, key); value != "" {
			t.Fatalf("app apigateway must not receive direct %s, got %q", key, value)
		}
	}
	for key, want := range map[string]string{
		"IDENTITY_SERVICE_MODE":  "rpc",
		"ROOM_SERVICE_MODE":      "rpc",
		"MEDIA_SERVICE_MODE":     "rpc",
		"PROGRESS_SERVICE_MODE":  "rpc",
		"HOME_SERVICE_MODE":      "rpc",
		"AUTHORITY_SERVICE_MODE": "rpc",
	} {
		if value := envString(localGateway.Environment, key); value != want {
			t.Fatalf("app apigateway %s = %q, want %q", key, value, want)
		}
	}
	if mode := envString(localRoomserver.Environment, "ROOM_RUNTIME_MODE"); !strings.Contains(mode, "distributed_authority") {
		t.Fatalf("app roomserver ROOM_RUNTIME_MODE = %q, want distributed_authority default", mode)
	}
	if mode := envString(localRoomserver.Environment, "MEDIA_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver MEDIA_SERVICE_MODE = %q, want rpc default", mode)
	}
	if mode := envString(localRoomserver.Environment, "IDENTITY_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver IDENTITY_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(localRoomserver.Environment, "IDENTITY_SERVICE_ADDR"); !strings.Contains(addr, "http://identityservice:8090") {
		t.Fatalf("app roomserver IDENTITY_SERVICE_ADDR = %q, want identityservice URL", addr)
	}
	if mode := envString(localRoomserver.Environment, "ROOM_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver ROOM_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(localRoomserver.Environment, "ROOM_SERVICE_ADDR"); !strings.Contains(addr, "http://roomservice:8090") {
		t.Fatalf("app roomserver ROOM_SERVICE_ADDR = %q, want roomservice URL", addr)
	}
	roomDatabaseURL := envString(localRoomserver.Environment, "ROOM_DATABASE_URL")
	if strings.Contains(roomDatabaseURL, "${ROOM_DATABASE_URL") {
		t.Fatalf("app roomserver must not receive room-owned ROOM_DATABASE_URL directly, got %q", roomDatabaseURL)
	}
	if roomDatabaseURL != "" && !strings.Contains(roomDatabaseURL, "ROOMSERVER_ROOM_DATABASE_URL") {
		t.Fatalf("app roomserver room DB override must be explicit, got %q", roomDatabaseURL)
	}
	identityDatabaseURL := envString(localRoomserver.Environment, "IDENTITY_DATABASE_URL")
	if strings.Contains(identityDatabaseURL, "${IDENTITY_DATABASE_URL") {
		t.Fatalf("app roomserver must not receive identity-owned IDENTITY_DATABASE_URL directly, got %q", identityDatabaseURL)
	}
	if identityDatabaseURL != "" && !strings.Contains(identityDatabaseURL, "ROOMSERVER_IDENTITY_DATABASE_URL") {
		t.Fatalf("app roomserver identity DB override must be explicit, got %q", identityDatabaseURL)
	}
	if addr := envString(localRoomserver.Environment, "MEDIA_SERVICE_ADDR"); !strings.Contains(addr, "http://mediaservice:8090") {
		t.Fatalf("app roomserver MEDIA_SERVICE_ADDR = %q, want mediaservice URL", addr)
	}
	if mode := envString(localRoomserver.Environment, "PROGRESS_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver PROGRESS_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(localRoomserver.Environment, "PROGRESS_SERVICE_ADDR"); !strings.Contains(addr, "http://progressservice:8090") {
		t.Fatalf("app roomserver PROGRESS_SERVICE_ADDR = %q, want progressservice URL", addr)
	}
	progressDatabaseURL := envString(localRoomserver.Environment, "PROGRESS_DATABASE_URL")
	if strings.Contains(progressDatabaseURL, "${PROGRESS_DATABASE_URL") {
		t.Fatalf("app roomserver must not receive progress-owned PROGRESS_DATABASE_URL directly, got %q", progressDatabaseURL)
	}
	if progressDatabaseURL != "" && !strings.Contains(progressDatabaseURL, "ROOMSERVER_PROGRESS_DATABASE_URL") {
		t.Fatalf("app roomserver progress DB override must be explicit, got %q", progressDatabaseURL)
	}
	if mode := envString(localRoomserver.Environment, "HOME_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver HOME_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(localRoomserver.Environment, "HOME_SERVICE_ADDR"); !strings.Contains(addr, "http://homecompositionservice:8090") {
		t.Fatalf("app roomserver HOME_SERVICE_ADDR = %q, want homecompositionservice URL", addr)
	}
	mediaDatabaseURL := envString(localRoomserver.Environment, "MEDIA_DATABASE_URL")
	if strings.Contains(mediaDatabaseURL, "${MEDIA_DATABASE_URL") {
		t.Fatalf("app roomserver must not receive media-owned MEDIA_DATABASE_URL directly, got %q", mediaDatabaseURL)
	}
	if mediaDatabaseURL != "" && !strings.Contains(mediaDatabaseURL, "ROOMSERVER_MEDIA_DATABASE_URL") {
		t.Fatalf("app roomserver media DB override must be explicit, got %q", mediaDatabaseURL)
	}
	if mode := envString(localRoomserver.Environment, "AUTHORITY_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("app roomserver AUTHORITY_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(localRoomserver.Environment, "AUTHORITY_SERVICE_ADDR"); !strings.Contains(addr, "http://roomauthorityservice:8090") {
		t.Fatalf("app roomserver AUTHORITY_SERVICE_ADDR = %q, want roomauthorityservice URL", addr)
	}
	if leaseID := envString(localRoomserver.Environment, "AUTHORITY_LEASE_INSTANCE_ID"); !strings.Contains(leaseID, "roomauthorityservice-1") {
		t.Fatalf("app roomserver AUTHORITY_LEASE_INSTANCE_ID = %q, want roomauthorityservice lease owner", leaseID)
	}
	if timeout := envString(localRoomserver.Environment, "WS_CONTROL_REQUEST_TIMEOUT_MS"); !strings.Contains(timeout, "10000") {
		t.Fatalf("app roomserver WS_CONTROL_REQUEST_TIMEOUT_MS = %q, want cold-start authority RPC budget", timeout)
	}
	if serviceInstanceID := envString(localAuthority.Environment, "SERVER_INSTANCE_ID"); !strings.Contains(serviceInstanceID, "roomauthorityservice-1") {
		t.Fatalf("roomauthorityservice SERVER_INSTANCE_ID = %q, want matching lease owner", serviceInstanceID)
	}
	if !containsContext(localIdentity.Profiles, "app") {
		t.Fatalf("identityservice profiles = %v, want app profile", localIdentity.Profiles)
	}
	if !dependsOnService(localIdentity.DependsOn, "identity-postgres-init") {
		t.Fatalf("identityservice must depend on identity-postgres-init")
	}
	if identityDatabaseURL := envString(localIdentity.Environment, "IDENTITY_DATABASE_URL"); !strings.Contains(identityDatabaseURL, "anime_watch_identity_dev") {
		t.Fatalf("identityservice IDENTITY_DATABASE_URL = %q, want local identity database", identityDatabaseURL)
	}
	if databaseURL := envString(localIdentity.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("identityservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localRoom.Profiles, "app") {
		t.Fatalf("roomservice profiles = %v, want app profile", localRoom.Profiles)
	}
	if !dependsOnService(localRoom.DependsOn, "room-postgres-init") {
		t.Fatalf("roomservice must depend on room-postgres-init")
	}
	if !dependsOnService(localRoom.DependsOn, "identityservice") {
		t.Fatalf("roomservice must depend on identityservice")
	}
	if roomDatabaseURL := envString(localRoom.Environment, "ROOM_DATABASE_URL"); !strings.Contains(roomDatabaseURL, "anime_watch_room_dev") {
		t.Fatalf("roomservice ROOM_DATABASE_URL = %q, want local room database", roomDatabaseURL)
	}
	if databaseURL := envString(localRoom.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("roomservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localMedia.Profiles, "app") {
		t.Fatalf("mediaservice profiles = %v, want app profile", localMedia.Profiles)
	}
	if !dependsOnService(localMedia.DependsOn, "media-postgres-init") {
		t.Fatalf("mediaservice must depend on media-postgres-init")
	}
	if mediaDatabaseURL := envString(localMedia.Environment, "MEDIA_DATABASE_URL"); !strings.Contains(mediaDatabaseURL, "anime_watch_media_dev") {
		t.Fatalf("mediaservice MEDIA_DATABASE_URL = %q, want local media database", mediaDatabaseURL)
	}
	if databaseURL := envString(localMedia.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("mediaservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localProgress.Profiles, "app") {
		t.Fatalf("progressservice profiles = %v, want app profile", localProgress.Profiles)
	}
	if !dependsOnService(localProgress.DependsOn, "progress-postgres-init") {
		t.Fatalf("progressservice must depend on progress-postgres-init")
	}
	if !dependsOnService(localProgress.DependsOn, "identityservice") {
		t.Fatalf("progressservice must depend on identityservice")
	}
	if !dependsOnService(localProgress.DependsOn, "mediaservice") {
		t.Fatalf("progressservice must depend on mediaservice")
	}
	if progressDatabaseURL := envString(localProgress.Environment, "PROGRESS_DATABASE_URL"); !strings.Contains(progressDatabaseURL, "anime_watch_progress_dev") {
		t.Fatalf("progressservice PROGRESS_DATABASE_URL = %q, want local progress database", progressDatabaseURL)
	}
	if databaseURL := envString(localProgress.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("progressservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localHome.Profiles, "app") {
		t.Fatalf("homecompositionservice profiles = %v, want app profile", localHome.Profiles)
	}
	if !dependsOnService(localHome.DependsOn, "identityservice") {
		t.Fatalf("homecompositionservice must depend on identityservice")
	}
	if !dependsOnService(localHome.DependsOn, "progressservice") {
		t.Fatalf("homecompositionservice must depend on progressservice")
	}
	if !dependsOnService(localHome.DependsOn, "mediaservice") {
		t.Fatalf("homecompositionservice must depend on mediaservice")
	}
	if databaseURL := envString(localHome.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("homecompositionservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localTimeline.Profiles, "app") {
		t.Fatalf("timelineservice profiles = %v, want app profile", localTimeline.Profiles)
	}
	if !dependsOnService(localTimeline.DependsOn, "timeline-postgres-init") {
		t.Fatalf("timelineservice must depend on timeline-postgres-init")
	}
	if timelineDatabaseURL := envString(localTimeline.Environment, "TIMELINE_DATABASE_URL"); !strings.Contains(timelineDatabaseURL, "anime_watch_timeline_dev") {
		t.Fatalf("timelineservice TIMELINE_DATABASE_URL = %q, want local timeline database", timelineDatabaseURL)
	}
	if databaseURL := envString(localTimeline.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("timelineservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if timelineDatabaseURL := envString(localOutbox.Environment, "TIMELINE_DATABASE_URL"); !strings.Contains(timelineDatabaseURL, "anime_watch_timeline_dev") {
		t.Fatalf("outboxworker TIMELINE_DATABASE_URL = %q, want local timeline database", timelineDatabaseURL)
	}
	if databaseURL := envString(localOutbox.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("outboxworker must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !containsContext(localAuthority.Profiles, "app") {
		t.Fatalf("roomauthorityservice profiles = %v, want app profile", localAuthority.Profiles)
	}
	if old := envString(localRoomserver.Environment, "AUTHORITY_SERVICE_INSTANCE_ID"); old != "" {
		t.Fatalf("app roomserver must not use deprecated AUTHORITY_SERVICE_INSTANCE_ID, got %q", old)
	}
	if !containsContext(rollingRoomserver.Profiles, "rolling-smoke") {
		t.Fatalf("roomserver-rolling profiles = %v, want rolling-smoke profile", rollingRoomserver.Profiles)
	}
	if primaryInstanceID := envString(localRoomserver.Environment, "SERVER_INSTANCE_ID"); strings.Contains(primaryInstanceID, "local-roomserver-2") {
		t.Fatalf("primary roomserver SERVER_INSTANCE_ID unexpectedly matches rolling instance: %q", primaryInstanceID)
	}
	if rollingInstanceID := envString(rollingRoomserver.Environment, "SERVER_INSTANCE_ID"); rollingInstanceID != "local-roomserver-2" {
		t.Fatalf("roomserver-rolling SERVER_INSTANCE_ID = %q, want local-roomserver-2", rollingInstanceID)
	}
	if mode := envString(rollingRoomserver.Environment, "SERVER_EDGE_MODE"); !strings.Contains(mode, "session_gateway") {
		t.Fatalf("roomserver-rolling SERVER_EDGE_MODE = %q, want session_gateway", mode)
	}
	if runtimeMode := envString(rollingRoomserver.Environment, "ROOM_RUNTIME_MODE"); !strings.Contains(runtimeMode, "distributed_authority") {
		t.Fatalf("roomserver-rolling ROOM_RUNTIME_MODE = %q, want distributed_authority", runtimeMode)
	}
	if timeout := envString(rollingRoomserver.Environment, "WS_CONTROL_REQUEST_TIMEOUT_MS"); !strings.Contains(timeout, "10000") {
		t.Fatalf("roomserver-rolling WS_CONTROL_REQUEST_TIMEOUT_MS = %q, want cold-start authority RPC budget", timeout)
	}
	for _, serviceName := range []string{"identityservice", "roomservice", "mediaservice", "progressservice", "homecompositionservice", "timelineservice", "roomauthorityservice", "redis", "nats", "kafka"} {
		if !dependsOnService(rollingRoomserver.DependsOn, serviceName) {
			t.Fatalf("roomserver-rolling must depend on %s", serviceName)
		}
	}
	for _, key := range []string{"DATABASE_URL", "IDENTITY_DATABASE_URL", "ROOM_DATABASE_URL", "MEDIA_DATABASE_URL", "PROGRESS_DATABASE_URL", "TIMELINE_DATABASE_URL"} {
		value := envString(rollingRoomserver.Environment, key)
		if strings.Contains(value, "${"+key) {
			t.Fatalf("roomserver-rolling must not receive direct %s, got %q", key, value)
		}
		if value != "" && !strings.Contains(value, "ROOMSERVER_"+key) {
			t.Fatalf("roomserver-rolling %s override must be explicit, got %q", key, value)
		}
	}
	if !containsContext(rollingNginx.Profiles, "rolling-smoke") {
		t.Fatalf("nginx-rolling profiles = %v, want rolling-smoke profile", rollingNginx.Profiles)
	}
	for _, serviceName := range []string{"apigateway", "roomserver", "roomserver-rolling", "minio"} {
		if !dependsOnService(rollingNginx.DependsOn, serviceName) {
			t.Fatalf("nginx-rolling must depend on %s", serviceName)
		}
	}
	if !containsContext(rollingRoomserver.Ports, "8099:8080") {
		t.Fatalf("roomserver-rolling ports = %v, want 8099:8080", rollingRoomserver.Ports)
	}
	if !containsContext(rollingNginx.Ports, "8081:80") {
		t.Fatalf("nginx-rolling ports = %v, want 8081:80", rollingNginx.Ports)
	}

	pilotRoomserver := requireComposeService(t, localCompose, "roomserver-rpc-pilot")
	pilotAuthority := requireComposeService(t, localCompose, "roomauthorityservice")
	if !dependsOnService(pilotRoomserver.DependsOn, "identityservice") {
		t.Fatalf("rpc-pilot roomserver must depend on identityservice")
	}
	if !dependsOnService(pilotRoomserver.DependsOn, "roomservice") {
		t.Fatalf("rpc-pilot roomserver must depend on roomservice")
	}
	if !dependsOnService(pilotRoomserver.DependsOn, "mediaservice") {
		t.Fatalf("rpc-pilot roomserver must depend on mediaservice")
	}
	if !dependsOnService(pilotRoomserver.DependsOn, "progressservice") {
		t.Fatalf("rpc-pilot roomserver must depend on progressservice")
	}
	if !dependsOnService(pilotRoomserver.DependsOn, "homecompositionservice") {
		t.Fatalf("rpc-pilot roomserver must depend on homecompositionservice")
	}
	if !dependsOnService(pilotRoomserver.DependsOn, "roomauthorityservice") {
		t.Fatalf("rpc-pilot roomserver must depend on roomauthorityservice")
	}
	if mode := envString(pilotRoomserver.Environment, "AUTHORITY_SERVICE_MODE"); mode != "rpc" {
		t.Fatalf("rpc-pilot roomserver AUTHORITY_SERVICE_MODE = %q, want rpc", mode)
	}
	if addr := envString(pilotRoomserver.Environment, "AUTHORITY_SERVICE_ADDR"); addr != "http://roomauthorityservice:8090" {
		t.Fatalf("rpc-pilot roomserver AUTHORITY_SERVICE_ADDR = %q, want roomauthorityservice URL", addr)
	}
	if leaseID := envString(pilotRoomserver.Environment, "AUTHORITY_LEASE_INSTANCE_ID"); !strings.Contains(leaseID, "roomauthorityservice-1") {
		t.Fatalf("rpc-pilot roomserver AUTHORITY_LEASE_INSTANCE_ID = %q, want roomauthorityservice lease owner", leaseID)
	}
	if old := envString(pilotRoomserver.Environment, "AUTHORITY_SERVICE_INSTANCE_ID"); old != "" {
		t.Fatalf("rpc-pilot roomserver must not use deprecated AUTHORITY_SERVICE_INSTANCE_ID, got %q", old)
	}
	if !containsContext(pilotAuthority.Profiles, "rpc-pilot") {
		t.Fatalf("roomauthorityservice profiles = %v, want rpc-pilot profile", pilotAuthority.Profiles)
	}
	if old := envString(pilotAuthority.Environment, "AUTHORITY_SERVICE_INSTANCE_ID"); old != "" {
		t.Fatalf("roomauthorityservice must not use deprecated AUTHORITY_SERVICE_INSTANCE_ID, got %q", old)
	}
	if addr := envString(pilotAuthority.Environment, "ROOM_SERVICE_ADDR"); !strings.Contains(addr, "http://roomservice:8090") {
		t.Fatalf("roomauthorityservice ROOM_SERVICE_ADDR = %q, want roomservice URL", addr)
	}
	if databaseURL := envString(pilotAuthority.Environment, "ROOM_DATABASE_URL"); databaseURL != "" {
		t.Fatalf("roomauthorityservice must not receive direct ROOM_DATABASE_URL, got %q", databaseURL)
	}

	prodCompose := readCompose(t, "../../compose.prod.yaml")
	prodRoomserver := requireComposeService(t, prodCompose, "roomserver")
	prodGateway := requireComposeService(t, prodCompose, "apigateway")
	prodIdentity := requireComposeService(t, prodCompose, "identityservice")
	prodRoom := requireComposeService(t, prodCompose, "roomservice")
	prodMedia := requireComposeService(t, prodCompose, "mediaservice")
	prodProgress := requireComposeService(t, prodCompose, "progressservice")
	prodHome := requireComposeService(t, prodCompose, "homecompositionservice")
	prodTimeline := requireComposeService(t, prodCompose, "timelineservice")
	prodAuthority := requireComposeService(t, prodCompose, "roomauthorityservice")
	prodOutbox := requireComposeService(t, prodCompose, "outboxworker")
	if len(prodAuthority.Profiles) != 0 {
		t.Fatalf("prod roomauthorityservice should be a default service, got profiles %v", prodAuthority.Profiles)
	}
	if !dependsOnService(prodRoomserver.DependsOn, "mediaservice") {
		t.Fatalf("prod roomserver must depend on mediaservice by default")
	}
	if !dependsOnService(prodRoomserver.DependsOn, "roomservice") {
		t.Fatalf("prod roomserver must depend on roomservice by default")
	}
	if !dependsOnService(prodRoomserver.DependsOn, "progressservice") {
		t.Fatalf("prod roomserver must depend on progressservice by default")
	}
	if !dependsOnService(prodRoomserver.DependsOn, "homecompositionservice") {
		t.Fatalf("prod roomserver must depend on homecompositionservice by default")
	}
	if !dependsOnService(prodRoomserver.DependsOn, "identityservice") {
		t.Fatalf("prod roomserver must depend on identityservice by default")
	}
	if !dependsOnService(prodRoomserver.DependsOn, "roomauthorityservice") {
		t.Fatalf("prod roomserver must depend on roomauthorityservice by default")
	}
	if mode := envString(prodRoomserver.Environment, "SERVER_EDGE_MODE"); !strings.Contains(mode, "session_gateway") {
		t.Fatalf("prod roomserver SERVER_EDGE_MODE = %q, want session_gateway default", mode)
	}
	if databaseURL := envString(prodRoomserver.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod roomserver must not receive main DATABASE_URL, got %q", databaseURL)
	}
	for _, serviceName := range []string{"identityservice", "roomservice", "mediaservice", "progressservice", "homecompositionservice", "roomauthorityservice"} {
		if !dependsOnService(prodGateway.DependsOn, serviceName) {
			t.Fatalf("prod apigateway must depend on %s", serviceName)
		}
	}
	if mode := envString(prodGateway.Environment, "SERVER_EDGE_MODE"); mode != "api_gateway" {
		t.Fatalf("prod apigateway SERVER_EDGE_MODE = %q, want api_gateway", mode)
	}
	if mode := envString(prodGateway.Environment, "ROOM_RUNTIME_MODE"); mode != "local_process" {
		t.Fatalf("prod apigateway ROOM_RUNTIME_MODE = %q, want local_process", mode)
	}
	if enabled := envString(prodGateway.Environment, "WS_CROSS_INSTANCE_BROADCAST_ENABLED"); enabled != "false" {
		t.Fatalf("prod apigateway must not enable websocket broadcast, got %q", enabled)
	}
	if natsURL := envString(prodGateway.Environment, "NATS_URL"); natsURL != "" {
		t.Fatalf("prod apigateway must not receive NATS_URL, got %q", natsURL)
	}
	for _, key := range []string{"DATABASE_URL", "IDENTITY_DATABASE_URL", "ROOM_DATABASE_URL", "MEDIA_DATABASE_URL", "PROGRESS_DATABASE_URL", "TIMELINE_DATABASE_URL"} {
		if value := envString(prodGateway.Environment, key); value != "" {
			t.Fatalf("prod apigateway must not receive direct %s, got %q", key, value)
		}
	}
	if mode := envString(prodRoomserver.Environment, "ROOM_RUNTIME_MODE"); !strings.Contains(mode, "distributed_authority") {
		t.Fatalf("prod roomserver ROOM_RUNTIME_MODE = %q, want distributed_authority default", mode)
	}
	if enabled := envString(prodRoomserver.Environment, "WS_CROSS_INSTANCE_BROADCAST_ENABLED"); !strings.Contains(enabled, "true") {
		t.Fatalf("prod roomserver WS_CROSS_INSTANCE_BROADCAST_ENABLED = %q, want true default", enabled)
	}
	if mode := envString(prodRoomserver.Environment, "ROOM_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("prod roomserver ROOM_SERVICE_MODE = %q, want rpc default", mode)
	}
	if mode := envString(prodRoomserver.Environment, "MEDIA_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("prod roomserver MEDIA_SERVICE_MODE = %q, want rpc default", mode)
	}
	if mode := envString(prodRoomserver.Environment, "PROGRESS_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("prod roomserver PROGRESS_SERVICE_MODE = %q, want rpc default", mode)
	}
	if mode := envString(prodRoomserver.Environment, "HOME_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("prod roomserver HOME_SERVICE_MODE = %q, want rpc default", mode)
	}
	if mediaDatabaseURL := envString(prodRoomserver.Environment, "MEDIA_DATABASE_URL"); strings.Contains(mediaDatabaseURL, "${MEDIA_DATABASE_URL") {
		t.Fatalf("prod roomserver must not receive media-owned MEDIA_DATABASE_URL directly, got %q", mediaDatabaseURL)
	}
	if roomDatabaseURL := envString(prodRoomserver.Environment, "ROOM_DATABASE_URL"); strings.Contains(roomDatabaseURL, "${ROOM_DATABASE_URL") {
		t.Fatalf("prod roomserver must not receive room-owned ROOM_DATABASE_URL directly, got %q", roomDatabaseURL)
	}
	if progressDatabaseURL := envString(prodRoomserver.Environment, "PROGRESS_DATABASE_URL"); strings.Contains(progressDatabaseURL, "${PROGRESS_DATABASE_URL") {
		t.Fatalf("prod roomserver must not receive progress-owned PROGRESS_DATABASE_URL directly, got %q", progressDatabaseURL)
	}
	if identityDatabaseURL := envString(prodRoomserver.Environment, "IDENTITY_DATABASE_URL"); strings.Contains(identityDatabaseURL, "${IDENTITY_DATABASE_URL") {
		t.Fatalf("prod roomserver must not receive identity-owned IDENTITY_DATABASE_URL directly, got %q", identityDatabaseURL)
	}
	if !dependsOnService(prodIdentity.DependsOn, "identity-postgres-init") {
		t.Fatalf("prod identityservice must depend on identity-postgres-init")
	}
	if identityDatabaseURL := envString(prodIdentity.Environment, "IDENTITY_DATABASE_URL"); !strings.Contains(identityDatabaseURL, "watch_together_identity_prod") {
		t.Fatalf("prod identityservice IDENTITY_DATABASE_URL = %q, want prod identity database", identityDatabaseURL)
	}
	if databaseURL := envString(prodIdentity.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod identityservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if roomDatabaseURL := envString(prodRoom.Environment, "ROOM_DATABASE_URL"); !strings.Contains(roomDatabaseURL, "watch_together_room_prod") {
		t.Fatalf("prod roomservice ROOM_DATABASE_URL = %q, want prod room database", roomDatabaseURL)
	}
	if databaseURL := envString(prodRoom.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod roomservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !dependsOnService(prodRoom.DependsOn, "room-postgres-init") {
		t.Fatalf("prod roomservice must depend on room-postgres-init")
	}
	if !dependsOnService(prodMedia.DependsOn, "media-postgres-init") {
		t.Fatalf("prod mediaservice must depend on media-postgres-init")
	}
	if mediaDatabaseURL := envString(prodMedia.Environment, "MEDIA_DATABASE_URL"); !strings.Contains(mediaDatabaseURL, "watch_together_media_prod") {
		t.Fatalf("prod mediaservice MEDIA_DATABASE_URL = %q, want prod media database", mediaDatabaseURL)
	}
	if databaseURL := envString(prodMedia.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod mediaservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !dependsOnService(prodProgress.DependsOn, "progress-postgres-init") {
		t.Fatalf("prod progressservice must depend on progress-postgres-init")
	}
	if !dependsOnService(prodProgress.DependsOn, "identityservice") {
		t.Fatalf("prod progressservice must depend on identityservice")
	}
	if !dependsOnService(prodProgress.DependsOn, "mediaservice") {
		t.Fatalf("prod progressservice must depend on mediaservice")
	}
	if progressDatabaseURL := envString(prodProgress.Environment, "PROGRESS_DATABASE_URL"); !strings.Contains(progressDatabaseURL, "watch_together_progress_prod") {
		t.Fatalf("prod progressservice PROGRESS_DATABASE_URL = %q, want prod progress database", progressDatabaseURL)
	}
	if databaseURL := envString(prodProgress.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod progressservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !dependsOnService(prodHome.DependsOn, "identityservice") {
		t.Fatalf("prod homecompositionservice must depend on identityservice")
	}
	if !dependsOnService(prodHome.DependsOn, "progressservice") {
		t.Fatalf("prod homecompositionservice must depend on progressservice")
	}
	if !dependsOnService(prodHome.DependsOn, "mediaservice") {
		t.Fatalf("prod homecompositionservice must depend on mediaservice")
	}
	if databaseURL := envString(prodHome.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod homecompositionservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if !dependsOnService(prodTimeline.DependsOn, "timeline-postgres-init") {
		t.Fatalf("prod timelineservice must depend on timeline-postgres-init")
	}
	if timelineDatabaseURL := envString(prodTimeline.Environment, "TIMELINE_DATABASE_URL"); !strings.Contains(timelineDatabaseURL, "watch_together_timeline_prod") {
		t.Fatalf("prod timelineservice TIMELINE_DATABASE_URL = %q, want prod timeline database", timelineDatabaseURL)
	}
	if databaseURL := envString(prodTimeline.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod timelineservice must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if timelineDatabaseURL := envString(prodOutbox.Environment, "TIMELINE_DATABASE_URL"); !strings.Contains(timelineDatabaseURL, "watch_together_timeline_prod") {
		t.Fatalf("prod outboxworker TIMELINE_DATABASE_URL = %q, want prod timeline database", timelineDatabaseURL)
	}
	if databaseURL := envString(prodOutbox.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod outboxworker must not receive main DATABASE_URL, got %q", databaseURL)
	}
	if len(prodIdentity.Profiles) != 0 {
		t.Fatalf("prod identityservice should be a default service, got profiles %v", prodIdentity.Profiles)
	}
	if len(prodMedia.Profiles) != 0 {
		t.Fatalf("prod mediaservice should be a default service, got profiles %v", prodMedia.Profiles)
	}
	if len(prodRoom.Profiles) != 0 {
		t.Fatalf("prod roomservice should be a default service, got profiles %v", prodRoom.Profiles)
	}
	if len(prodProgress.Profiles) != 0 {
		t.Fatalf("prod progressservice should be a default service, got profiles %v", prodProgress.Profiles)
	}
	if len(prodHome.Profiles) != 0 {
		t.Fatalf("prod homecompositionservice should be a default service, got profiles %v", prodHome.Profiles)
	}
	if len(prodTimeline.Profiles) != 0 {
		t.Fatalf("prod timelineservice should be a default service, got profiles %v", prodTimeline.Profiles)
	}
	if mode := envString(prodRoomserver.Environment, "AUTHORITY_SERVICE_MODE"); !strings.Contains(mode, "rpc") {
		t.Fatalf("prod roomserver AUTHORITY_SERVICE_MODE = %q, want rpc default", mode)
	}
	if addr := envString(prodRoomserver.Environment, "AUTHORITY_SERVICE_ADDR"); !strings.Contains(addr, "http://roomauthorityservice:8090") {
		t.Fatalf("prod roomserver AUTHORITY_SERVICE_ADDR = %q, want roomauthorityservice URL", addr)
	}
	if leaseID := envString(prodRoomserver.Environment, "AUTHORITY_LEASE_INSTANCE_ID"); !strings.Contains(leaseID, "roomauthorityservice-prod-1") {
		t.Fatalf("prod roomserver AUTHORITY_LEASE_INSTANCE_ID = %q, want authority service lease owner", leaseID)
	}
	if timeout := envString(prodRoomserver.Environment, "WS_CONTROL_REQUEST_TIMEOUT_MS"); !strings.Contains(timeout, "10000") {
		t.Fatalf("prod roomserver WS_CONTROL_REQUEST_TIMEOUT_MS = %q, want cold-start authority RPC budget", timeout)
	}
	if old := envString(prodRoomserver.Environment, "AUTHORITY_SERVICE_INSTANCE_ID"); old != "" {
		t.Fatalf("prod roomserver must not use deprecated AUTHORITY_SERVICE_INSTANCE_ID, got %q", old)
	}
	for _, serviceName := range []string{"redis", "nats", "roomservice", "timelineservice"} {
		if !dependsOnService(prodAuthority.DependsOn, serviceName) {
			t.Fatalf("prod authority service must depend on %s", serviceName)
		}
	}
	if mode := envString(prodAuthority.Environment, "ROOM_SERVICE_MODE"); mode != "rpc" {
		t.Fatalf("prod authority service ROOM_SERVICE_MODE = %q, want rpc", mode)
	}
	if addr := envString(prodAuthority.Environment, "ROOM_SERVICE_ADDR"); !strings.Contains(addr, "http://roomservice:8090") {
		t.Fatalf("prod authority service ROOM_SERVICE_ADDR = %q, want roomservice URL", addr)
	}
	if mode := envString(prodAuthority.Environment, "TIMELINE_SERVICE_MODE"); mode != "rpc" {
		t.Fatalf("prod authority service TIMELINE_SERVICE_MODE = %q, want rpc", mode)
	}
	if addr := envString(prodAuthority.Environment, "TIMELINE_SERVICE_ADDR"); !strings.Contains(addr, "http://timelineservice:8090") {
		t.Fatalf("prod authority service TIMELINE_SERVICE_ADDR = %q, want timelineservice URL", addr)
	}
	if databaseURL := envString(prodAuthority.Environment, "ROOM_DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod authority service must not receive direct ROOM_DATABASE_URL, got %q", databaseURL)
	}
	if databaseURL := envString(prodAuthority.Environment, "DATABASE_URL"); databaseURL != "" {
		t.Fatalf("prod authority service must not receive direct DATABASE_URL, got %q", databaseURL)
	}
}

func TestNginxRoutesRESTToAPIGatewayAndWebSocketToRoomserver(t *testing.T) {
	content, err := os.ReadFile("../../deploy/nginx/default.conf")
	if err != nil {
		t.Fatalf("read nginx config: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"upstream watch_together_api",
		"server apigateway:8080;",
		"upstream watch_together_ws",
		"server roomserver:8080;",
		"location = /healthz",
		"location = /readyz",
		"location = /metrics",
		"location /auth/",
		"proxy_pass http://watch_together_api;",
		"location /ws",
		"proxy_pass http://watch_together_ws;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected nginx config to contain %q", expected)
		}
	}
	if strings.Contains(text, "roomserver-rolling:8080") {
		t.Fatalf("default nginx config must not depend on smoke-only roomserver-rolling backend")
	}
}

func TestRollingNginxRoutesRESTToAPIGatewayAndWebSocketToBothRoomservers(t *testing.T) {
	content, err := os.ReadFile("../../deploy/nginx/default.rolling.conf")
	if err != nil {
		t.Fatalf("read rolling nginx config: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"upstream watch_together_api",
		"server apigateway:8080;",
		"upstream watch_together_ws",
		"server roomserver:8080",
		"server roomserver-rolling:8080",
		"location /auth/",
		"location /rooms",
		"proxy_pass http://watch_together_api;",
		"location /ws",
		"proxy_pass http://watch_together_ws;",
		"proxy_next_upstream_tries 2;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected rolling nginx config to contain %q", expected)
		}
	}
}

func TestComposeKafkaUsesApacheImageBaseline(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "local compose", path: "../../compose.yaml"},
		{name: "prod compose", path: "../../compose.prod.yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			compose := readCompose(t, tc.path)
			kafka := requireComposeService(t, compose, "kafka")
			if kafka.Image != "${KAFKA_IMAGE:-apache/kafka:3.7.0}" {
				t.Fatalf("kafka image = %q, want Apache Kafka image override baseline", kafka.Image)
			}
			if kafka.User != "${KAFKA_USER:-0:0}" {
				t.Fatalf("kafka user = %q, want root default so Apache Kafka can format the named KRaft volume", kafka.User)
			}
			for key := range kafka.Environment {
				if strings.HasPrefix(key, "KAFKA_CFG_") || key == "ALLOW_PLAINTEXT_LISTENER" {
					t.Fatalf("kafka environment contains Bitnami-only key %q", key)
				}
			}
			for _, expected := range []struct {
				key   string
				value string
			}{
				{key: "KAFKA_PROCESS_ROLES", value: "broker,controller"},
				{key: "KAFKA_CONTROLLER_QUORUM_VOTERS", value: "1@kafka:9093"},
				{key: "KAFKA_ADVERTISED_LISTENERS", value: "PLAINTEXT://kafka:9092"},
				{key: "KAFKA_LOG_DIRS", value: "/tmp/kraft-combined-logs"},
				{key: "CLUSTER_ID", value: "${KAFKA_CLUSTER_ID:-5L6g3nShT-eMCtK--X86sw}"},
			} {
				if got := envString(kafka.Environment, expected.key); got != expected.value {
					t.Fatalf("kafka %s = %q, want %q", expected.key, got, expected.value)
				}
			}
			if !containsContext(kafka.Volumes, "kafka_data:/tmp/kraft-combined-logs") {
				t.Fatalf("kafka volumes = %v, want Apache KRaft log volume", kafka.Volumes)
			}
			if containsContext(kafka.Volumes, "kafka_data:/bitnami/kafka") {
				t.Fatalf("kafka volumes still include Bitnami path: %v", kafka.Volumes)
			}
			healthcheck := fmt.Sprint(kafka.Healthcheck["test"])
			if !strings.Contains(healthcheck, "/opt/kafka/bin/kafka-topics.sh") || !strings.Contains(healthcheck, "--bootstrap-server localhost:9092 --list") {
				t.Fatalf("kafka healthcheck = %q, want Apache kafka-topics readiness probe", healthcheck)
			}
		})
	}
}

func TestServiceImageAllowsComposeCommandToSelectRoleBinary(t *testing.T) {
	content, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	text := string(content)
	if strings.Contains(text, `ENTRYPOINT ["/app/roomserver"]`) {
		t.Fatalf("service image must not pin roomserver as ENTRYPOINT; compose command must select apigateway and worker binaries")
	}
	if !strings.Contains(text, `CMD ["/app/roomserver"]`) {
		t.Fatalf("service image should keep roomserver as the default CMD for bare image runs")
	}
}

func TestStoreSQLWritesStayInsideOwnerBoundary(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	for fileName, contextName := range registry.StoreFiles {
		content := rawStringContent(readStoreFile(t, fileName))
		for _, table := range writeTables(content) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("%s writes table %s but the table has no owner", fileName, table)
			}
			if !containsContext(ownership.Writers, contextName) {
				t.Fatalf(
					"%s context %q writes table %s owned by %q; allowed writers: %v",
					fileName,
					contextName,
					table,
					ownership.Owner,
					ownership.Writers,
				)
			}
		}
	}
}

func TestCrossContextSQLReadsAreRegistered(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	for fileName, contextName := range registry.StoreFiles {
		content := rawStringContent(readStoreFile(t, fileName))
		for _, table := range readTables(content) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("%s reads table %s but the table has no owner", fileName, table)
			}
			if contextName == ownership.Owner {
				continue
			}
			if !containsContext(ownership.Readers, contextName) {
				t.Fatalf(
					"%s context %q reads table %s owned by %q without registry permission; allowed readers: %v",
					fileName,
					contextName,
					table,
					ownership.Owner,
					ownership.Readers,
				)
			}
			if !hasCrossContextAccess(registry.CrossContextAccess, contextName, ownership.Owner, fileName, table, "read") {
				t.Fatalf("%s context %q reads cross-context table %s without cross_context_access entry", fileName, contextName, table)
			}
		}
	}
}

func TestHomeCompositionDoesNotUseSQLReadModel(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	if _, ok := registry.StoreFiles["home_postgres.go"]; ok {
		t.Fatalf("home_postgres.go must not be registered after home composition moved to service ports")
	}
	for _, access := range registry.CrossContextAccess {
		if access.Caller == "home-composition" {
			t.Fatalf("home-composition must not keep direct SQL cross-context access: %+v", access)
		}
	}
	if _, err := os.Stat("home_postgres.go"); !os.IsNotExist(err) {
		t.Fatalf("home_postgres.go must be removed, stat err=%v", err)
	}
}

func TestMigrationCreatedTablesDeclareOwners(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			if _, ok := registry.Tables[table]; !ok {
				t.Fatalf("migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
		}
	}
}

func TestMainMigrationsDoNotReferenceOwnerTables(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		content := string(contentBytes)
		for table := range registry.Tables {
			pattern := regexp.MustCompile(`(?i)(^|[^a-z0-9_])` + regexp.QuoteMeta(table) + `([^a-z0-9_]|$)`)
			if pattern.MatchString(content) {
				t.Fatalf("main migration %s references owner table %s; owner schemas must live only in owner migration directories", filepath.Base(file), table)
			}
		}
	}
}

func TestIdentityMigrationCreatedTablesDeclareIdentityOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../identity_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob identity migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected identity migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read identity migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("identity migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "identity" {
				t.Fatalf("identity migration %s creates table %s owned by %q, want identity", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func TestMediaMigrationCreatedTablesDeclareMediaOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../media_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob media migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected media migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read media migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("media migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "media" {
				t.Fatalf("media migration %s creates table %s owned by %q, want media", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func TestTimelineMigrationCreatedTablesDeclareTimelineOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../timeline_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob timeline migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected timeline migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read timeline migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("timeline migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "timeline" {
				t.Fatalf("timeline migration %s creates table %s owned by %q, want timeline", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func TestProgressMigrationCreatedTablesDeclareProgressOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../progress_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob progress migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected progress migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read progress migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("progress migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "progress" {
				t.Fatalf("progress migration %s creates table %s owned by %q, want progress", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func TestRoomMigrationCreatedTablesDeclareRoomOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../room_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob room migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected room migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read room migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("room migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "room-session" {
				t.Fatalf("room migration %s creates table %s owned by %q, want room-session", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func loadOwnershipRegistry(t *testing.T) ownershipRegistry {
	t.Helper()
	content, err := os.ReadFile("db_ownership.yaml")
	if err != nil {
		t.Fatalf("read ownership registry: %v", err)
	}
	var registry ownershipRegistry
	if err := yaml.Unmarshal(content, &registry); err != nil {
		t.Fatalf("parse ownership registry: %v", err)
	}
	if registry.Version != 1 {
		t.Fatalf("expected ownership registry version 1, got %d", registry.Version)
	}
	if len(registry.Tables) == 0 {
		t.Fatalf("expected ownership registry tables")
	}
	return registry
}

func readCompose(t *testing.T, path string) composeDocument {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read compose file %s: %v", path, err)
	}
	var document composeDocument
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse compose file %s: %v", path, err)
	}
	if len(document.Services) == 0 {
		t.Fatalf("expected compose file %s to contain services", path)
	}
	return document
}

func requireComposeService(t *testing.T, document composeDocument, name string) composeService {
	t.Helper()
	service, ok := document.Services[name]
	if !ok {
		t.Fatalf("expected compose service %q", name)
	}
	return service
}

func readStoreFile(t *testing.T, fileName string) string {
	t.Helper()
	content, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("read store file %s: %v", fileName, err)
	}
	return string(content)
}

func writeTables(content string) []string {
	tables := uniqueMatches(content, regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)\s*\(`))
	tables = append(tables, uniqueMatches(content, regexp.MustCompile(`(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET\b`))...)
	tables = append(tables, uniqueMatches(content, regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([a-z_][a-z0-9_]*)\b`))...)
	return uniqueStrings(tables)
}

func readTables(content string) []string {
	return uniqueMatches(content, regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)`))
}

func createdTables(content string) []string {
	return uniqueMatches(content, regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`))
}

func envString(environment map[string]any, name string) string {
	if environment == nil {
		return ""
	}
	value, ok := environment[name]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(toString(value))
}

func dependsOnService(dependsOn any, service string) bool {
	switch value := dependsOn.(type) {
	case []any:
		for _, item := range value {
			if toString(item) == service {
				return true
			}
		}
	case map[string]any:
		_, ok := value[service]
		return ok
	}
	return false
}

func toString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func uniqueMatches(content string, expression *regexp.Regexp) []string {
	seen := map[string]bool{}
	var tables []string
	for _, match := range expression.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		table := strings.ToLower(strings.TrimSpace(match[1]))
		if table == "" || seen[table] {
			continue
		}
		seen[table] = true
		tables = append(tables, table)
	}
	return tables
}

func rawStringContent(content string) string {
	matches := regexp.MustCompile("(?s)`([^`]*)`").FindAllStringSubmatch(content, -1)
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			parts = append(parts, match[1])
		}
	}
	return strings.Join(parts, "\n")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func containsContext(contexts []string, contextName string) bool {
	for _, candidate := range contexts {
		if candidate == contextName {
			return true
		}
	}
	return false
}

func hasCrossContextAccess(
	entries []crossContextAccess,
	caller string,
	owner string,
	path string,
	table string,
	access string,
) bool {
	for _, entry := range entries {
		if entry.Caller != caller || entry.Owner != owner || entry.Path != path || entry.Access != access {
			continue
		}
		for _, entryTable := range entry.Tables {
			if entryTable == table {
				return strings.TrimSpace(entry.Reason) != ""
			}
		}
	}
	return false
}
