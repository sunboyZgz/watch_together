package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var errConfigFileMissing = errors.New("config file missing")

func newLoader(configDir string, defaults map[string]any, keys []string) (*viper.Viper, error) {
	loader := viper.New()
	for key, value := range defaults {
		loader.SetDefault(key, value)
	}
	for _, key := range keys {
		if err := loader.BindEnv(key); err != nil {
			return nil, fmt.Errorf("bind env %s: %w", key, err)
		}
	}
	loader.AutomaticEnv()

	if err := mergeOptionalEnvFile(loader, filepath.Join(configDir, ".env")); err != nil {
		return nil, err
	}

	appEnv := strings.TrimSpace(loader.GetString("APP_ENV"))
	if appEnv == "" {
		appEnv = "local"
	}

	for _, name := range []string{
		".env." + appEnv,
		".env.local",
		".env." + appEnv + ".local",
	} {
		if err := mergeOptionalEnvFile(loader, filepath.Join(configDir, name)); err != nil {
			return nil, err
		}
	}
	return loader, nil
}

func mergeOptionalEnvFile(loader *viper.Viper, path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat config file %q: %w", path, err)
	}

	fileLoader := viper.New()
	fileLoader.SetConfigFile(path)
	fileLoader.SetConfigType("env")
	if err := fileLoader.ReadInConfig(); err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	return loader.MergeConfigMap(fileLoader.AllSettings())
}

func trimmedString(loader *viper.Viper, key string) string {
	return strings.TrimSpace(loader.GetString(key))
}
