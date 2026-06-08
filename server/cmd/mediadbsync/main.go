package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type tableSpec struct {
	name         string
	columns      []string
	conflictCols []string
	orderBy      []string
	jsonbCols    map[string]struct{}
}

var mediaTables = []tableSpec{
	{
		name:         "media_tags",
		columns:      []string{"id", "name", "slug", "sort_order", "is_featured", "is_active", "created_at", "updated_at"},
		conflictCols: []string{"id"},
		orderBy:      []string{"id"},
	},
	{
		name: "media_seasons",
		columns: []string{
			"id", "slug", "title", "original_title", "description", "cover_url", "category", "production_team",
			"search_aliases", "season_number", "season_label", "sort_order", "status", "created_at", "updated_at",
		},
		conflictCols: []string{"id"},
		orderBy:      []string{"id"},
		jsonbCols:    map[string]struct{}{"search_aliases": {}},
	},
	{
		name: "media_episodes",
		columns: []string{
			"id", "season_id", "title", "subtitle", "description", "cover_url", "media_url", "duration_ms",
			"episode_number", "episode_label", "source_key", "source_hash", "sort_order", "status", "created_at", "updated_at",
		},
		conflictCols: []string{"id"},
		orderBy:      []string{"id"},
	},
	{
		name:         "media_season_tags",
		columns:      []string{"season_id", "media_tag_id", "created_at"},
		conflictCols: []string{"season_id", "media_tag_id"},
		orderBy:      []string{"season_id", "media_tag_id"},
	},
	{
		name: "media_episode_variants",
		columns: []string{
			"id", "media_episode_id", "variant_key", "label", "playlist_url", "width", "height", "bandwidth_bps",
			"codecs", "is_default", "sort_order", "segment_count", "average_segment_ms", "created_at", "updated_at",
		},
		conflictCols: []string{"id"},
		orderBy:      []string{"id"},
	},
}

type options struct {
	sourceURL  string
	targetURL  string
	dryRun     bool
	verifyOnly bool
	batchSize  int
}

