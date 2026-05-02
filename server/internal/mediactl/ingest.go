package mediactl

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultStorageDriver     = "local"
	defaultMediaLocalRoot    = "../media/tmp"
	defaultMediaPublicBase   = "http://127.0.0.1:9000/media/tmp"
	defaultObjectKeyPrefix   = "media"
	defaultStoragePathStyle  = "true"
	defaultFFmpegExecutable  = "ffmpeg"
	defaultFFprobeExecutable = "ffprobe"
	defaultHLSSegmentTime    = 6
)

// EnvLookup reads environment values while keeping command parsing testable.
type EnvLookup func(string) string

// StorageConfig is the media storage config shared by future ingest stages.
type StorageConfig struct {
	Driver          string `json:"driver"`
	LocalRoot       string `json:"localRoot"`
	PublicBaseURL   string `json:"publicBaseUrl"`
	ObjectKeyPrefix string `json:"objectKeyPrefix"`
	Endpoint        string `json:"endpoint,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	Region          string `json:"region,omitempty"`
	ForcePathStyle  string `json:"forcePathStyle"`
	FFmpegBin       string `json:"ffmpegBin"`
	FFprobeBin      string `json:"ffprobeBin"`
}

// IngestOptions captures the mediactl ingest contract across dry-run and HLS stages.
type IngestOptions struct {
	MediaID        string        `json:"mediaId,omitempty"`
	Input          string        `json:"input"`
	LibraryRoot    string        `json:"libraryRoot"`
	SourceKey      string        `json:"sourceKey"`
	SourceHash     string        `json:"sourceHash"`
	SeasonSlug     string        `json:"seasonSlug"`
	SeasonNumber   *int          `json:"seasonNumber,omitempty"`
	EpisodeNumber  *int          `json:"episodeNumber,omitempty"`
	Title          string        `json:"title"`
	Subtitle       string        `json:"subtitle,omitempty"`
	Description    string        `json:"description,omitempty"`
	Category       string        `json:"category,omitempty"`
	OriginalTitle  string        `json:"originalTitle,omitempty"`
	ProductionTeam string        `json:"productionTeam,omitempty"`
	SearchAliases  []string      `json:"searchAliases,omitempty"`
	SeasonLabel    string        `json:"seasonLabel,omitempty"`
	EpisodeLabel   string        `json:"episodeLabel,omitempty"`
	Tags           []string      `json:"tags,omitempty"`
	Cover          string        `json:"cover,omitempty"`
	OutputDir      string        `json:"outputDir,omitempty"`
	HLSSegment     int           `json:"hlsSegmentSeconds"`
	Upload         bool          `json:"upload"`
	WriteDB        bool          `json:"writeDb"`
	DryRun         bool          `json:"dryRun"`
	DatabaseURL    string        `json:"-"`
	Storage        StorageConfig `json:"storage"`
}

// IngestSummary reports either planned or completed local ingest work.
type IngestSummary struct {
	IngestOptions
	HLSPlaylistPath string `json:"hlsPlaylistPath,omitempty"`
	MediaURL        string `json:"mediaUrl,omitempty"`
	DurationMs      int64  `json:"durationMs,omitempty"`
	DatabaseUpsert  bool   `json:"databaseUpsert"`
}

// Run executes the mediactl command and returns a process-style exit code.
func Run(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return 2
	}

	switch args[0] {
	case "ingest":
		if err := runIngest(args[1:], getenv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mediactl ingest: %v\n", err)
			return 1
		}
		return 0
	case "-h", "--help", "help":
		printRootUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "mediactl: unknown command %q\n", args[0])
		printRootUsage(stderr)
		return 2
	}
}

// runIngest validates ingest inputs and optionally creates local HLS output.
func runIngest(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	options, err := ParseIngestOptions(args, getenv, stderr)
	if err != nil {
		return err
	}

	summary := IngestSummary{IngestOptions: options}
	if options.DryRun {
		summary.HLSPlaylistPath = plannedPlaylistPath(options)
		summary.MediaURL = plannedMediaURL(options)
		return printIngestSummary(stdout, "mediactl ingest dry-run summary:", summary, "next stages are not implemented yet: upload, database upsert")
	}

	result, err := GenerateHLS(options)
	if err != nil {
		return err
	}
	summary.HLSPlaylistPath = result.PlaylistPath
	summary.DurationMs = result.DurationMs
	summary.MediaURL = publicMediaURL(options)

	if options.WriteDB {
		if err := UpsertMediaMetadata(context.Background(), options, result); err != nil {
			return err
		}
		summary.DatabaseUpsert = true
	}

	return printIngestSummary(stdout, "mediactl ingest completed:", summary, "next stage is not implemented yet: upload")
}

func printIngestSummary(stdout io.Writer, title string, summary IngestSummary, footer string) error {
	encoded, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ingest summary: %w", err)
	}

	fmt.Fprintln(stdout, title)
	fmt.Fprintln(stdout, string(encoded))
	fmt.Fprintln(stdout, footer)
	return nil
}

// ParseIngestOptions parses flags and validates local inputs without side effects.
func ParseIngestOptions(args []string, getenv EnvLookup, stderr io.Writer) (IngestOptions, error) {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var tags string
	var searchAliases string
	options := IngestOptions{DryRun: true, HLSSegment: defaultHLSSegmentTime}
	flags.StringVar(&options.MediaID, "media-id", "", "legacy media id override; normal ingest should use --library-root")
	flags.StringVar(&options.Input, "input", "", "source video file path")
	flags.StringVar(&options.LibraryRoot, "library-root", "", "media library root used to derive source_key from --input")
	flags.StringVar(&options.Title, "title", "", "media title")
	flags.StringVar(&options.Subtitle, "subtitle", "", "media subtitle")
	flags.StringVar(&options.Description, "description", "", "media description")
	flags.StringVar(&options.Category, "category", "", "media category")
	flags.StringVar(&options.OriginalTitle, "original-title", "", "original media title")
	flags.StringVar(&options.ProductionTeam, "production-team", "", "production team or studio")
	flags.StringVar(&options.SeasonLabel, "season-label", "", "season display label")
	flags.StringVar(&options.EpisodeLabel, "episode-label", "", "episode display label")
	flags.StringVar(&tags, "tags", "", "comma-separated tag slugs or names")
	flags.StringVar(&searchAliases, "search-aliases", "", "comma-separated search aliases")
	flags.StringVar(&options.Cover, "cover", "", "optional cover image path")
	flags.StringVar(&options.OutputDir, "output-dir", "", "optional HLS output directory")
	flags.IntVar(&options.HLSSegment, "hls-segment-seconds", defaultHLSSegmentTime, "HLS segment duration in seconds")
	flags.BoolVar(&options.Upload, "upload", false, "request upload in later ingest stages")
	flags.BoolVar(&options.WriteDB, "write-db", false, "upsert media metadata into PostgreSQL after local HLS generation")
	flags.StringVar(&options.DatabaseURL, "database-url", "", "PostgreSQL connection string; falls back to DATABASE_URL")
	flags.BoolVar(&options.DryRun, "dry-run", true, "print planned ingest work without mutating files or database")

	if err := flags.Parse(args); err != nil {
		return IngestOptions{}, err
	}
	if flags.NArg() > 0 {
		return IngestOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	options.Input = strings.TrimSpace(options.Input)
	options.LibraryRoot = strings.TrimSpace(options.LibraryRoot)
	options.MediaID = strings.TrimSpace(options.MediaID)
	options.Title = strings.TrimSpace(options.Title)
	options.Subtitle = strings.TrimSpace(options.Subtitle)
	options.Description = strings.TrimSpace(options.Description)
	options.Category = strings.TrimSpace(options.Category)
	options.OriginalTitle = strings.TrimSpace(options.OriginalTitle)
	options.ProductionTeam = strings.TrimSpace(options.ProductionTeam)
	options.SeasonLabel = strings.TrimSpace(options.SeasonLabel)
	options.EpisodeLabel = strings.TrimSpace(options.EpisodeLabel)
	options.Cover = strings.TrimSpace(options.Cover)
	options.OutputDir = strings.TrimSpace(options.OutputDir)
	options.Tags = splitTags(tags)
	options.SearchAliases = splitTags(searchAliases)
	options.Storage = LoadStorageConfig(getenv)
	if strings.TrimSpace(options.DatabaseURL) == "" {
		options.DatabaseURL = strings.TrimSpace(getenv("DATABASE_URL"))
	}

	if options.Input == "" {
		return IngestOptions{}, errors.New("--input is required")
	}
	if options.LibraryRoot == "" {
		return IngestOptions{}, errors.New("--library-root is required")
	}
	if options.Title == "" {
		return IngestOptions{}, errors.New("--title is required")
	}
	if options.HLSSegment < 4 || options.HLSSegment > 6 {
		return IngestOptions{}, errors.New("--hls-segment-seconds must be between 4 and 6")
	}
	if options.WriteDB && options.DryRun {
		return IngestOptions{}, errors.New("--write-db requires --dry-run=false")
	}
	if options.WriteDB && options.DatabaseURL == "" {
		return IngestOptions{}, errors.New("--write-db requires DATABASE_URL or --database-url")
	}
	if err := requireExistingFile(options.Input, "--input"); err != nil {
		return IngestOptions{}, err
	}
	if err := requireExistingDir(options.LibraryRoot, "--library-root"); err != nil {
		return IngestOptions{}, err
	}
	sourceInfo, err := deriveSourceInfo(options.LibraryRoot, options.Input)
	if err != nil {
		return IngestOptions{}, err
	}
	options.SourceKey = sourceInfo.SourceKey
	options.SeasonSlug = sourceInfo.SeasonSlug
	options.SeasonNumber = sourceInfo.SeasonNumber
	options.EpisodeNumber = sourceInfo.EpisodeNumber
	sourceHash, err := ComputeSourceHash(options.Input)
	if err != nil {
		return IngestOptions{}, err
	}
	options.SourceHash = sourceHash
	if options.Cover != "" {
		if err := requireExistingFile(options.Cover, "--cover"); err != nil {
			return IngestOptions{}, err
		}
	}

	return options, nil
}

// UpsertMediaMetadata writes the ingest result into PostgreSQL episode-backed media tables.
func UpsertMediaMetadata(ctx context.Context, options IngestOptions, result HLSResult) error {
	db, err := sql.Open("pgx", options.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect database: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin media metadata transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	seasonID, err := upsertMediaSeason(ctx, tx, options)
	if err != nil {
		return err
	}
	if err := upsertMediaEpisode(ctx, tx, seasonID, options, result); err != nil {
		return err
	}
	if err := replaceMediaSeasonTags(ctx, tx, seasonID, options.Tags); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit media metadata transaction: %w", err)
	}
	return nil
}

func upsertMediaSeason(ctx context.Context, tx *sql.Tx, options IngestOptions) (string, error) {
	const query = `
		INSERT INTO media_seasons (
			slug,
			title,
			description,
			cover_url,
			category,
			original_title,
			production_team,
			search_aliases,
			season_number,
			season_label,
			status
		)
		VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			NULLIF($6, ''), NULLIF($7, ''), $8::jsonb, $9, NULLIF($10, ''), 'active'
		)
		ON CONFLICT (slug) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			cover_url = COALESCE(EXCLUDED.cover_url, media_seasons.cover_url),
			category = EXCLUDED.category,
			status = EXCLUDED.status,
			original_title = EXCLUDED.original_title,
			production_team = EXCLUDED.production_team,
			search_aliases = EXCLUDED.search_aliases,
			season_number = EXCLUDED.season_number,
			season_label = EXCLUDED.season_label,
			updated_at = NOW()
		RETURNING id::text
	`

	searchAliasesJSON, err := json.Marshal(options.SearchAliases)
	if err != nil {
		return "", fmt.Errorf("encode search aliases json: %w", err)
	}

	var seasonID string
	if err := tx.QueryRowContext(
		ctx,
		query,
		options.SeasonSlug,
		options.Title,
		options.Description,
		"", // cover URL is added by uploader stage.
		options.Category,
		options.OriginalTitle,
		options.ProductionTeam,
		string(searchAliasesJSON),
		nullableInt64(options.SeasonNumber),
		options.SeasonLabel,
	).Scan(&seasonID); err != nil {
		return "", fmt.Errorf("upsert media season: %w", err)
	}
	return seasonID, nil
}

func upsertMediaEpisode(ctx context.Context, tx *sql.Tx, seasonID string, options IngestOptions, result HLSResult) error {
	const query = `
		INSERT INTO media_episodes (
			season_id,
			title,
			subtitle,
			description,
			cover_url,
			media_url,
			duration_ms,
			episode_number,
			episode_label,
			source_key,
			source_hash,
			status
		)
		VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''),
			$6, $7, $8, NULLIF($9, ''), $10, $11, 'active'
		)
		ON CONFLICT (source_key) DO UPDATE SET
			season_id = EXCLUDED.season_id,
			title = EXCLUDED.title,
			subtitle = EXCLUDED.subtitle,
			description = EXCLUDED.description,
			cover_url = COALESCE(EXCLUDED.cover_url, media_episodes.cover_url),
			media_url = EXCLUDED.media_url,
			duration_ms = EXCLUDED.duration_ms,
			episode_number = EXCLUDED.episode_number,
			episode_label = EXCLUDED.episode_label,
			source_hash = EXCLUDED.source_hash,
			status = EXCLUDED.status,
			updated_at = NOW()
	`
	if _, err := tx.ExecContext(
		ctx,
		query,
		seasonID,
		options.Title,
		options.Subtitle,
		options.Description,
		"", // cover URL is added by uploader stage.
		publicMediaURL(options),
		result.DurationMs,
		nullableInt64(options.EpisodeNumber),
		options.EpisodeLabel,
		options.SourceKey,
		options.SourceHash,
	); err != nil {
		return fmt.Errorf("upsert media episode: %w", err)
	}
	return nil
}

func replaceMediaSeasonTags(ctx context.Context, tx *sql.Tx, seasonID string, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_season_tags WHERE season_id = $1`, seasonID); err != nil {
		return fmt.Errorf("clear media season tags: %w", err)
	}
	for order, rawTag := range tags {
		slug := normalizeTagSlug(rawTag)
		if slug == "" {
			continue
		}
		tagID, err := upsertMediaTag(ctx, tx, slug, rawTag, order)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO media_season_tags (season_id, media_tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			seasonID,
			tagID,
		); err != nil {
			return fmt.Errorf("link media tag %q: %w", slug, err)
		}
	}
	return nil
}

