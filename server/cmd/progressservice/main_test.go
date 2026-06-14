package main

import (
	"testing"

	wtconfig "watch_together/server/internal/config"
)

func TestProgressDatabaseURLRequiresProgressDatabase(t *testing.T) {
	cfg := wtconfig.ServerRuntimeConfig{
		DatabaseURL:         "postgres://main",
		ProgressDatabaseURL: "postgres://progress",
	}
	if got := progressDatabaseURL(cfg); got != "postgres://progress" {
		t.Fatalf("progressDatabaseURL() = %q, want progress database", got)
	}

	cfg.ProgressDatabaseURL = ""
	if got := progressDatabaseURL(cfg); got != "" {
		t.Fatalf("progressDatabaseURL() = %q, want no main database fallback", got)
	}
}
