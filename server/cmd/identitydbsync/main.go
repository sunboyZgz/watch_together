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

var usersTable = tableSpec{
	name: "users",
	columns: []string{
		"id", "account", "password_hash", "nickname", "avatar_seed", "avatar_url", "bio", "created_at", "updated_at",
	},
	conflictCols: []string{"id"},
	orderBy:      []string{"id"},
}

type tableSpec struct {
	name         string
	columns      []string
	conflictCols []string
	orderBy      []string
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
	flag.StringVar(&opts.sourceURL, "source-database-url", "", "legacy source PostgreSQL URL; must be explicit")
	flag.StringVar(&opts.targetURL, "target-database-url", strings.TrimSpace(os.Getenv("IDENTITY_DATABASE_URL")), "target identity PostgreSQL URL; defaults to IDENTITY_DATABASE_URL")
	flag.BoolVar(&opts.dryRun, "dry-run", false, "read source table and print planned row count without writing target")
	flag.BoolVar(&opts.verifyOnly, "verify-only", false, "compare source and target table counts plus stable row hash without writing")
	flag.IntVar(&opts.batchSize, "batch-size", 500, "rows to copy per batch")
	flag.Parse()
	return opts
}

func run(ctx context.Context, opts options) error {
	if strings.TrimSpace(opts.sourceURL) == "" {
		return fmt.Errorf("--source-database-url is required")
	}
	if !opts.dryRun && strings.TrimSpace(opts.targetURL) == "" {
		return fmt.Errorf("--target-database-url or IDENTITY_DATABASE_URL is required")
	}
	if opts.verifyOnly && strings.TrimSpace(opts.targetURL) == "" {
		return fmt.Errorf("--target-database-url or IDENTITY_DATABASE_URL is required for --verify-only")
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
		count, err := tableCount(ctx, source, usersTable.name)
		if err != nil {
			return err
		}
		fmt.Printf("%s would copy %d rows\n", usersTable.name, count)
		return nil
	}
	copied, err := syncTable(ctx, source, target, usersTable, opts.batchSize)
	if err != nil {
		return err
	}
	fmt.Printf("%s copied %d rows\n", usersTable.name, copied)
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

func verify(ctx context.Context, source *sql.DB, target *sql.DB) error {
	sourceCount, sourceHash, err := tableFingerprint(ctx, source, usersTable)
	if err != nil {
		return fmt.Errorf("source %s fingerprint: %w", usersTable.name, err)
	}
	targetCount, targetHash, err := tableFingerprint(ctx, target, usersTable)
	if err != nil {
		return fmt.Errorf("target %s fingerprint: %w", usersTable.name, err)
	}
	if sourceCount != targetCount || sourceHash != targetHash {
		fmt.Printf("%s mismatch source_count=%d target_count=%d source_hash=%s target_hash=%s\n",
			usersTable.name,
			sourceCount,
			targetCount,
			sourceHash,
			targetHash,
		)
		return fmt.Errorf("identity database verification failed")
	}
	fmt.Printf("%s ok count=%d hash=%s\n", usersTable.name, sourceCount, sourceHash)
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
		placeholders(len(table.columns)),
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

func placeholders(size int) string {
	values := make([]string, 0, size)
	for i := 0; i < size; i++ {
		values = append(values, fmt.Sprintf("$%d", i+1))
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
