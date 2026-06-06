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
	if cfg.InstanceID != "" {
		t.Fatalf("expected default instance id to be empty, got %q", cfg.InstanceID)
	}
	if cfg.RoomRuntimeMode != "local_process" {
		t.Fatalf("expected default room runtime mode local_process, got %q", cfg.RoomRuntimeMode)
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
	if cfg.WebSocket.SeekMinIntervalMs != 250 {
		t.Fatalf("expected default seek min interval 250ms, got %d", cfg.WebSocket.SeekMinIntervalMs)
	}
	if cfg.WebSocket.ControlIdempotencyTTLms != 600000 ||
		cfg.WebSocket.PresenceLeaseTTLms != 45000 ||
		cfg.WebSocket.PresenceRefreshIntervalMs != 15000 {
		t.Fatalf("unexpected default websocket hardening config: %+v", cfg.WebSocket)
	}
	if cfg.WebSocket.CrossInstanceBroadcast {
		t.Fatalf("expected cross-instance broadcast disabled by default")
	}
	if cfg.WebSocket.EventBus != "nats_core" {
		t.Fatalf("expected default websocket event bus nats_core, got %q", cfg.WebSocket.EventBus)
	}
	if cfg.NATS.URL != "nats://127.0.0.1:4222" {
		t.Fatalf("expected default nats url, got %q", cfg.NATS.URL)
	}
	if cfg.NATS.Name != "watch-together-roomserver" {
		t.Fatalf("expected default nats name, got %q", cfg.NATS.Name)
	}
	if cfg.NATS.SubjectRoomBroadcast != "wt.room.broadcast.v1" {
		t.Fatalf("expected default room broadcast subject, got %q", cfg.NATS.SubjectRoomBroadcast)
	}
	if cfg.NATS.SubjectRoomControl != "wt.room.control.v1" {
		t.Fatalf("expected default room control subject, got %q", cfg.NATS.SubjectRoomControl)
	}
	if len(cfg.Kafka.Brokers) != 0 {
		t.Fatalf("expected default kafka brokers to be empty, got %+v", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.ClientID != "watch-together-roomserver" {
		t.Fatalf("expected default kafka client id, got %q", cfg.Kafka.ClientID)
	}
	if cfg.Kafka.TopicRoomTimeline != "wt.room.timeline.v1" ||
		cfg.Kafka.TopicRoomControlResult != "wt.room.control_result.v1" ||
		cfg.Kafka.TopicRoomMembership != "wt.room.membership.v1" {
		t.Fatalf("unexpected default kafka topics: %+v", cfg.Kafka)
	}
	if cfg.OutboxWorker.BatchSize != 50 || cfg.OutboxWorker.PollIntervalMs != 1000 {
		t.Fatalf("unexpected default outbox worker config: %+v", cfg.OutboxWorker)
	}
	if cfg.AuthorityRecovery.RenewIntervalMs != 10000 ||
		cfg.AuthorityRecovery.TakeoverScanIntervalMs != 30000 ||
		cfg.AuthorityRecovery.RecoveryTimeoutMs != 5000 ||
		cfg.AuthorityRecovery.KafkaReplayTimeoutMs != 1000 {
		t.Fatalf("unexpected default authority recovery config: %+v", cfg.AuthorityRecovery)
	}
	if !cfg.Observability.MetricsEnabled ||
		cfg.Observability.MetricsAddr != "" ||
		cfg.Observability.MetricsPath != "/metrics" ||
		cfg.Observability.ReadinessPath != "/readyz" {
		t.Fatalf("unexpected default observability config: %+v", cfg.Observability)
	}
	if cfg.Service.Name != "watch-together-roomserver" || cfg.Service.Version != "dev" {
		t.Fatalf("unexpected default service config: %+v", cfg.Service)
	}
	if cfg.InternalRPC.Enabled ||
		cfg.InternalRPC.Addr != ":8090" ||
		cfg.InternalRPC.PathPrefix != "/internal.rpc" ||
		cfg.InternalRPC.TimeoutMs != 1000 {
		t.Fatalf("unexpected default internal rpc config: %+v", cfg.InternalRPC)
	}
	if cfg.ServiceClients.DiscoveryMode != "static" ||
		cfg.ServiceClients.MediaMode != "local" ||
		cfg.ServiceClients.TimelineMode != "local" {
		t.Fatalf("unexpected default service client config: %+v", cfg.ServiceClients)
	}
	if cfg.Telemetry.TracingEnabled ||
		cfg.Telemetry.ServiceName != "watch-together-roomserver" ||
		cfg.Telemetry.TraceSampleRatio != 0.1 {
		t.Fatalf("unexpected default telemetry config: %+v", cfg.Telemetry)
	}
	if cfg.Media.URLTTLSeconds != 7200 {
		t.Fatalf("expected default media playback url ttl 7200s, got %d", cfg.Media.URLTTLSeconds)
	}
	if cfg.Media.DeliveryMode != "signed_redirect" {
		t.Fatalf("expected default media delivery mode signed_redirect, got %q", cfg.Media.DeliveryMode)
	}
}

func TestLoadServerRuntimeConfigLoadsRuntimeBoundarySettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"SERVER_INSTANCE_ID=roomserver-a\nROOM_RUNTIME_MODE=local_process\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.InstanceID != "roomserver-a" {
		t.Fatalf("expected instance id roomserver-a, got %q", cfg.InstanceID)
	}
	if cfg.RoomRuntimeMode != "local_process" {
		t.Fatalf("expected room runtime mode local_process, got %q", cfg.RoomRuntimeMode)
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

func TestLoadServerRuntimeConfigLoadsRedisSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"REDIS_ADDR=127.0.0.1:6380\nREDIS_USERNAME=default\nREDIS_PASSWORD=secret\nREDIS_DB=2\nREDIS_TLS_ENABLED=true\nREDIS_REQUIRED=true\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.Redis.Addr != "127.0.0.1:6380" {
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
		"WS_BROADCAST_CONCURRENCY_LIMIT=128\nWS_BROADCAST_TIMEOUT_MS=7000\nWS_BROADCAST_ENQUEUE_TIMEOUT_MS=1500\nWS_CLIENT_OUTBOX_CAPACITY=32\nWS_MAX_CONNECTIONS=1000\nROOM_MAX_CLIENTS=25\nWS_SEEK_MIN_INTERVAL_MS=100\nCONTROL_IDEMPOTENCY_TTL_MS=120000\nPRESENCE_LEASE_TTL_MS=30000\nPRESENCE_REFRESH_INTERVAL_MS=10000\nWS_CROSS_INSTANCE_BROADCAST_ENABLED=true\nWS_EVENT_BUS=nats_core\n",
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
	if cfg.WebSocket.SeekMinIntervalMs != 100 {
		t.Fatalf("expected seek min interval 100ms, got %d", cfg.WebSocket.SeekMinIntervalMs)
	}
	if cfg.WebSocket.ControlIdempotencyTTLms != 120000 ||
		cfg.WebSocket.PresenceLeaseTTLms != 30000 ||
		cfg.WebSocket.PresenceRefreshIntervalMs != 10000 {
		t.Fatalf("unexpected websocket hardening config: %+v", cfg.WebSocket)
	}
	if !cfg.WebSocket.CrossInstanceBroadcast {
		t.Fatalf("expected cross-instance broadcast enabled")
	}
	if cfg.WebSocket.EventBus != "nats_core" {
		t.Fatalf("expected event bus nats_core, got %q", cfg.WebSocket.EventBus)
	}
}

func TestLoadServerRuntimeConfigLoadsNATSSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"NATS_URL=nats://nats:4222\nNATS_NAME=roomserver-a\nNATS_SUBJECT_ROOM_BROADCAST=wt.room.broadcast.test\nNATS_SUBJECT_ROOM_CONTROL=wt.room.control.test\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.NATS.URL != "nats://nats:4222" {
		t.Fatalf("expected nats url, got %q", cfg.NATS.URL)
	}
	if cfg.NATS.Name != "roomserver-a" {
		t.Fatalf("expected nats name, got %q", cfg.NATS.Name)
	}
	if cfg.NATS.SubjectRoomBroadcast != "wt.room.broadcast.test" {
		t.Fatalf("expected room broadcast subject, got %q", cfg.NATS.SubjectRoomBroadcast)
	}
	if cfg.NATS.SubjectRoomControl != "wt.room.control.test" {
		t.Fatalf("expected room control subject, got %q", cfg.NATS.SubjectRoomControl)
	}
}