func upsertMediaTag(ctx context.Context, tx *sql.Tx, slug string, name string, order int) (string, error) {
	const query = `
		INSERT INTO media_tags (slug, name, sort_order, is_featured, is_active)
		VALUES ($1, $2, $3, false, true)
		ON CONFLICT (slug) DO UPDATE SET
			name = EXCLUDED.name,
			is_active = true,
			updated_at = NOW()
		RETURNING id::text
	`
	var id string
	if err := tx.QueryRowContext(ctx, query, slug, strings.TrimSpace(name), 1000+order).Scan(&id); err != nil {
		return "", fmt.Errorf("upsert media tag %q: %w", slug, err)
	}
	return id, nil
}

// LoadStorageConfig resolves the INT-137 runtime config with local defaults.
func LoadStorageConfig(getenv EnvLookup) StorageConfig {
	return StorageConfig{
		Driver:          envOrDefault(getenv, "MEDIA_STORAGE_DRIVER", defaultStorageDriver),
		LocalRoot:       envOrDefault(getenv, "MEDIA_LOCAL_ROOT", defaultMediaLocalRoot),
		PublicBaseURL:   envOrDefault(getenv, "MEDIA_PUBLIC_BASE_URL", defaultMediaPublicBase),
		ObjectKeyPrefix: envOrDefault(getenv, "MEDIA_OBJECT_KEY_PREFIX", defaultObjectKeyPrefix),
		Endpoint:        strings.TrimSpace(getenv("MEDIA_STORAGE_ENDPOINT")),
		Bucket:          strings.TrimSpace(getenv("MEDIA_STORAGE_BUCKET")),
		Region:          strings.TrimSpace(getenv("MEDIA_STORAGE_REGION")),
		ForcePathStyle:  envOrDefault(getenv, "MEDIA_STORAGE_FORCE_PATH_STYLE", defaultStoragePathStyle),
		FFmpegBin:       envOrDefault(getenv, "FFMPEG_BIN", defaultFFmpegExecutable),
		FFprobeBin:      envOrDefault(getenv, "FFPROBE_BIN", defaultFFprobeExecutable),
	}
}

