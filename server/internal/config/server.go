package config

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	roomRuntimeModeLocalProcess         = "local_process"
	roomRuntimeModeDistributedAuthority = "distributed_authority"
)

const eventBusNATSCore = "nats_core"

type ServerRuntimeConfig struct {
	AppEnv              string
	Host                string
	Port                string
	LogLevel            string
	InstanceID          string
	RoomRuntimeMode     string
	DatabaseURL         string
	MediaDatabaseURL    string
	TimelineDatabaseURL string
	DebugSync           bool
	Auth                AuthConfig
	Redis               RedisConfig
	WebSocket           WebSocketConfig
	NATS                NATSConfig
	Kafka               KafkaConfig
	OutboxWorker        OutboxWorkerConfig
	AuthorityRecovery   AuthorityRecoveryConfig
	Observability       ObservabilityConfig
	Service             ServiceConfig
	InternalRPC         InternalRPCConfig
	ServiceClients      ServiceClientsConfig
	Telemetry           TelemetryConfig
	Media               MediaPlaybackConfig
}

type AuthConfig struct {
	JWTSecret           string
	AccessTokenTTLHours int
}

type RedisConfig struct {
	Addr       string
	Username   string
	Password   string
	DB         int
	TLSEnabled bool
	Required   bool
}

type WebSocketConfig struct {
	BroadcastConcurrencyLimit int64
	BroadcastTimeoutMs        int
	BroadcastEnqueueTimeoutMs int
	ClientOutboxCapacity      int
	MaxConnections            int64
	MaxRoomClients            int
	SeekMinIntervalMs         int
	ControlIdempotencyTTLms   int
	PresenceLeaseTTLms        int
	PresenceRefreshIntervalMs int
	CrossInstanceBroadcast    bool
	EventBus                  string
}

type NATSConfig struct {
	URL                  string
	Name                 string
	SubjectRoomBroadcast string
	SubjectRoomControl   string
}

type KafkaConfig struct {
	Brokers                   []string
	ClientID                  string
	TopicRoomTimeline         string
	TopicRoomControlResult    string
	TopicRoomMembership       string
	DerivedConsumerGroupID    string
	DerivedWorkerPollInterval int
}

type OutboxWorkerConfig struct {
	BatchSize      int
	PollIntervalMs int
}

type AuthorityRecoveryConfig struct {
	RenewIntervalMs        int
	TakeoverScanIntervalMs int
	RecoveryTimeoutMs      int
	KafkaReplayTimeoutMs   int
}

type ObservabilityConfig struct {
	MetricsEnabled bool
	MetricsAddr    string
	MetricsPath    string
	ReadinessPath  string
}

type ServiceConfig struct {
	Name    string
	Version string
}

type InternalRPCConfig struct {
	Enabled    bool
	Addr       string
	PathPrefix string
	TimeoutMs  int
	AuthToken  string
}

type ServiceClientsConfig struct {
	DiscoveryMode string
	MediaMode     string
	MediaAddr     string
	TimelineMode  string
	TimelineAddr  string
}

type TelemetryConfig struct {
	TracingEnabled   bool
	ServiceName      string
	OTLPEndpoint     string
	TraceSampleRatio float64
}

type MediaPlaybackConfig struct {
	DeliveryMode           string
	SigningSecret          string
	URLTTLSeconds          int
	PublicBaseURL          string
	InternalBaseURL        string
	StorageEndpoint        string
	StorageBucket          string
	StorageRegion          string
	StorageAccessKeyID     string
	StorageSecretAccessKey string
	StorageForcePathStyle  bool
}

