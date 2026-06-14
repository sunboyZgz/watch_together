package main

import (
	"testing"

	wtconfig "watch_together/server/internal/config"
)

func TestTimelineDatabaseURLRequiresTimelineDatabase(t *testing.T) {
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
			name: "main database ignored",
			cfg: wtconfig.ServerRuntimeConfig{
				DatabaseURL: "postgres://main",
			},
			want: "",
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
