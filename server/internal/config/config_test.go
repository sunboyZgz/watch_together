package config

import (
	"os"
	"path/filepath"
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

func TestLoadServerRuntimeConfigFallsBackToDefaults(t *testing.T) {
	cfg, err := LoadServerRuntimeConfig(t.TempDir())
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.AppEnv != "local" {
		t.Fatalf("expected default app env, got %q", cfg.AppEnv)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected default host, got %q", cfg.Host)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port, got %q", cfg.Port)
	}
	if !cfg.DebugSync {
		t.Fatalf("expected default debug sync to be true")
	}
	if cfg.WebSocket.BroadcastConcurrencyLimit != 64 {
		t.Fatalf("expected default broadcast concurrency 64, got %d", cfg.WebSocket.BroadcastConcurrencyLimit)
	}
	if cfg.WebSocket.BroadcastTimeoutMs != 5000 {
		t.Fatalf("expected default broadcast timeout 5000ms, got %d", cfg.WebSocket.BroadcastTimeoutMs)
	}
	if cfg.WebSocket.BroadcastEnqueueTimeoutMs != 3000 {
		t.Fatalf("expected default enqueue timeout 3000ms, got %d", cfg.WebSocket.BroadcastEnqueueTimeoutMs)
	}
	if cfg.WebSocket.ClientOutboxCapacity != 64 {
		t.Fatalf("expected default outbox capacity 64, got %d", cfg.WebSocket.ClientOutboxCapacity)
	}
	if cfg.WebSocket.MaxConnections != 0 {
		t.Fatalf("expected default max connections unlimited, got %d", cfg.WebSocket.MaxConnections)
	}
	if cfg.WebSocket.MaxRoomClients != 0 {
		t.Fatalf("expected default room max clients unlimited, got %d", cfg.WebSocket.MaxRoomClients)
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

func TestLoadServerRuntimeConfigLoadsRedisSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"REDIS_ADDR=127.0.0.1:6379\nREDIS_USERNAME=default\nREDIS_PASSWORD=secret\nREDIS_DB=2\nREDIS_TLS_ENABLED=true\nREDIS_REQUIRED=true\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Fatalf("expected redis addr, got %q", cfg.Redis.Addr)
	}
	if cfg.Redis.Username != "default" {
		t.Fatalf("expected redis username default, got %q", cfg.Redis.Username)
	}
	if cfg.Redis.Password != "secret" {
		t.Fatalf("expected redis password secret, got %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 2 {
		t.Fatalf("expected redis db 2, got %d", cfg.Redis.DB)
	}
	if !cfg.Redis.TLSEnabled {
		t.Fatalf("expected redis tls enabled")
	}
	if !cfg.Redis.Required {
		t.Fatalf("expected redis required")
	}
}

func TestLoadServerRuntimeConfigLoadsWebSocketRuntimeSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"WS_BROADCAST_CONCURRENCY_LIMIT=128\nWS_BROADCAST_TIMEOUT_MS=7000\nWS_BROADCAST_ENQUEUE_TIMEOUT_MS=1500\nWS_CLIENT_OUTBOX_CAPACITY=32\nWS_MAX_CONNECTIONS=1000\nROOM_MAX_CLIENTS=25\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.WebSocket.BroadcastConcurrencyLimit != 128 {
		t.Fatalf("expected broadcast concurrency 128, got %d", cfg.WebSocket.BroadcastConcurrencyLimit)
	}
	if cfg.WebSocket.BroadcastTimeoutMs != 7000 {
		t.Fatalf("expected broadcast timeout 7000ms, got %d", cfg.WebSocket.BroadcastTimeoutMs)
	}
	if cfg.WebSocket.BroadcastEnqueueTimeoutMs != 1500 {
		t.Fatalf("expected enqueue timeout 1500ms, got %d", cfg.WebSocket.BroadcastEnqueueTimeoutMs)
	}
	if cfg.WebSocket.ClientOutboxCapacity != 32 {
		t.Fatalf("expected outbox capacity 32, got %d", cfg.WebSocket.ClientOutboxCapacity)
	}
	if cfg.WebSocket.MaxConnections != 1000 {
		t.Fatalf("expected max connections 1000, got %d", cfg.WebSocket.MaxConnections)
	}
	if cfg.WebSocket.MaxRoomClients != 25 {
		t.Fatalf("expected room max clients 25, got %d", cfg.WebSocket.MaxRoomClients)
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