// HLSResult is the local ffmpeg output that later stages can upload or persist.
type HLSResult struct {
	OutputDir    string
	PlaylistPath string
	DurationMs   int64
}

// GenerateHLS creates a single-bitrate VOD HLS output with ffmpeg.
func GenerateHLS(options IngestOptions) (HLSResult, error) {
	outputDir := resolveOutputDir(options)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return HLSResult{}, fmt.Errorf("create hls output directory: %w", err)
	}

	playlistPath := filepath.Join(outputDir, "index.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment_%05d.ts")
	args := BuildFFmpegHLSArgs(options.Input, playlistPath, segmentPattern, options.HLSSegment)

	cmd := exec.Command(options.Storage.FFmpegBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return HLSResult{}, fmt.Errorf("ffmpeg hls generation failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	durationMs, err := ProbeDurationMs(options.Storage.FFprobeBin, options.Input)
	if err != nil {
		return HLSResult{}, err
	}

	return HLSResult{
		OutputDir:    outputDir,
		PlaylistPath: playlistPath,
		DurationMs:   durationMs,
	}, nil
}

// BuildFFmpegHLSArgs returns the single-bitrate HLS command used by INT-139.
func BuildFFmpegHLSArgs(input string, playlistPath string, segmentPattern string, segmentSeconds int) []string {
	return []string{
		"-y",
		"-i", input,
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-pix_fmt", "yuv420p",
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segmentSeconds),
		"-sc_threshold", "0",
		"-c:a", "aac",
		"-b:a", "128k",
		"-hls_time", strconv.Itoa(segmentSeconds),
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	}
}

