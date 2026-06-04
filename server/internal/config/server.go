package config

import (
	"fmt"
	"strconv"
	"strings"
)

const roomRuntimeModeLocalProcess = "local_process"
const eventBusNATSCore = "nats_core"

type ServerRuntimeConfig struct {
	AppEnv          string
	Host            string
	Port            string
	LogLevel        string
	InstanceID      string
	RoomRuntimeMode string
	DatabaseURL     string
	DebugSync       bool
	Auth            AuthConfig
	Redis           RedisConfig
	WebSocket       WebSocketConfig
	NATS            NATSConfig
	Media           MediaPlaybackConfig
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
	CrossInstanceBroadcast    bool
	EventBus                  string
}

type NATSConfig struct {
	URL                  string
	Name                 string
	SubjectRoomBroadcast string
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
		"APP_ENV":                             "local",
		"SERVER_HOST":                         "0.0.0.0",
		"SERVER_PORT":                         "8080",
		"LOG_LEVEL":                           "debug",
		"SERVER_INSTANCE_ID":                  "",
		"ROOM_RUNTIME_MODE":                   "local_process",
		"DEBUG_SYNC":                          true,
		"AUTH_ACCESS_TOKEN_TTL_HOURS":         24,
		"REDIS_DB":                            0,
		"WS_BROADCAST_CONCURRENCY_LIMIT":      64,
		"WS_BROADCAST_TIMEOUT_MS":             5000,
		"WS_BROADCAST_ENQUEUE_TIMEOUT_MS":     3000,
		"WS_CLIENT_OUTBOX_CAPACITY":           64,
		"WS_MAX_CONNECTIONS":                  0,
		"ROOM_MAX_CLIENTS":                    0,
		"WS_SEEK_MIN_INTERVAL_MS":             250,
		"WS_CROSS_INSTANCE_BROADCAST_ENABLED": false,
		"WS_EVENT_BUS":                        "nats_core",
		"NATS_URL":                            "nats://127.0.0.1:4222",
		"NATS_NAME":                           "watch-together-roomserver",
		"NATS_SUBJECT_ROOM_BROADCAST":         "wt.room.broadcast.v1",
		"MEDIA_DELIVERY_MODE":                 "signed_redirect",
		"MEDIA_PLAYBACK_URL_TTL_SECONDS":      7200,
		"MEDIA_STORAGE_FORCE_PATH_STYLE":      "true",
	}
	keys := []string{
		"APP_ENV",
		"SERVER_HOST",
		"SERVER_PORT",
		"LOG_LEVEL",
		"SERVER_INSTANCE_ID",
		"ROOM_RUNTIME_MODE",
		"DATABASE_URL",
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
		"WS_CROSS_INSTANCE_BROADCAST_ENABLED",
		"WS_EVENT_BUS",
		"NATS_URL",
		"NATS_NAME",
		"NATS_SUBJECT_ROOM_BROADCAST",
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

	return ServerRuntimeConfig{
		AppEnv:          trimmedString(loader, "APP_ENV"),
		Host:            trimmedString(loader, "SERVER_HOST"),
		Port:            trimmedString(loader, "SERVER_PORT"),
		LogLevel:        trimmedString(loader, "LOG_LEVEL"),
		InstanceID:      trimmedString(loader, "SERVER_INSTANCE_ID"),
		RoomRuntimeMode: roomRuntimeMode,
		DatabaseURL:     trimmedString(loader, "DATABASE_URL"),
		DebugSync:       strings.EqualFold(strings.TrimSpace(loader.GetString("DEBUG_SYNC")), "true"),
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
			CrossInstanceBroadcast:    boolFromConfig(loader, "WS_CROSS_INSTANCE_BROADCAST_ENABLED"),
			EventBus:                  eventBus,
		},
		NATS: NATSConfig{
			URL:                  trimmedString(loader, "NATS_URL"),
			Name:                 trimmedString(loader, "NATS_NAME"),
			SubjectRoomBroadcast: trimmedString(loader, "NATS_SUBJECT_ROOM_BROADCAST"),
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
	}, nil
}

func parseRoomRuntimeMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return roomRuntimeModeLocalProcess, nil
	}
	if normalized != roomRuntimeModeLocalProcess {
		return "", fmt.Errorf("unsupported ROOM_RUNTIME_MODE %q; supported values: %s", value, roomRuntimeModeLocalProcess)
	}
	return normalized, nil
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
