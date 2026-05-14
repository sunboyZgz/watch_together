package config

import (
	"strconv"
	"strings"
)

type ServerRuntimeConfig struct {
	AppEnv      string
	Host        string
	Port        string
	LogLevel    string
	DatabaseURL string
	DebugSync   bool
	Redis       RedisConfig
}

type RedisConfig struct {
	Addr       string
	Username   string
	Password   string
	DB         int
	TLSEnabled bool
	Required   bool
}

func LoadServerRuntimeConfig(configDir string) (ServerRuntimeConfig, error) {
	defaults := map[string]any{
		"APP_ENV":     "local",
		"SERVER_HOST": "0.0.0.0",
		"SERVER_PORT": "8080",
		"LOG_LEVEL":   "debug",
		"DEBUG_SYNC":  true,
		"REDIS_DB":    0,
	}
	keys := []string{
		"APP_ENV",
		"SERVER_HOST",
		"SERVER_PORT",
		"LOG_LEVEL",
		"DATABASE_URL",
		"DEBUG_SYNC",
		"REDIS_ADDR",
		"REDIS_USERNAME",
		"REDIS_PASSWORD",
		"REDIS_DB",
		"REDIS_TLS_ENABLED",
		"REDIS_REQUIRED",
	}

	loader, err := newLoader(configDir, defaults, keys)
	if err != nil {
		return ServerRuntimeConfig{}, err
	}

	return ServerRuntimeConfig{
		AppEnv:      trimmedString(loader, "APP_ENV"),
		Host:        trimmedString(loader, "SERVER_HOST"),
		Port:        trimmedString(loader, "SERVER_PORT"),
		LogLevel:    trimmedString(loader, "LOG_LEVEL"),
		DatabaseURL: trimmedString(loader, "DATABASE_URL"),
		DebugSync:   strings.EqualFold(strings.TrimSpace(loader.GetString("DEBUG_SYNC")), "true"),
		Redis: RedisConfig{
			Addr:       trimmedString(loader, "REDIS_ADDR"),
			Username:   trimmedString(loader, "REDIS_USERNAME"),
			Password:   trimmedString(loader, "REDIS_PASSWORD"),
			DB:         intFromConfig(loader, "REDIS_DB"),
			TLSEnabled: boolFromConfig(loader, "REDIS_TLS_ENABLED"),
			Required:   boolFromConfig(loader, "REDIS_REQUIRED"),
		},
	}, nil
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