// ProbeDurationMs reads source media duration in milliseconds with ffprobe.
func ProbeDurationMs(ffprobeBin string, input string) (int64, error) {
	cmd := exec.Command(
		ffprobeBin,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		input,
	)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration failed: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", strings.TrimSpace(string(output)), err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("ffprobe duration must be positive, got %f", seconds)
	}
	return int64(math.Round(seconds * 1000)), nil
}

func resolveOutputDir(options IngestOptions) string {
	if options.OutputDir != "" {
		return options.OutputDir
	}
	return filepath.Join(options.Storage.LocalRoot, sourceObjectKey(options), "hls")
}

func plannedPlaylistPath(options IngestOptions) string {
	if options.OutputDir == "" && options.MediaID == "" {
		return ""
	}
	return filepath.Join(resolveOutputDir(options), "index.m3u8")
}

func publicMediaURL(options IngestOptions) string {
	return joinURLPath(options.Storage.PublicBaseURL, sourceObjectKey(options), "hls", "index.m3u8")
}

func plannedMediaURL(options IngestOptions) string {
	if options.SourceKey == "" {
		return ""
	}
	return publicMediaURL(options)
}

func sourceObjectKey(options IngestOptions) string {
	if options.MediaID != "" {
		return filepath.ToSlash(filepath.Join(options.Storage.ObjectKeyPrefix, options.MediaID))
	}
	sourceKey := strings.TrimSuffix(options.SourceKey, filepath.Ext(options.SourceKey))
	return filepath.ToSlash(filepath.Join(options.Storage.ObjectKeyPrefix, sourceKey))
}

