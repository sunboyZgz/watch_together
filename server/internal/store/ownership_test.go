package store

import (
	"os"
	"strings"
	"testing"
)

func TestDatabaseOwnershipDocumentCoversCoreTables(t *testing.T) {
	content, err := os.ReadFile("../../../docs/database-ownership.md")
	if err != nil {
		t.Fatalf("read database ownership document: %v", err)
	}
	required := []string{
		"users",
		"media_tags",
		"media_seasons",
		"media_episodes",
		"media_episode_variants",
		"media_season_tags",
		"rooms",
		"room_members",
		"user_media_progress",
		"room_timeline_outbox",
	}
	text := string(content)
	for _, table := range required {
		if !strings.Contains(text, "`"+table+"`") {
			t.Fatalf("expected database ownership document to mention table %s", table)
		}
	}
	if !strings.Contains(text, "home-composition") {
		t.Fatalf("expected database ownership document to describe home-composition")
	}
	if !strings.Contains(text, "No physical database split") {
		t.Fatalf("expected database ownership document to state Phase 7 does not split PostgreSQL")
	}
}
