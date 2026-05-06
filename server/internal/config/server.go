package config

import "strings"

type ServerRuntimeConfig struct {
	AppEnv      string
	Host        string
	Port        string
	LogLevel    string
	DatabaseURL string
	DebugSync   bool
}

func LoadServerRuntimeConfig(configDir string) (ServerRuntimeConfig, error) {
	defaults := map[string]any{
		"APP_ENV":     "local",
		"SERVER_HOST": "0.0.0.0",
		"SERVER_PORT": "8080",
		"LOG_LEVEL":   "debug",
		"DEBUG_SYNC":  true,
	}
	keys := []string{
		"APP_ENV",
		"SERVER_HOST",
		"SERVER_PORT",
		"LOG_LEVEL",
		"DATABASE_URL",
		"DEBUG_SYNC",
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
	}, nil
}