func joinURLPath(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for index, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if index == 0 {
			cleaned = append(cleaned, strings.TrimRight(trimmed, "/"))
			continue
		}
		cleaned = append(cleaned, strings.Trim(trimmed, "/"))
	}
	return strings.Join(cleaned, "/")
}

func normalizeTagSlug(tag string) string {
	slug := strings.ToLower(strings.TrimSpace(tag))
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

func splitTags(raw string) []string {
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

type sourceInfo struct {
	SourceKey     string
	SeasonSlug    string
	SeasonNumber  *int
	EpisodeNumber *int
}

func deriveSourceInfo(libraryRoot string, input string) (sourceInfo, error) {
	absRoot, err := filepath.Abs(libraryRoot)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("resolve --library-root: %w", err)
	}
	absInput, err := filepath.Abs(input)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("resolve --input: %w", err)
	}
	relative, err := filepath.Rel(absRoot, absInput)
	if err != nil {
		return sourceInfo{}, fmt.Errorf("derive source key: %w", err)
	}
	if relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." || filepath.IsAbs(relative) {
		return sourceInfo{}, fmt.Errorf("--input must be inside --library-root")
	}
	sourceKey := filepath.ToSlash(filepath.Clean(relative))
	parts := strings.Split(sourceKey, "/")
	if len(parts) < 3 {
		return sourceInfo{}, fmt.Errorf("source path must follow <season-slug>/season-XX/episode-XX.ext")
	}
	for _, part := range parts {
		if !isSafeSourcePathComponent(part) {
			return sourceInfo{}, fmt.Errorf("source path component %q must use lowercase letters, numbers, dot, dash or underscore", part)
		}
	}
	seasonSlug := normalizePathSlug(parts[0])
	if seasonSlug == "" {
		return sourceInfo{}, fmt.Errorf("source path season slug is invalid")
	}
	return sourceInfo{
		SourceKey:     sourceKey,
		SeasonSlug:    seasonSlug,
		SeasonNumber:  parseNumberAfterPrefix(parts[1], "season-"),
		EpisodeNumber: parseNumberAfterPrefix(strings.TrimSuffix(filepath.Base(sourceKey), filepath.Ext(sourceKey)), "episode-"),
	}, nil
}