func LoadServerRuntimeConfig(configDir string) (ServerRuntimeConfig, error) {
	defaults := map[string]any{
		"APP_ENV":                               "local",
		"SERVER_HOST":                           "0.0.0.0",
		"SERVER_PORT":                           "8080",
		"LOG_LEVEL":                             "debug",
		"SERVER_INSTANCE_ID":                    "",
		"ROOM_RUNTIME_MODE":                     "local_process",
		"DEBUG_SYNC":                            true,
		"AUTH_ACCESS_TOKEN_TTL_HOURS":           24,
		"REDIS_DB":                              0,
		"WS_BROADCAST_CONCURRENCY_LIMIT":        64,
		"WS_BROADCAST_TIMEOUT_MS":               5000,
		"WS_BROADCAST_ENQUEUE_TIMEOUT_MS":       3000,
		"WS_CLIENT_OUTBOX_CAPACITY":             64,
		"WS_MAX_CONNECTIONS":                    0,
		"ROOM_MAX_CLIENTS":                      0,
		"WS_SEEK_MIN_INTERVAL_MS":               250,
		"CONTROL_IDEMPOTENCY_TTL_MS":            600000,
		"PRESENCE_LEASE_TTL_MS":                 45000,
		"PRESENCE_REFRESH_INTERVAL_MS":          15000,
		"WS_CROSS_INSTANCE_BROADCAST_ENABLED":   false,
		"WS_EVENT_BUS":                          "nats_core",
		"NATS_URL":                              "nats://127.0.0.1:4222",
		"NATS_NAME":                             "watch-together-roomserver",
		"NATS_SUBJECT_ROOM_BROADCAST":           "wt.room.broadcast.v1",
		"NATS_SUBJECT_ROOM_CONTROL":             "wt.room.control.v1",
		"KAFKA_BROKERS":                         "",
		"KAFKA_CLIENT_ID":                       "watch-together-roomserver",
		"KAFKA_TOPIC_ROOM_TIMELINE":             "wt.room.timeline.v1",
		"KAFKA_TOPIC_ROOM_CONTROL_RESULT":       "wt.room.control_result.v1",
		"KAFKA_TOPIC_ROOM_MEMBERSHIP":           "wt.room.membership.v1",
		"KAFKA_DERIVED_CONSUMER_GROUP_ID":       "watch-together-derived-workers",
		"KAFKA_DERIVED_WORKER_POLL_INTERVAL_MS": 1000,
		"OUTBOX_WORKER_BATCH_SIZE":              50,
		"OUTBOX_WORKER_POLL_INTERVAL_MS":        1000,
		"AUTHORITY_RENEW_INTERVAL_MS":           10000,
		"AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS":   30000,
		"AUTHORITY_RECOVERY_TIMEOUT_MS":         5000,
		"KAFKA_REPLAY_TIMEOUT_MS":               1000,
		"METRICS_ENABLED":                       true,
		"METRICS_ADDR":                          "",
		"METRICS_PATH":                          "/metrics",
		"READINESS_PATH":                        "/readyz",
		"SERVICE_NAME":                          "watch-together-roomserver",
		"SERVICE_VERSION":                       "dev",
		"INTERNAL_RPC_ENABLED":                  false,
		"INTERNAL_RPC_ADDR":                     ":8090",
		"INTERNAL_RPC_PATH_PREFIX":              "/internal.rpc",
		"INTERNAL_RPC_TIMEOUT_MS":               1000,
		"INTERNAL_RPC_AUTH_TOKEN":               "",
		"SERVICE_DISCOVERY_MODE":                "static",
		"MEDIA_SERVICE_MODE":                    "local",
		"MEDIA_SERVICE_ADDR":                    "",
		"TIMELINE_SERVICE_MODE":                 "local",
		"TIMELINE_SERVICE_ADDR":                 "",
		"OTEL_TRACING_ENABLED":                  false,
		"OTEL_SERVICE_NAME":                     "watch-together-roomserver",
		"OTEL_EXPORTER_OTLP_ENDPOINT":           "",
		"OTEL_TRACE_SAMPLE_RATIO":               "0.1",
		"MEDIA_DELIVERY_MODE":                   "signed_redirect",
		"MEDIA_PLAYBACK_URL_TTL_SECONDS":        7200,
		"MEDIA_STORAGE_FORCE_PATH_STYLE":        "true",
	}
	keys := []string{
		"APP_ENV",
		"SERVER_HOST",
		"SERVER_PORT",
		"LOG_LEVEL",
		"SERVER_INSTANCE_ID",
		"ROOM_RUNTIME_MODE",
		"DATABASE_URL",
		"MEDIA_DATABASE_URL",
		"TIMELINE_DATABASE_URL",
		"DEBUG_SYNC",
		"AUTH_JWT_SECRET",
		"AUTH_ACCESS_TOKEN_TTL_HOURS",
		"REDIS_ADDR",
		"REDIS_USERNAME",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"REDIS_TLS_ENABLED",
		"REDIS_REQUIRED",
		"WS_BROADCAST_CONCURRENCY_LIMIT",
		"WS_BROADCAST_TIMEOUT_MS",
		"WS_BROADCAST_ENQUEUE_TIMEOUT_MS",
		"WS_CLIENT_OUTBOX_CAPACITY",
		"WS_MAX_CONNECTIONS",
		"ROOM_MAX_CLIENTS",
		"WS_SEEK_MIN_INTERVAL_MS",
		"CONTROL_IDEMPOTENCY_TTL_MS",
		"PRESENCE_LEASE_TTL_MS",
		"PRESENCE_REFRESH_INTERVAL_MS",
		"WS_CROSS_INSTANCE_BROADCAST_ENABLED",
		"WS_EVENT_BUS",
		"NATS_URL",
		"NATS_NAME",
		"NATS_SUBJECT_ROOM_BROADCAST",
		"NATS_SUBJECT_ROOM_CONTROL",
		"KAFKA_BROKERS",
		"KAFKA_CLIENT_ID",
		"KAFKA_TOPIC_ROOM_TIMELINE",
		"KAFKA_TOPIC_ROOM_CONTROL_RESULT",
		"KAFKA_TOPIC_ROOM_MEMBERSHIP",
		"KAFKA_DERIVED_CONSUMER_GROUP_ID",
		"KAFKA_DERIVED_WORKER_POLL_INTERVAL_MS",
		"OUTBOX_WORKER_BATCH_SIZE",
		"OUTBOX_WORKER_POLL_INTERVAL_MS",
		"AUTHORITY_RENEW_INTERVAL_MS",
		"AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS",
		"AUTHORITY_RECOVERY_TIMEOUT_MS",
		"KAFKA_REPLAY_TIMEOUT_MS",
		"METRICS_ENABLED",
		"METRICS_ADDR",
		"METRICS_PATH",
		"READINESS_PATH",
		"SERVICE_NAME",
		"SERVICE_VERSION",
		"INTERNAL_RPC_ENABLED",
		"INTERNAL_RPC_ADDR",
		"INTERNAL_RPC_PATH_PREFIX",
		"INTERNAL_RPC_TIMEOUT_MS",
		"INTERNAL_RPC_AUTH_TOKEN",
		"SERVICE_DISCOVERY_MODE",
		"MEDIA_SERVICE_MODE",
		"MEDIA_SERVICE_ADDR",
		"TIMELINE_SERVICE_MODE",
		"TIMELINE_SERVICE_ADDR",
		"OTEL_TRACING_ENABLED",
		"OTEL_SERVICE_NAME",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_TRACE_SAMPLE_RATIO",
		"MEDIA_DELIVERY_MODE",
		"MEDIA_PLAYBACK_SIGNING_SECRET",
		"MEDIA_PLAYBACK_URL_TTL_SECONDS",
		"MEDIA_PUBLIC_BASE_URL",
		"MEDIA_INTERNAL_BASE_URL",
		"MEDIA_STORAGE_ENDPOINT",
		"MEDIA_STORAGE_BUCKET",
		"MEDIA_STORAGE_REGION",
		"MEDIA_STORAGE_ACCESS_KEY_ID",
		"MEDIA_STORAGE_SECRET_ACCESS_KEY",
		"MEDIA_STORAGE_FORCE_PATH_STYLE",
	}

	loader, err := newLoader(configDir, defaults, keys)
	if err != nil {
		return ServerRuntimeConfig{}, err
	}
	roomRuntimeMode, err := parseRoomRuntimeMode(trimmedString(loader, "ROOM_RUNTIME_MODE"))
	if err != nil {
		return ServerRuntimeConfig{}, err
	}
	eventBus, err := parseEventBus(trimmedString(loader, "WS_EVENT_BUS"))
	if err != nil {
		return ServerRuntimeConfig{}, err
	}

	config := ServerRuntimeConfig{
		AppEnv:              trimmedString(loader, "APP_ENV"),
		Host:                trimmedString(loader, "SERVER_HOST"),
		Port:                trimmedString(loader, "SERVER_PORT"),
		LogLevel:            trimmedString(loader, "LOG_LEVEL"),
		InstanceID:          trimmedString(loader, "SERVER_INSTANCE_ID"),
		RoomRuntimeMode:     roomRuntimeMode,
		DatabaseURL:         trimmedString(loader, "DATABASE_URL"),
		MediaDatabaseURL:    trimmedString(loader, "MEDIA_DATABASE_URL"),
		TimelineDatabaseURL: trimmedString(loader, "TIMELINE_DATABASE_URL"),
		DebugSync:           strings.EqualFold(strings.TrimSpace(loader.GetString("DEBUG_SYNC")), "true"),
		Auth: AuthConfig{
			JWTSecret:           trimmedString(loader, "AUTH_JWT_SECRET"),
			AccessTokenTTLHours: intFromConfig(loader, "AUTH_ACCESS_TOKEN_TTL_HOURS"),
		},
		Redis: RedisConfig{
			Addr:       trimmedString(loader, "REDIS_ADDR"),
			Username:   trimmedString(loader, "REDIS_USERNAME"),
			Password:   trimmedString(loader, "REDIS_PASSWORD"),
			DB:         intFromConfig(loader, "REDIS_DB"),
			TLSEnabled: boolFromConfig(loader, "REDIS_TLS_ENABLED"),
			Required:   boolFromConfig(loader, "REDIS_REQUIRED"),
		},
		WebSocket: WebSocketConfig{
			BroadcastConcurrencyLimit: int64FromConfig(loader, "WS_BROADCAST_CONCURRENCY_LIMIT"),
			BroadcastTimeoutMs:        intFromConfig(loader, "WS_BROADCAST_TIMEOUT_MS"),
			BroadcastEnqueueTimeoutMs: intFromConfig(loader, "WS_BROADCAST_ENQUEUE_TIMEOUT_MS"),
			ClientOutboxCapacity:      intFromConfig(loader, "WS_CLIENT_OUTBOX_CAPACITY"),
			MaxConnections:            int64FromConfig(loader, "WS_MAX_CONNECTIONS"),
			MaxRoomClients:            intFromConfig(loader, "ROOM_MAX_CLIENTS"),
			SeekMinIntervalMs:         intFromConfig(loader, "WS_SEEK_MIN_INTERVAL_MS"),
			ControlIdempotencyTTLms:   intFromConfig(loader, "CONTROL_IDEMPOTENCY_TTL_MS"),
			PresenceLeaseTTLms:        intFromConfig(loader, "PRESENCE_LEASE_TTL_MS"),
			PresenceRefreshIntervalMs: intFromConfig(loader, "PRESENCE_REFRESH_INTERVAL_MS"),
			CrossInstanceBroadcast:    boolFromConfig(loader, "WS_CROSS_INSTANCE_BROADCAST_ENABLED"),
			EventBus:                  eventBus,
		},
		NATS: NATSConfig{
			URL:                  trimmedString(loader, "NATS_URL"),
			Name:                 trimmedString(loader, "NATS_NAME"),
			SubjectRoomBroadcast: trimmedString(loader, "NATS_SUBJECT_ROOM_BROADCAST"),
			SubjectRoomControl:   trimmedString(loader, "NATS_SUBJECT_ROOM_CONTROL"),
		},
		Kafka: KafkaConfig{
			Brokers:                   csvFromConfig(loader, "KAFKA_BROKERS"),
			ClientID:                  trimmedString(loader, "KAFKA_CLIENT_ID"),
			TopicRoomTimeline:         trimmedString(loader, "KAFKA_TOPIC_ROOM_TIMELINE"),
			TopicRoomControlResult:    trimmedString(loader, "KAFKA_TOPIC_ROOM_CONTROL_RESULT"),
			TopicRoomMembership:       trimmedString(loader, "KAFKA_TOPIC_ROOM_MEMBERSHIP"),
			DerivedConsumerGroupID:    trimmedString(loader, "KAFKA_DERIVED_CONSUMER_GROUP_ID"),
			DerivedWorkerPollInterval: intFromConfig(loader, "KAFKA_DERIVED_WORKER_POLL_INTERVAL_MS"),
		},
		OutboxWorker: OutboxWorkerConfig{
			BatchSize:      intFromConfig(loader, "OUTBOX_WORKER_BATCH_SIZE"),
			PollIntervalMs: intFromConfig(loader, "OUTBOX_WORKER_POLL_INTERVAL_MS"),
		},
		AuthorityRecovery: AuthorityRecoveryConfig{
			RenewIntervalMs:        intFromConfig(loader, "AUTHORITY_RENEW_INTERVAL_MS"),
			TakeoverScanIntervalMs: intFromConfig(loader, "AUTHORITY_TAKEOVER_SCAN_INTERVAL_MS"),
			RecoveryTimeoutMs:      intFromConfig(loader, "AUTHORITY_RECOVERY_TIMEOUT_MS"),
			KafkaReplayTimeoutMs:   intFromConfig(loader, "KAFKA_REPLAY_TIMEOUT_MS"),
		},
		Observability: ObservabilityConfig{
			MetricsEnabled: boolFromConfig(loader, "METRICS_ENABLED"),
			MetricsAddr:    trimmedString(loader, "METRICS_ADDR"),
			MetricsPath:    trimmedString(loader, "METRICS_PATH"),
			ReadinessPath:  trimmedString(loader, "READINESS_PATH"),
		},
		Service: ServiceConfig{
			Name:    trimmedString(loader, "SERVICE_NAME"),
			Version: trimmedString(loader, "SERVICE_VERSION"),
		},
		InternalRPC: InternalRPCConfig{
			Enabled:    boolFromConfig(loader, "INTERNAL_RPC_ENABLED"),
			Addr:       trimmedString(loader, "INTERNAL_RPC_ADDR"),
			PathPrefix: trimmedString(loader, "INTERNAL_RPC_PATH_PREFIX"),
			TimeoutMs:  intFromConfig(loader, "INTERNAL_RPC_TIMEOUT_MS"),
			AuthToken:  trimmedString(loader, "INTERNAL_RPC_AUTH_TOKEN"),
		},
		ServiceClients: ServiceClientsConfig{
			DiscoveryMode: strings.ToLower(trimmedString(loader, "SERVICE_DISCOVERY_MODE")),
			MediaMode:     strings.ToLower(trimmedString(loader, "MEDIA_SERVICE_MODE")),
			MediaAddr:     trimmedString(loader, "MEDIA_SERVICE_ADDR"),
			TimelineMode:  strings.ToLower(trimmedString(loader, "TIMELINE_SERVICE_MODE")),
			TimelineAddr:  trimmedString(loader, "TIMELINE_SERVICE_ADDR"),
		},
		Telemetry: TelemetryConfig{
			TracingEnabled:   boolFromConfig(loader, "OTEL_TRACING_ENABLED"),
			ServiceName:      trimmedString(loader, "OTEL_SERVICE_NAME"),
			OTLPEndpoint:     trimmedString(loader, "OTEL_EXPORTER_OTLP_ENDPOINT"),
			TraceSampleRatio: floatFromConfig(loader, "OTEL_TRACE_SAMPLE_RATIO"),
		},
		Media: MediaPlaybackConfig{
			DeliveryMode:           trimmedString(loader, "MEDIA_DELIVERY_MODE"),
			SigningSecret:          trimmedString(loader, "MEDIA_PLAYBACK_SIGNING_SECRET"),
			URLTTLSeconds:          intFromConfig(loader, "MEDIA_PLAYBACK_URL_TTL_SECONDS"),
			PublicBaseURL:          trimmedString(loader, "MEDIA_PUBLIC_BASE_URL"),
			InternalBaseURL:        trimmedString(loader, "MEDIA_INTERNAL_BASE_URL"),
			StorageEndpoint:        trimmedString(loader, "MEDIA_STORAGE_ENDPOINT"),
			StorageBucket:          trimmedString(loader, "MEDIA_STORAGE_BUCKET"),
			StorageRegion:          trimmedString(loader, "MEDIA_STORAGE_REGION"),
			StorageAccessKeyID:     trimmedString(loader, "MEDIA_STORAGE_ACCESS_KEY_ID"),
			StorageSecretAccessKey: trimmedString(loader, "MEDIA_STORAGE_SECRET_ACCESS_KEY"),
			StorageForcePathStyle:  boolFromConfig(loader, "MEDIA_STORAGE_FORCE_PATH_STYLE"),
		},
	}
	if err := validateServerRuntimeConfig(config); err != nil {
		return ServerRuntimeConfig{}, err
	}
	return config, nil
}

func parseRoomRuntimeMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return roomRuntimeModeLocalProcess, nil
	}
	if normalized != roomRuntimeModeLocalProcess && normalized != roomRuntimeModeDistributedAuthority {
		return "", fmt.Errorf(
			"unsupported ROOM_RUNTIME_MODE %q; supported values: %s, %s",
			value,
			roomRuntimeModeLocalProcess,
			roomRuntimeModeDistributedAuthority,
		)
	}
	return normalized, nil
}

func validateServerRuntimeConfig(config ServerRuntimeConfig) error {
	if err := validateServiceConfig(config); err != nil {
		return err
	}
	if config.RoomRuntimeMode != roomRuntimeModeDistributedAuthority {
		return nil
	}
	if strings.TrimSpace(config.InstanceID) == "" {
		return fmt.Errorf("SERVER_INSTANCE_ID is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if !config.WebSocket.CrossInstanceBroadcast {
		return fmt.Errorf("WS_CROSS_INSTANCE_BROADCAST_ENABLED=true is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if strings.TrimSpace(config.DatabaseURL) == "" {
		return fmt.Errorf("DATABASE_URL is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if strings.TrimSpace(config.Redis.Addr) == "" {
		return fmt.Errorf("REDIS_ADDR is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if strings.TrimSpace(config.NATS.URL) == "" {
		return fmt.Errorf("NATS_URL is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if len(config.Kafka.Brokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	if strings.TrimSpace(config.Kafka.TopicRoomTimeline) == "" ||
		strings.TrimSpace(config.Kafka.TopicRoomControlResult) == "" ||
		strings.TrimSpace(config.Kafka.TopicRoomMembership) == "" {
		return fmt.Errorf("Kafka room timeline and derived topics are required when ROOM_RUNTIME_MODE=%s", roomRuntimeModeDistributedAuthority)
	}
	return nil
}

func validateServiceConfig(config ServerRuntimeConfig) error {
	if config.InternalRPC.Enabled && strings.TrimSpace(config.InternalRPC.Addr) == "" {
		return fmt.Errorf("INTERNAL_RPC_ADDR is required when INTERNAL_RPC_ENABLED=true")
	}
	if config.InternalRPC.Enabled && strings.TrimSpace(config.InternalRPC.AuthToken) == "" &&
		strings.EqualFold(strings.TrimSpace(config.AppEnv), "prod") {
		return fmt.Errorf("INTERNAL_RPC_AUTH_TOKEN is required in prod when INTERNAL_RPC_ENABLED=true")
	}
	if mode := normalizeServiceMode(config.ServiceClients.MediaMode); mode != "local" && mode != "rpc" {
		return fmt.Errorf("unsupported MEDIA_SERVICE_MODE %q; supported values: local, rpc", config.ServiceClients.MediaMode)
	}
	if mode := normalizeServiceMode(config.ServiceClients.TimelineMode); mode != "local" && mode != "rpc" {
		return fmt.Errorf("unsupported TIMELINE_SERVICE_MODE %q; supported values: local, rpc", config.ServiceClients.TimelineMode)
	}
	if normalizeServiceMode(config.ServiceClients.MediaMode) == "rpc" && strings.TrimSpace(config.ServiceClients.MediaAddr) == "" {
		return fmt.Errorf("MEDIA_SERVICE_ADDR is required when MEDIA_SERVICE_MODE=rpc")
	}
	if normalizeServiceMode(config.ServiceClients.TimelineMode) == "rpc" && strings.TrimSpace(config.ServiceClients.TimelineAddr) == "" {
		return fmt.Errorf("TIMELINE_SERVICE_ADDR is required when TIMELINE_SERVICE_MODE=rpc")
	}
	discoveryMode := strings.TrimSpace(config.ServiceClients.DiscoveryMode)
	if discoveryMode == "" {
		discoveryMode = "static"
	}
	if discoveryMode != "static" {
		return fmt.Errorf("unsupported SERVICE_DISCOVERY_MODE %q; supported values: static", config.ServiceClients.DiscoveryMode)
	}
	if config.Telemetry.TraceSampleRatio < 0 || config.Telemetry.TraceSampleRatio > 1 {
		return fmt.Errorf("OTEL_TRACE_SAMPLE_RATIO must be between 0 and 1")
	}
	return nil
}

func normalizeServiceMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "local"
	}
	return mode
}

func parseEventBus(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return eventBusNATSCore, nil
	}
	if normalized != eventBusNATSCore {
		return "", fmt.Errorf("unsupported WS_EVENT_BUS %q; supported values: %s", value, eventBusNATSCore)
	}
	return normalized, nil
}

func boolFromConfig(loader interface{ GetString(string) string }, key string) bool {
	return strings.EqualFold(strings.TrimSpace(loader.GetString(key)), "true")
}

func intFromConfig(loader interface{ GetString(string) string }, key string) int {
	value := strings.TrimSpace(loader.GetString(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func int64FromConfig(loader interface{ GetString(string) string }, key string) int64 {
	value := strings.TrimSpace(loader.GetString(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func floatFromConfig(loader interface{ GetString(string) string }, key string) float64 {
	value := strings.TrimSpace(loader.GetString(key))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func csvFromConfig(loader interface{ GetString(string) string }, key string) []string {
	value := strings.TrimSpace(loader.GetString(key))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}