func TestLoadServerRuntimeConfigLoadsKafkaAndOutboxSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"KAFKA_BROKERS=kafka:9092,kafka-2:9092\nKAFKA_CLIENT_ID=roomserver-a\nKAFKA_TOPIC_ROOM_TIMELINE=timeline.test\nKAFKA_TOPIC_ROOM_CONTROL_RESULT=control.test\nKAFKA_TOPIC_ROOM_MEMBERSHIP=membership.test\nKAFKA_DERIVED_CONSUMER_GROUP_ID=derived-test\nKAFKA_DERIVED_WORKER_POLL_INTERVAL_MS=2500\nOUTBOX_WORKER_BATCH_SIZE=25\nOUTBOX_WORKER_POLL_INTERVAL_MS=500\nAUTHORITY_RENEW_INTERVAL_MS=7000\nAUTHORITY_TAKEOVER_SCAN_INTERVAL_MS=17000\nAUTHORITY_RECOVERY_TIMEOUT_MS=4500\nKAFKA_REPLAY_TIMEOUT_MS=800\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if len(cfg.Kafka.Brokers) != 2 || cfg.Kafka.Brokers[0] != "kafka:9092" || cfg.Kafka.Brokers[1] != "kafka-2:9092" {
		t.Fatalf("unexpected kafka brokers: %+v", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.ClientID != "roomserver-a" ||
		cfg.Kafka.TopicRoomTimeline != "timeline.test" ||
		cfg.Kafka.TopicRoomControlResult != "control.test" ||
		cfg.Kafka.TopicRoomMembership != "membership.test" ||
		cfg.Kafka.DerivedConsumerGroupID != "derived-test" ||
		cfg.Kafka.DerivedWorkerPollInterval != 2500 {
		t.Fatalf("unexpected kafka config: %+v", cfg.Kafka)
	}
	if cfg.OutboxWorker.BatchSize != 25 || cfg.OutboxWorker.PollIntervalMs != 500 {
		t.Fatalf("unexpected outbox worker config: %+v", cfg.OutboxWorker)
	}
	if cfg.AuthorityRecovery.RenewIntervalMs != 7000 ||
		cfg.AuthorityRecovery.TakeoverScanIntervalMs != 17000 ||
		cfg.AuthorityRecovery.RecoveryTimeoutMs != 4500 ||
		cfg.AuthorityRecovery.KafkaReplayTimeoutMs != 800 {
		t.Fatalf("unexpected authority recovery config: %+v", cfg.AuthorityRecovery)
	}
}

