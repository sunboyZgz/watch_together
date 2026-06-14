package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMediactlConfigPrefersEnvOverLocalFiles(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "MEDIA_STORAGE_DRIVER=local\nMEDIA_LOCAL_ROOT=../media/base\nDATABASE_URL=postgres://base\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.local"), "MEDIA_STORAGE_DRIVER=minio\nMEDIA_LOCAL_ROOT=../media/local\nDATABASE_URL=postgres://local\n")

	t.Setenv("MEDIA_STORAGE_DRIVER", "s3")
	t.Setenv("DATABASE_URL", "postgres://env")

	cfg, err := LoadMediactlConfig(configDir)
	if err != nil {
		t.Fatalf("load mediactl config: %v", err)
	}

	if cfg.Storage.Driver != "s3" {
		t.Fatalf("expected env override for storage driver, got %q", cfg.Storage.Driver)
	}
	if cfg.Storage.LocalRoot != "../media/local" {
		t.Fatalf("expected .env.local override for local root, got %q", cfg.Storage.LocalRoot)
	}
	if cfg.DatabaseURL != "postgres://env" {
		t.Fatalf("expected env override for database url, got %q", cfg.DatabaseURL)
	}
}

func TestLoadServerRuntimeConfigAcceptsDistributedAuthorityWithDependencies(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"ROOM_RUNTIME_MODE=distributed_authority\nSERVER_INSTANCE_ID=roomserver-a\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nDATABASE_URL=postgres://app:app@postgres:5432/app\nREDIS_ADDR=redis:6380\nNATS_URL=nats://nats:4222\nKAFKA_BROKERS=kafka:9092, kafka-2:9092\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.RoomRuntimeMode != "distributed_authority" {
		t.Fatalf("expected distributed_authority, got %q", cfg.RoomRuntimeMode)
	}
	if len(cfg.Kafka.Brokers) != 2 || cfg.Kafka.Brokers[0] != "kafka:9092" || cfg.Kafka.Brokers[1] != "kafka-2:9092" {
		t.Fatalf("unexpected kafka brokers: %+v", cfg.Kafka.Brokers)
	}
}

func TestLoadServerRuntimeConfigLoadsMediaDatabaseURL(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "MEDIA_DATABASE_URL=postgres://media-db\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.MediaDatabaseURL != "postgres://media-db" {
		t.Fatalf("expected media database url, got %q", cfg.MediaDatabaseURL)
	}
}

func TestLoadServerRuntimeConfigLoadsIdentityDatabaseURL(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "IDENTITY_DATABASE_URL=postgres://identity-db\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.IdentityDatabaseURL != "postgres://identity-db" {
		t.Fatalf("expected identity database url, got %q", cfg.IdentityDatabaseURL)
	}
}

func TestLoadServerRuntimeConfigLoadsRoomDatabaseURL(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "ROOM_DATABASE_URL=postgres://room-db\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.RoomDatabaseURL != "postgres://room-db" {
		t.Fatalf("expected room database url, got %q", cfg.RoomDatabaseURL)
	}
}

func TestLoadServerRuntimeConfigLoadsTimelineDatabaseURL(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "TIMELINE_DATABASE_URL=postgres://timeline-db\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.TimelineDatabaseURL != "postgres://timeline-db" {
		t.Fatalf("expected timeline database url, got %q", cfg.TimelineDatabaseURL)
	}
}

func TestLoadServerRuntimeConfigLoadsProgressDatabaseURL(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "PROGRESS_DATABASE_URL=postgres://progress-db\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.ProgressDatabaseURL != "postgres://progress-db" {
		t.Fatalf("expected progress database url, got %q", cfg.ProgressDatabaseURL)
	}
}

func TestLoadRoomserverRuntimeConfigRejectsOwnedDatabaseURLInRPCMode(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantMessage string
	}{
		{
			name:        "identity",
			content:     "IDENTITY_SERVICE_MODE=rpc\nIDENTITY_SERVICE_ADDR=http://identityservice:8090\nIDENTITY_DATABASE_URL=postgres://identity-db\n",
			wantMessage: "IDENTITY_DATABASE_URL",
		},
		{
			name:        "room",
			content:     "ROOM_SERVICE_MODE=rpc\nROOM_SERVICE_ADDR=http://roomservice:8090\nROOM_DATABASE_URL=postgres://room-db\n",
			wantMessage: "ROOM_DATABASE_URL",
		},
		{
			name:        "media",
			content:     "MEDIA_SERVICE_MODE=rpc\nMEDIA_SERVICE_ADDR=http://mediaservice:8090\nMEDIA_DATABASE_URL=postgres://media-db\n",
			wantMessage: "MEDIA_DATABASE_URL",
		},
		{
			name:        "timeline",
			content:     "TIMELINE_SERVICE_MODE=rpc\nTIMELINE_SERVICE_ADDR=http://timelineservice:8090\nTIMELINE_DATABASE_URL=postgres://timeline-db\n",
			wantMessage: "TIMELINE_DATABASE_URL",
		},
		{
			name:        "progress",
			content:     "PROGRESS_SERVICE_MODE=rpc\nPROGRESS_SERVICE_ADDR=http://progressservice:8090\nPROGRESS_DATABASE_URL=postgres://progress-db\n",
			wantMessage: "PROGRESS_DATABASE_URL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			mustWriteConfigFile(t, filepath.Join(configDir, ".env"), tc.content)

			if _, err := LoadRoomserverRuntimeConfig(configDir); err == nil || !strings.Contains(err.Error(), tc.wantMessage) {
				t.Fatalf("expected roomserver config to reject %s in rpc mode, got %v", tc.wantMessage, err)
			}
			if _, err := LoadServerRuntimeConfig(configDir); err != nil {
				t.Fatalf("expected generic service config to keep accepting owned database url: %v", err)
			}
		})
	}
}