func isSafeSourcePathComponent(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char >= '0' && char <= '9' {
			continue
		}
		if char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func ComputeSourceHash(input string) (string, error) {
	file, err := os.Open(input)
	if err != nil {
		return "", fmt.Errorf("open source for hash: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash source file: %w", err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizePathSlug(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = strings.ReplaceAll(slug, "_", "-")
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}

func parseNumberAfterPrefix(value string, prefix string) *int {
	normalized := strings.ToLower(strings.TrimSpace(value))
	trimmed := strings.TrimPrefix(normalized, prefix)
	if trimmed == normalized || trimmed == "" {
		return nil
	}
	number, err := strconv.Atoi(trimmed)
	if err != nil || number <= 0 {
		return nil
	}
	return &number
}

func nullableInt64(value *int) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*value), Valid: true}
}

func requireExistingFile(path string, flagName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s file is not accessible: %w", flagName, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s must point to a file, got directory %q", flagName, path)
	}
	return nil
}

func requireExistingDir(path string, flagName string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s directory is not accessible: %w", flagName, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s must point to a directory, got file %q", flagName, path)
	}
	return nil
}

func envOrDefault(getenv EnvLookup, name string, fallback string) string {
	value := strings.TrimSpace(getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: mediactl <command>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  ingest    validate a media ingest request and print a dry-run summary")
}