func TestLoadServerRuntimeConfigLoadsObservabilitySettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"METRICS_ENABLED=false\nMETRICS_ADDR=:9091\nMETRICS_PATH=/internal/metrics\nREADINESS_PATH=/internal/readyz\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.Observability.MetricsEnabled {
		t.Fatalf("expected metrics disabled")
	}
	if cfg.Observability.MetricsAddr != ":9091" ||
		cfg.Observability.MetricsPath != "/internal/metrics" ||
		cfg.Observability.ReadinessPath != "/internal/readyz" {
		t.Fatalf("unexpected observability config: %+v", cfg.Observability)
	}
}

func TestLoadServerRuntimeConfigLoadsServiceFoundationSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"SERVICE_NAME=watch-together-roomserver-a\nSERVICE_VERSION=2026.06\nINTERNAL_RPC_ENABLED=true\nINTERNAL_RPC_ADDR=:8095\nINTERNAL_RPC_PATH_PREFIX=/internal.test\nINTERNAL_RPC_TIMEOUT_MS=2500\nINTERNAL_RPC_AUTH_TOKEN=secret\nSERVICE_DISCOVERY_MODE=static\nMEDIA_SERVICE_MODE=rpc\nMEDIA_SERVICE_ADDR=http://media:8090\nTIMELINE_SERVICE_MODE=rpc\nTIMELINE_SERVICE_ADDR=http://timeline:8090\nOTEL_TRACING_ENABLED=true\nOTEL_SERVICE_NAME=roomserver-traced\nOTEL_EXPORTER_OTLP_ENDPOINT=otel:4318\nOTEL_TRACE_SAMPLE_RATIO=0.25\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}
	if cfg.Service.Name != "watch-together-roomserver-a" || cfg.Service.Version != "2026.06" {
		t.Fatalf("unexpected service config: %+v", cfg.Service)
	}
	if !cfg.InternalRPC.Enabled ||
		cfg.InternalRPC.Addr != ":8095" ||
		cfg.InternalRPC.PathPrefix != "/internal.test" ||
		cfg.InternalRPC.TimeoutMs != 2500 ||
		cfg.InternalRPC.AuthToken != "secret" {
		t.Fatalf("unexpected internal rpc config: %+v", cfg.InternalRPC)
	}
	if cfg.ServiceClients.MediaMode != "rpc" ||
		cfg.ServiceClients.MediaAddr != "http://media:8090" ||
		cfg.ServiceClients.TimelineMode != "rpc" ||
		cfg.ServiceClients.TimelineAddr != "http://timeline:8090" {
		t.Fatalf("unexpected service clients config: %+v", cfg.ServiceClients)
	}
	if !cfg.Telemetry.TracingEnabled ||
		cfg.Telemetry.ServiceName != "roomserver-traced" ||
		cfg.Telemetry.OTLPEndpoint != "otel:4318" ||
		cfg.Telemetry.TraceSampleRatio != 0.25 {
		t.Fatalf("unexpected telemetry config: %+v", cfg.Telemetry)
	}
}