func TestLoadServerRuntimeConfigDistributedAuthorityRequiresDependencies(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "missing instance id",
			content: "ROOM_RUNTIME_MODE=distributed_authority\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nDATABASE_URL=postgres://db\nREDIS_ADDR=redis:6380\nNATS_URL=nats://nats:4222\nKAFKA_BROKERS=kafka:9092\n",
		},
		{
			name:    "cross instance broadcast disabled",
			content: "ROOM_RUNTIME_MODE=distributed_authority\nSERVER_INSTANCE_ID=roomserver-a\nDATABASE_URL=postgres://db\nREDIS_ADDR=redis:6380\nNATS_URL=nats://nats:4222\nKAFKA_BROKERS=kafka:9092\n",
		},
		{
			name:    "missing database",
			content: "ROOM_RUNTIME_MODE=distributed_authority\nSERVER_INSTANCE_ID=roomserver-a\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nREDIS_ADDR=redis:6380\nNATS_URL=nats://nats:4222\nKAFKA_BROKERS=kafka:9092\n",
		},
		{
			name:    "missing redis",
			content: "ROOM_RUNTIME_MODE=distributed_authority\nSERVER_INSTANCE_ID=roomserver-a\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nDATABASE_URL=postgres://db\nNATS_URL=nats://nats:4222\nKAFKA_BROKERS=kafka:9092\n",
		},
		{
			name:    "missing kafka",
			content: "ROOM_RUNTIME_MODE=distributed_authority\nSERVER_INSTANCE_ID=roomserver-a\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nDATABASE_URL=postgres://db\nREDIS_ADDR=redis:6380\nNATS_URL=nats://nats:4222\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			mustWriteConfigFile(t, filepath.Join(configDir, ".env"), tc.content)
			if _, err := LoadServerRuntimeConfig(configDir); err == nil {
				t.Fatalf("expected distributed_authority dependency validation to fail")
			}
		})
	}
}

func TestLoadServerRuntimeConfigRejectsUnsupportedRoomRuntimeMode(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "ROOM_RUNTIME_MODE=redis_authority\n")

	if _, err := LoadServerRuntimeConfig(configDir); err == nil {
		t.Fatalf("expected unsupported room runtime mode to fail config loading")
	}
}

func TestLoadServerRuntimeConfigRejectsUnsupportedEventBus(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "WS_EVENT_BUS=kafka\n")

	if _, err := LoadServerRuntimeConfig(configDir); err == nil {
		t.Fatalf("expected unsupported websocket event bus to fail config loading")
	}
}

func TestLoadServerRuntimeConfigSupportsAppEnvSpecificDebugSync(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "APP_ENV=prod\nDEBUG_SYNC=true\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.prod"), "DEBUG_SYNC=false\nLOG_LEVEL=info\n")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.AppEnv != "prod" {
		t.Fatalf("expected prod app env, got %q", cfg.AppEnv)
	}
	if cfg.DebugSync {
		t.Fatalf("expected .env.prod DEBUG_SYNC=false to disable debug sync")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected .env.prod LOG_LEVEL=info, got %q", cfg.LogLevel)
	}
}

func TestLoadServerRuntimeConfigEnvOverridesDebugSyncFiles(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "APP_ENV=prod\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.prod"), "DEBUG_SYNC=false\n")
	t.Setenv("DEBUG_SYNC", "true")

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if !cfg.DebugSync {
		t.Fatalf("expected DEBUG_SYNC=true env to override .env.prod")
	}
}

