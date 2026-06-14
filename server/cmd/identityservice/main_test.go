package main

import (
	"testing"

	wtconfig "watch_together/server/internal/config"
)

func TestIdentityDatabaseURLRequiresIdentityDatabase(t *testing.T) {
	cfg := wtconfig.ServerRuntimeConfig{
		DatabaseURL:         "postgres://main",
		IdentityDatabaseURL: "postgres://identity",
	}
	if got := identityDatabaseURL(cfg); got != "postgres://identity" {
		t.Fatalf("identityDatabaseURL() = %q, want identity database", got)
	}

	cfg.IdentityDatabaseURL = ""
	if got := identityDatabaseURL(cfg); got != "" {
		t.Fatalf("identityDatabaseURL() = %q, want no main database fallback", got)
	}
}
