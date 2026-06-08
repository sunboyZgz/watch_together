package main

import (
	"testing"

	wtconfig "watch_together/server/internal/config"
)

func TestTimelineDatabaseURLPrefersTimelineDatabaseAndFallsBackToMain(t *testing.T) {
	cases := []struct {
		name string
		cfg  wtconfig.ServerRuntimeConfig
		want string
	}{
		{
			name: "timeline database configured",
			cfg: wtconfig.ServerRuntimeConfig{
				DatabaseURL:         "postgres://main",
				TimelineDatabaseURL: "postgres://timeline",
			},
			want: "postgres://timeline",
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
			if got := timelineDatabaseURL(tc.cfg); got != tc.want {
				t.Fatalf("timelineDatabaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
