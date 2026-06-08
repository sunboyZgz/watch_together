package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type ownershipRegistry struct {
	Version            int                       `yaml:"version"`
	StoreFiles         map[string]string         `yaml:"store_files"`
	Tables             map[string]tableOwnership `yaml:"tables"`
	CrossContextAccess []crossContextAccess      `yaml:"cross_context_access"`
}

type tableOwnership struct {
	Owner   string   `yaml:"owner"`
	Writers []string `yaml:"writers"`
	Readers []string `yaml:"readers"`
	Status  string   `yaml:"status"`
}

type crossContextAccess struct {
	Caller string   `yaml:"caller"`
	Owner  string   `yaml:"owner"`
	Tables []string `yaml:"tables"`
	Access string   `yaml:"access"`
	Path   string   `yaml:"path"`
	Reason string   `yaml:"reason"`
}

func TestDatabaseOwnershipRegistryCoversCoreTables(t *testing.T) {
	registry := loadOwnershipRegistry(t)
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
	for _, table := range required {
		ownership, ok := registry.Tables[table]
		if !ok {
			t.Fatalf("expected ownership registry to include table %s", table)
		}
		if strings.TrimSpace(ownership.Owner) == "" {
			t.Fatalf("expected table %s to declare an owner", table)
		}
		if len(ownership.Writers) == 0 {
			t.Fatalf("expected table %s to declare writer contexts", table)
		}
	}
}

func TestDatabaseOwnershipDocumentReferencesRegistryAndMediaDatabaseBoundary(t *testing.T) {
	content, err := os.ReadFile("../../../docs/database-ownership.md")
	if err != nil {
		t.Fatalf("read database ownership document: %v", err)
	}
	text := string(content)
	for _, expected := range []string{
		"server/internal/store/db_ownership.yaml",
		"MEDIA_DATABASE_URL",
		"independent media database",
		"home-composition",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected database ownership document to mention %q", expected)
		}
	}
}

func TestStoreSQLWritesStayInsideOwnerBoundary(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	for fileName, contextName := range registry.StoreFiles {
		content := rawStringContent(readStoreFile(t, fileName))
		for _, table := range writeTables(content) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("%s writes table %s but the table has no owner", fileName, table)
			}
			if !containsContext(ownership.Writers, contextName) {
				t.Fatalf(
					"%s context %q writes table %s owned by %q; allowed writers: %v",
					fileName,
					contextName,
					table,
					ownership.Owner,
					ownership.Writers,
				)
			}
		}
	}
}

func TestCrossContextSQLReadsAreRegistered(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	for fileName, contextName := range registry.StoreFiles {
		content := rawStringContent(readStoreFile(t, fileName))
		for _, table := range readTables(content) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("%s reads table %s but the table has no owner", fileName, table)
			}
			if contextName == ownership.Owner {
				continue
			}
			if !containsContext(ownership.Readers, contextName) {
				t.Fatalf(
					"%s context %q reads table %s owned by %q without registry permission; allowed readers: %v",
					fileName,
					contextName,
					table,
					ownership.Owner,
					ownership.Readers,
				)
			}
			if !hasCrossContextAccess(registry.CrossContextAccess, contextName, ownership.Owner, fileName, table, "read") {
				t.Fatalf("%s context %q reads cross-context table %s without cross_context_access entry", fileName, contextName, table)
			}
		}
	}
}

func TestMigrationCreatedTablesDeclareOwners(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			if _, ok := registry.Tables[table]; !ok {
				t.Fatalf("migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
		}
	}
}

func TestMediaMigrationCreatedTablesDeclareMediaOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../media_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob media migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected media migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read media migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("media migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "media" {
				t.Fatalf("media migration %s creates table %s owned by %q, want media", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func TestTimelineMigrationCreatedTablesDeclareTimelineOwner(t *testing.T) {
	registry := loadOwnershipRegistry(t)
	files, err := filepath.Glob("../../timeline_migrations/*.up.sql")
	if err != nil {
		t.Fatalf("glob timeline migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected timeline migrations to exist")
	}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read timeline migration %s: %v", file, err)
		}
		for _, table := range createdTables(string(contentBytes)) {
			ownership, ok := registry.Tables[table]
			if !ok {
				t.Fatalf("timeline migration %s creates table %s without ownership registry entry", filepath.Base(file), table)
			}
			if ownership.Owner != "timeline" {
				t.Fatalf("timeline migration %s creates table %s owned by %q, want timeline", filepath.Base(file), table, ownership.Owner)
			}
		}
	}
}

func loadOwnershipRegistry(t *testing.T) ownershipRegistry {
	t.Helper()
	content, err := os.ReadFile("db_ownership.yaml")
	if err != nil {
		t.Fatalf("read ownership registry: %v", err)
	}
	var registry ownershipRegistry
	if err := yaml.Unmarshal(content, &registry); err != nil {
		t.Fatalf("parse ownership registry: %v", err)
	}
	if registry.Version != 1 {
		t.Fatalf("expected ownership registry version 1, got %d", registry.Version)
	}
	if len(registry.Tables) == 0 {
		t.Fatalf("expected ownership registry tables")
	}
	return registry
}

func readStoreFile(t *testing.T, fileName string) string {
	t.Helper()
	content, err := os.ReadFile(fileName)
	if err != nil {
		t.Fatalf("read store file %s: %v", fileName, err)
	}
	return string(content)
}

func writeTables(content string) []string {
	tables := uniqueMatches(content, regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+([a-z_][a-z0-9_]*)\s*\(`))
	tables = append(tables, uniqueMatches(content, regexp.MustCompile(`(?i)\bUPDATE\s+([a-z_][a-z0-9_]*)\s+SET\b`))...)
	tables = append(tables, uniqueMatches(content, regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+([a-z_][a-z0-9_]*)\b`))...)
	return uniqueStrings(tables)
}

func readTables(content string) []string {
	return uniqueMatches(content, regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-z_][a-z0-9_]*)`))
}

func createdTables(content string) []string {
	return uniqueMatches(content, regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`))
}

func uniqueMatches(content string, expression *regexp.Regexp) []string {
	seen := map[string]bool{}
	var tables []string
	for _, match := range expression.FindAllStringSubmatch(content, -1) {
		if len(match) < 2 {
			continue
		}
		table := strings.ToLower(strings.TrimSpace(match[1]))
		if table == "" || seen[table] {
			continue
		}
		seen[table] = true
		tables = append(tables, table)
	}
	return tables
}

func rawStringContent(content string) string {
	matches := regexp.MustCompile("(?s)`([^`]*)`").FindAllStringSubmatch(content, -1)
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			parts = append(parts, match[1])
		}
	}
	return strings.Join(parts, "\n")
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func containsContext(contexts []string, contextName string) bool {
	for _, candidate := range contexts {
		if candidate == contextName {
			return true
		}
	}
	return false
}

func hasCrossContextAccess(
	entries []crossContextAccess,
	caller string,
	owner string,
	path string,
	table string,
	access string,
) bool {
	for _, entry := range entries {
		if entry.Caller != caller || entry.Owner != owner || entry.Path != path || entry.Access != access {
			continue
		}
		for _, entryTable := range entry.Tables {
			if entryTable == table {
				return strings.TrimSpace(entry.Reason) != ""
			}
		}
	}
	return false
}