func TestLoadServerRuntimeConfigRejectsInvalidServiceFoundationSettings(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "media rpc missing addr", content: "MEDIA_SERVICE_MODE=rpc\n"},
		{name: "progress rpc missing addr", content: "PROGRESS_SERVICE_MODE=rpc\n"},
		{name: "home rpc missing addr", content: "HOME_SERVICE_MODE=rpc\n"},
		{name: "identity rpc missing addr", content: "IDENTITY_SERVICE_MODE=rpc\n"},
		{name: "room rpc missing addr", content: "ROOM_SERVICE_MODE=rpc\n"},
		{name: "timeline rpc missing addr", content: "TIMELINE_SERVICE_MODE=rpc\n"},
		{name: "authority rpc missing addr", content: "AUTHORITY_SERVICE_MODE=rpc\nAUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1\n"},
		{name: "authority rpc missing instance id", content: "AUTHORITY_SERVICE_MODE=rpc\nAUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090\n"},
		{name: "invalid discovery", content: "SERVICE_DISCOVERY_MODE=consul\n"},
		{name: "invalid sample ratio", content: "OTEL_TRACE_SAMPLE_RATIO=1.5\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			mustWriteConfigFile(t, filepath.Join(configDir, ".env"), tc.content)
			if _, err := LoadServerRuntimeConfig(configDir); err == nil {
				t.Fatalf("expected invalid service foundation config to fail")
			}
		})
	}
}

func TestLoadServerRuntimeConfigLoadsAuthorityLeaseInstanceID(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"AUTHORITY_SERVICE_MODE=rpc\nAUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090\nAUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.ServiceClients.AuthorityLeaseID != "roomauthorityservice-1" {
		t.Fatalf("expected authority lease instance id, got %q", cfg.ServiceClients.AuthorityLeaseID)
	}
}

func TestLoadRoomserverRuntimeConfigRequiresDistributedRuntimeForAuthorityRPC(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"AUTHORITY_SERVICE_MODE=rpc\nAUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090\nAUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1\n",
	)

	if _, err := LoadRoomserverRuntimeConfig(configDir); err == nil ||
		!strings.Contains(err.Error(), "AUTHORITY_SERVICE_MODE=rpc requires ROOM_RUNTIME_MODE=distributed_authority") {
		t.Fatalf("expected authority rpc to require distributed room runtime, got %v", err)
	}
}

func TestLoadServerRuntimeConfigDoesNotReadDeprecatedAuthorityServiceInstanceID(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"AUTHORITY_SERVICE_MODE=rpc\nAUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090\nAUTHORITY_SERVICE_INSTANCE_ID=roomauthorityservice-1\n",
	)

	if _, err := LoadServerRuntimeConfig(configDir); err == nil || !strings.Contains(err.Error(), "AUTHORITY_LEASE_INSTANCE_ID") {
		t.Fatalf("expected deprecated authority service instance id to be ignored, got %v", err)
	}
}

func TestLoadMediactlConfigSupportsAppEnvSpecificFiles(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "APP_ENV=prod\nMEDIA_STORAGE_DRIVER=local\nDATABASE_URL=postgres://base\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.prod"), "MEDIA_STORAGE_DRIVER=minio\nMEDIA_PUBLIC_BASE_URL=http://prod.example.com/media\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.local"), "MEDIA_STORAGE_BUCKET=generic-local-bucket\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.prod.local"), "MEDIA_STORAGE_BUCKET=prod-local-bucket\n")

	cfg, err := LoadMediactlConfig(configDir)
	if err != nil {
		t.Fatalf("load mediactl config: %v", err)
	}

	if cfg.Storage.Driver != "minio" {
		t.Fatalf("expected .env.prod override for storage driver, got %q", cfg.Storage.Driver)
	}
	if cfg.Storage.PublicBaseURL != "http://prod.example.com/media" {
		t.Fatalf("expected .env.prod override for public base url, got %q", cfg.Storage.PublicBaseURL)
	}
	if cfg.Storage.Bucket != "prod-local-bucket" {
		t.Fatalf("expected .env.prod.local override for bucket, got %q", cfg.Storage.Bucket)
	}
}

func TestLoadMediactlConfigAppEnvSpecificOverridesGenericLocal(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(t, filepath.Join(configDir, ".env"), "APP_ENV=prod\nMEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9000/media/tmp\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.local"), "MEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9100/watch-together-media\n")
	mustWriteConfigFile(t, filepath.Join(configDir, ".env.prod"), "MEDIA_PUBLIC_BASE_URL=http://106.12.35.52:9100/watch-together-media\n")

	cfg, err := LoadMediactlConfig(configDir)
	if err != nil {
		t.Fatalf("load mediactl config: %v", err)
	}

	if cfg.Storage.PublicBaseURL != "http://106.12.35.52:9100/watch-together-media" {
		t.Fatalf("expected env-specific config to override generic local config, got %q", cfg.Storage.PublicBaseURL)
	}
}

func mustWriteConfigFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file %q: %v", path, err)
	}
}
