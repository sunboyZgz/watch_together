package main

import (
	"testing"

	wtconfig "watch_together/server/internal/config"
)

func TestMediaDatabaseURLPrefersMediaDatabaseAndFallsBackToMain(t *testing.T) {
	cases := []struct {
		name string
		cfg  wtconfig.ServerRuntimeConfig
		want string
	}{
		{
			name: "media database configured",
			cfg: wtconfig.ServerRuntimeConfig{
				DatabaseURL:      "postgres://main",
				MediaDatabaseURL: "postgres://media",
			},
			want: "postgres://media",
		},
		{
			name: "fallback main database",
			cfg: wtconfig.ServerRuntimeConfig{
				DatabaseURL: "postgres://main",
			},
			want: "postgres://main",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaDatabaseURL(tc.cfg); got != tc.want {
				t.Fatalf("mediaDatabaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