func TestLoadServerRuntimeConfigRejectsInvalidServiceFoundationSettings(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "media rpc missing addr", content: "MEDIA_SERVICE_MODE=rpc\n"},
		{name: "timeline rpc missing addr", content: "TIMELINE_SERVICE_MODE=rpc\n"},
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

func TestLoadServerRuntimeConfigLoadsMediaPlaybackSettings(t *testing.T) {
	configDir := t.TempDir()
	mustWriteConfigFile(
		t,
		filepath.Join(configDir, ".env"),
		"MEDIA_DELIVERY_MODE=minio_presign\nMEDIA_PLAYBACK_SIGNING_SECRET=media-secret\nMEDIA_PLAYBACK_URL_TTL_SECONDS=900\nMEDIA_PUBLIC_BASE_URL=http://127.0.0.1:9100/watch-together-media\nMEDIA_INTERNAL_BASE_URL=http://minio:9000/watch-together-media\nMEDIA_STORAGE_ENDPOINT=http://127.0.0.1:9100\nMEDIA_STORAGE_BUCKET=watch-together-media\nMEDIA_STORAGE_REGION=auto\nMEDIA_STORAGE_ACCESS_KEY_ID=minioadmin\nMEDIA_STORAGE_SECRET_ACCESS_KEY=miniosecret\nMEDIA_STORAGE_FORCE_PATH_STYLE=false\n",
	)

	cfg, err := LoadServerRuntimeConfig(configDir)
	if err != nil {
		t.Fatalf("load runtime config: %v", err)
	}

	if cfg.Media.DeliveryMode != "minio_presign" {
		t.Fatalf("expected media delivery mode minio_presign, got %q", cfg.Media.DeliveryMode)
	}
	if cfg.Media.SigningSecret != "media-secret" {
		t.Fatalf("expected media signing secret, got %q", cfg.Media.SigningSecret)
	}
	if cfg.Media.URLTTLSeconds != 900 {
		t.Fatalf("expected media playback ttl 900s, got %d", cfg.Media.URLTTLSeconds)
	}
	if cfg.Media.PublicBaseURL != "http://127.0.0.1:9100/watch-together-media" {
		t.Fatalf("expected media public base url, got %q", cfg.Media.PublicBaseURL)
	}
	if cfg.Media.InternalBaseURL != "http://minio:9000/watch-together-media" {
		t.Fatalf("expected media internal base url, got %q", cfg.Media.InternalBaseURL)
	}
	if cfg.Media.StorageEndpoint != "http://127.0.0.1:9100" {
		t.Fatalf("expected media storage endpoint, got %q", cfg.Media.StorageEndpoint)
	}
	if cfg.Media.StorageBucket != "watch-together-media" {
		t.Fatalf("expected media storage bucket, got %q", cfg.Media.StorageBucket)
	}
	if cfg.Media.StorageRegion != "auto" {
		t.Fatalf("expected media storage region, got %q", cfg.Media.StorageRegion)
	}
	if cfg.Media.StorageAccessKeyID != "minioadmin" {
		t.Fatalf("expected media storage access key, got %q", cfg.Media.StorageAccessKeyID)
	}
	if cfg.Media.StorageSecretAccessKey != "miniosecret" {
		t.Fatalf("expected media storage secret key, got %q", cfg.Media.StorageSecretAccessKey)
	}
	if cfg.Media.StorageForcePathStyle {
		t.Fatalf("expected media storage force path style false")
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