func main() {
	opts := parseOptions()
	if err := run(context.Background(), opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOptions() options {
	var opts options
	flag.StringVar(&opts.sourceURL, "source-database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "source PostgreSQL URL; defaults to DATABASE_URL")
	flag.StringVar(&opts.targetURL, "target-database-url", strings.TrimSpace(os.Getenv("MEDIA_DATABASE_URL")), "target media PostgreSQL URL; defaults to MEDIA_DATABASE_URL")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "read source tables and print planned row counts without writing target")
	flag.BoolVar(&opts.verifyOnly, "verify-only", false, "compare source and target table counts plus stable row hashes without writing")
	flag.IntVar(&opts.batchSize, "batch-size", 500, "rows to copy per batch")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) error {
	if strings.TrimSpace(opts.sourceURL) == "" {
		return fmt.Errorf("--source-database-url or DATABASE_URL is required")
	}
	if !opts.dryRun && strings.TrimSpace(opts.targetURL) == "" {
		return fmt.Errorf("--target-database-url or MEDIA_DATABASE_URL is required")
	}
	if opts.verifyOnly && strings.TrimSpace(opts.targetURL) == "" {
		return fmt.Errorf("--target-database-url or MEDIA_DATABASE_URL is required for --verify-only")
	}
	if opts.batchSize <= 0 {
		opts.batchSize = 500
	}

	source, err := openDB(ctx, opts.sourceURL)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer source.Close()

	var target *sql.DB
	if !opts.dryRun || opts.verifyOnly {
		target, err = openDB(ctx, opts.targetURL)
		if err != nil {
			return fmt.Errorf("open target database: %w", err)
		}
		defer target.Close()
	}

	if opts.verifyOnly {
		return verify(ctx, source, target)
	}
	if opts.dryRun {
		return dryRun(ctx, source)
	}
	for _, table := range mediaTables {
		copied, err := syncTable(ctx, source, target, table, opts.batchSize)
		if err != nil {
			return err
		}
		fmt.Printf("%s copied %d rows\n", table.name, copied)
	}
	return nil
}

func openDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func dryRun(ctx context.Context, source *sql.DB) error {
	for _, table := range mediaTables {
		count, err := tableCount(ctx, source, table.name)
		if err != nil {
			return err
		}
		fmt.Printf("%s would copy %d rows\n", table.name, count)
	}
	return nil
}

func verify(ctx context.Context, source *sql.DB, target *sql.DB) error {
	var failed bool
	for _, table := range mediaTables {
		sourceCount, sourceHash, err := tableFingerprint(ctx, source, table)
		if err != nil {
			return fmt.Errorf("source %s fingerprint: %w", table.name, err)
		}
		targetCount, targetHash, err := tableFingerprint(ctx, target, table)
		if err != nil {
			return fmt.Errorf("target %s fingerprint: %w", table.name, err)
		}
		if sourceCount != targetCount || sourceHash != targetHash {
			failed = true
			fmt.Printf("%s mismatch source_count=%d target_count=%d source_hash=%s target_hash=%s\n",
				table.name,
				sourceCount,
				targetCount,
				sourceHash,
				targetHash,
			)
			continue
		}
		fmt.Printf("%s ok count=%d hash=%s\n", table.name, sourceCount, sourceHash)
	}
	if failed {
		return fmt.Errorf("media database verification failed")
	}
	return nil
}

func syncTable(ctx context.Context, source *sql.DB, target *sql.DB, table tableSpec, batchSize int) (int, error) {
	total := 0
	offset := 0
	for {
		rows, err := readBatch(ctx, source, table, batchSize, offset)
		if err != nil {
			return total, err
		}
		if len(rows) == 0 {
			return total, nil
		}
		if err := upsertRows(ctx, target, table, rows); err != nil {
			return total, err
		}
		total += len(rows)
		offset += len(rows)
	}
}

func readBatch(ctx context.Context, db *sql.DB, table tableSpec, limit int, offset int) ([][]any, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY %s LIMIT $1 OFFSET $2",
		joinIdentifiers(table.columns),
		table.name,
		joinIdentifiers(table.orderBy),
	)
	rows, err := db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("read %s batch: %w", table.name, err)
	}
	defer rows.Close()

	result := make([][]any, 0, limit)
	for rows.Next() {
		values, err := scanRow(rows, len(table.columns))
		if err != nil {
			return nil, fmt.Errorf("scan %s batch: %w", table.name, err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s batch: %w", table.name, err)
	}
	return result, nil
}

func upsertRows(ctx context.Context, db *sql.DB, table tableSpec, rows [][]any) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s sync: %w", table.name, err)
	}
	defer tx.Rollback()

	query := upsertQuery(table)
	for _, row := range rows {
		if _, err := tx.ExecContext(ctx, query, row...); err != nil {
			return fmt.Errorf("upsert %s: %w", table.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s sync: %w", table.name, err)
	}
	return nil
}

func upsertQuery(table tableSpec) string {
	updates := make([]string, 0, len(table.columns))
	for _, column := range table.columns {
		if contains(table.conflictCols, column) {
			continue
		}
		updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", column, column))
	}
	return fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		table.name,
		joinIdentifiers(table.columns),
		placeholders(table),
		joinIdentifiers(table.conflictCols),
		strings.Join(updates, ", "),
	)
}

func tableCount(ctx context.Context, db *sql.DB, tableName string) (int, error) {
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+tableName).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", tableName, err)
	}
	return count, nil
}

func tableFingerprint(ctx context.Context, db *sql.DB, table tableSpec) (int, string, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM %s ORDER BY %s",
		joinIdentifiers(table.columns),
		table.name,
		joinIdentifiers(table.orderBy),
	)
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()

	hash := sha256.New()
	count := 0
	for rows.Next() {
		values, err := scanRow(rows, len(table.columns))
		if err != nil {
			return 0, "", err
		}
		for _, value := range values {
			hash.Write([]byte(stableValue(value)))
			hash.Write([]byte{0})
		}
		hash.Write([]byte{'\n'})
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	return count, hex.EncodeToString(hash.Sum(nil)), nil
}

func scanRow(rows *sql.Rows, size int) ([]any, error) {
	values := make([]any, size)
	dest := make([]any, size)
	for i := range values {
		dest[i] = &values[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	return values, nil
}

func stableValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "<null>"
	case []byte:
		return string(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(v)
	}
}

func joinIdentifiers(columns []string) string {
	return strings.Join(columns, ", ")
}

func placeholders(table tableSpec) string {
	values := make([]string, 0, len(table.columns))
	for i, column := range table.columns {
		placeholder := fmt.Sprintf("$%d", i+1)
		if _, ok := table.jsonbCols[column]; ok {
			placeholder += "::jsonb"
		}
		values = append(values, placeholder)
	}
	return strings.Join(values, ", ")
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
