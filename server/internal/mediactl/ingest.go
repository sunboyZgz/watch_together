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
	defaultRenditions        = "720p-fast,720p-high,1080p"
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
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"-"`
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
	Renditions     []string      `json:"renditions"`
	WriteDB        bool          `json:"writeDb"`
	DryRun         bool          `json:"dryRun"`
	DatabaseURL    string        `json:"-"`
	Storage        StorageConfig `json:"storage"`
}

// IngestSummary reports either planned or completed local ingest work.
type IngestSummary struct {
	IngestOptions
	HLSPlaylistPath string       `json:"hlsPlaylistPath,omitempty"`
	MediaURL        string       `json:"mediaUrl,omitempty"`
	CoverURL        string       `json:"coverUrl,omitempty"`
	DurationMs      int64        `json:"durationMs,omitempty"`
	Variants        []HLSVariant `json:"variants,omitempty"`
	DatabaseUpsert  bool         `json:"databaseUpsert"`
}

type mediactlStage string

const (
	stagePlan    mediactlStage = "plan"
	stageBuild   mediactlStage = "build-hls"
	stageUpload  mediactlStage = "upload"
	stageWriteDB mediactlStage = "write-db"
	stageIngest  mediactlStage = "ingest"
)

// Run executes the mediactl command and returns a process-style exit code.
func Run(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printRootUsage(stderr)
		return 2
	}

	switch args[0] {
	case "plan":
		if err := runPlan(args[1:], getenv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mediactl plan: %v\n", err)
			return 1
		}
		return 0
	case "build-hls":
		if err := runBuildHLS(args[1:], getenv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mediactl build-hls: %v\n", err)
			return 1
		}
		return 0
	case "upload":
		if err := runUpload(args[1:], getenv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mediactl upload: %v\n", err)
			return 1
		}
		return 0
	case "write-db":
		if err := runWriteDB(args[1:], getenv, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mediactl write-db: %v\n", err)
			return 1
		}
		return 0
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

func runPlan(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	options, err := parseIngestOptionsForStage(stagePlan, args, getenv, stderr)
	if err != nil {
		return err
	}

	summary := IngestSummary{IngestOptions: options}
	summary.HLSPlaylistPath = plannedPlaylistPath(options)
	summary.MediaURL = plannedMediaURL(options)
	summary.CoverURL = plannedCoverURL(options)
	return printIngestSummary(stdout, "mediactl plan summary:", summary, "plan only validates inputs and prints the expected HLS / storage layout")
}

func runBuildHLS(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	options, err := parseIngestOptionsForStage(stageBuild, args, getenv, stderr)
	if err != nil {
		return err
	}

	result, err := GenerateHLS(options)
	if err != nil {
		return err
	}
	summary := buildSummary(options, result)
	return printIngestSummary(stdout, "mediactl build-hls completed:", summary, "HLS assets were generated locally; upload and database upsert were skipped")
}

func runUpload(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	options, err := parseIngestOptionsForStage(stageUpload, args, getenv, stderr)
	if err != nil {
		return err
	}

	result, err := LoadExistingHLSResult(options)
	if err != nil {
		return err
	}
	result, err = UploadIngestAssets(context.Background(), options, result)
	if err != nil {
		return err
	}
	summary := buildSummary(options, result)
	return printIngestSummary(stdout, "mediactl upload completed:", summary, uploadCompletionFooter(options))
}

func runWriteDB(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	options, err := parseIngestOptionsForStage(stageWriteDB, args, getenv, stderr)
	if err != nil {
		return err
	}

	result, err := LoadExistingHLSResult(options)
	if err != nil {
		return err
	}
	if err := UpsertMediaMetadata(context.Background(), options, result); err != nil {
		return err
	}
	summary := buildSummary(options, result)
	summary.DatabaseUpsert = true
	return printIngestSummary(stdout, "mediactl write-db completed:", summary, "episode-backed media metadata was upserted; rerunning write-db updates the same season and source_key rows")
}

// runIngest validates ingest inputs and optionally creates local HLS output.
func runIngest(args []string, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	requestedStages, hasRequestedStages, err := parseRequestedStageSequence(args)
	if err != nil {
		return err
	}
	if hasRequestedStages {
		return runComposedIngest(args, requestedStages, getenv, stdout, stderr)
	}

	options, err := ParseIngestOptions(args, getenv, stderr)
	if err != nil {
		return err
	}

	if options.DryRun {
		summary := IngestSummary{IngestOptions: options}
		summary.HLSPlaylistPath = plannedPlaylistPath(options)
		summary.MediaURL = plannedMediaURL(options)
		summary.CoverURL = plannedCoverURL(options)
		return printIngestSummary(stdout, "mediactl ingest dry-run summary:", summary, "dry-run does not generate files, upload assets, or write database rows")
	}

	result, err := GenerateHLS(options)
	if err != nil {
		return err
	}
	result, err = UploadIngestAssets(context.Background(), options, result)
	if err != nil {
		return err
	}
	summary := buildSummary(options, result)

	if options.WriteDB {
		if err := UpsertMediaMetadata(context.Background(), options, result); err != nil {
			return err
		}
		summary.DatabaseUpsert = true
	}

	return printIngestSummary(stdout, "mediactl ingest completed:", summary, ingestCompletionFooter(options))
}

func runComposedIngest(args []string, stages []mediactlStage, getenv EnvLookup, stdout io.Writer, stderr io.Writer) error {
	parseStage := dominantValidationStage(stages)
	filteredArgs := stripStagesArgs(args)
	options, err := parseIngestOptionsForStage(parseStage, filteredArgs, getenv, stderr)
	if err != nil {
		return err
	}
	return executeStageSequence("mediactl ingest staged pipeline completed:", stages, options, stdout)
}

func executeStageSequence(title string, stages []mediactlStage, options IngestOptions, stdout io.Writer) error {
	if len(stages) == 1 && stages[0] == stagePlan {
		summary := IngestSummary{IngestOptions: options}
		summary.HLSPlaylistPath = plannedPlaylistPath(options)
		summary.MediaURL = plannedMediaURL(options)
		summary.CoverURL = plannedCoverURL(options)
		return printIngestSummary(stdout, title, summary, "plan only validates inputs and prints the expected HLS / storage layout")
	}

	var (
		result HLSResult
		err    error
		loaded bool
	)

	for _, stage := range stages {
		switch stage {
		case stageBuild:
			result, err = GenerateHLS(options)
			if err != nil {
				return err
			}
			result = ApplyIngestPublicURLs(options, result)
			loaded = true
		case stageUpload:
			if !loaded {
				result, err = LoadExistingHLSResult(options)
				if err != nil {
					return err
				}
				loaded = true
			}
			result, err = UploadIngestAssets(context.Background(), options, result)
			if err != nil {
				return err
			}
		case stageWriteDB:
			if !loaded {
				result, err = LoadExistingHLSResult(options)
				if err != nil {
					return err
				}
				loaded = true
			}
			result = ApplyIngestPublicURLs(options, result)
			if err := UpsertMediaMetadata(context.Background(), options, result); err != nil {
				return err
			}
		}
	}

	summary := buildSummary(options, result)
	if containsStage(stages, stageWriteDB) {
		summary.DatabaseUpsert = true
	}
	footer := fmt.Sprintf("%s Stages executed in dependency order: %s.", stageSequenceFooterPrefix(options, stages), joinStageNames(stages))
	return printIngestSummary(stdout, title, summary, footer)
}

func buildSummary(options IngestOptions, result HLSResult) IngestSummary {
	return IngestSummary{
		IngestOptions:   options,
		HLSPlaylistPath: result.PlaylistPath,
		MediaURL:        result.MediaURL,
		CoverURL:        result.CoverURL,
		DurationMs:      result.DurationMs,
		Variants:        result.Variants,
	}
}

func parseRequestedStageSequence(args []string) ([]mediactlStage, bool, error) {
	raw := ""
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "--stages" {
			if index+1 >= len(args) {
				return nil, true, errors.New("--stages requires a comma-separated value")
			}
			raw = args[index+1]
			break
		}
		if strings.HasPrefix(arg, "--stages=") {
			raw = strings.TrimPrefix(arg, "--stages=")
			break
		}
	}
	if raw == "" {
		return nil, false, nil
	}
	stages, err := parseStageSequence(raw)
	if err != nil {
		return nil, true, err
	}
	return stages, true, nil
}

func stripStagesArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := strings.TrimSpace(args[index])
		if arg == "--stages" {
			index++
			continue
		}
		if strings.HasPrefix(arg, "--stages=") {
			continue
		}
		filtered = append(filtered, args[index])
	}
	return filtered
}

func parseStageSequence(raw string) ([]mediactlStage, error) {
	parts := splitTags(raw)
	if len(parts) == 0 {
		return nil, errors.New("--stages must include at least one stage")
	}

	seen := map[mediactlStage]struct{}{}
	containsPlan := false
	containsMutation := false
	for _, part := range parts {
		stage, err := normalizeStageName(part)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[stage]; exists {
			continue
		}
		seen[stage] = struct{}{}
		if stage == stagePlan {
			containsPlan = true
		} else {
			containsMutation = true
		}
	}
	if containsPlan && containsMutation {
		return nil, errors.New("--stages cannot mix plan with mutating stages")
	}

	ordered := make([]mediactlStage, 0, len(seen))
	for _, candidate := range []mediactlStage{stagePlan, stageBuild, stageUpload, stageWriteDB} {
		if _, ok := seen[candidate]; ok {
			ordered = append(ordered, candidate)
		}
	}
	return ordered, nil
}

func normalizeStageName(raw string) (mediactlStage, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(stagePlan):
		return stagePlan, nil
	case string(stageBuild), "build", "transcode":
		return stageBuild, nil
	case string(stageUpload):
		return stageUpload, nil
	case string(stageWriteDB), "db", "write":
		return stageWriteDB, nil
	default:
		return "", fmt.Errorf("unsupported stage %q; supported values are plan, build-hls, upload, write-db", raw)
	}
}

func dominantValidationStage(stages []mediactlStage) mediactlStage {
	if containsStage(stages, stageBuild) {
		return stageBuild
	}
	if containsStage(stages, stageWriteDB) {
		return stageWriteDB
	}
	if containsStage(stages, stageUpload) {
		return stageUpload
	}
	return stagePlan
}

func containsStage(stages []mediactlStage, want mediactlStage) bool {
	for _, stage := range stages {
		if stage == want {
			return true
		}
	}
	return false
}

func joinStageNames(stages []mediactlStage) string {
	names := make([]string, 0, len(stages))
	for _, stage := range stages {
		names = append(names, string(stage))
	}
	return strings.Join(names, " -> ")
}

func uploadCompletionFooter(options IngestOptions) string {
	switch normalizedStorageDriver(options.Storage.Driver) {
	case "minio", "s3":
		return fmt.Sprintf(
			"HLS assets were uploaded to %s bucket %q under %q. Re-running upload will overwrite the same object keys instead of creating duplicates.",
			normalizedStorageDriver(options.Storage.Driver),
			options.Storage.Bucket,
			sourceObjectKey(options),
		)
	default:
		return fmt.Sprintf(
			"HLS assets were stored locally at %s. Re-running upload will overwrite the same stable paths instead of creating duplicates.",
			filepath.Join(options.Storage.LocalRoot, sourceObjectKey(options), "hls"),
		)
	}
}

func ingestCompletionFooter(options IngestOptions) string {
	switch normalizedStorageDriver(options.Storage.Driver) {
	case "minio", "s3":
		return fmt.Sprintf(
			"HLS assets were generated locally and uploaded to %s bucket %q under %q.",
			normalizedStorageDriver(options.Storage.Driver),
			options.Storage.Bucket,
			sourceObjectKey(options),
		)
	default:
		return fmt.Sprintf(
			"HLS assets were generated and stored locally at %s, and are ready for playback.",
			resolveOutputDir(options),
		)
	}
}

func stageSequenceFooterPrefix(options IngestOptions, stages []mediactlStage) string {
	if containsStage(stages, stageUpload) {
		return uploadCompletionFooter(options)
	}
	if containsStage(stages, stageBuild) {
		return ingestCompletionFooter(options)
	}
	if containsStage(stages, stageWriteDB) {
		return "Media metadata was upserted successfully."
	}
	return "Staged pipeline completed successfully."
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
	return parseIngestOptionsForStage(stageIngest, args, getenv, stderr)
}

func parseIngestOptionsForStage(stage mediactlStage, args []string, getenv EnvLookup, stderr io.Writer) (IngestOptions, error) {
	flags := flag.NewFlagSet("ingest", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var tags string
	var searchAliases string
	var renditions string
	options := IngestOptions{DryRun: stage == stageIngest || stage == stagePlan, HLSSegment: defaultHLSSegmentTime}
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
	flags.StringVar(&renditions, "renditions", defaultRenditions, "comma-separated HLS renditions; supported values: 720p-fast,720p-high,720p,1080p")
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
	options.Renditions = splitTags(renditions)
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
	if stage != stageUpload && options.Title == "" {
		return IngestOptions{}, errors.New("--title is required")
	}
	if options.HLSSegment < 4 || options.HLSSegment > 6 {
		return IngestOptions{}, errors.New("--hls-segment-seconds must be between 4 and 6")
	}
	if len(options.Renditions) == 0 {
		return IngestOptions{}, errors.New("--renditions must include at least one rendition")
	}
	if !isSupportedStorageDriver(options.Storage.Driver) {
		return IngestOptions{}, fmt.Errorf("unsupported MEDIA_STORAGE_DRIVER %q; supported values are local, minio, s3", options.Storage.Driver)
	}
	if _, err := ResolveRenditionSpecs(options.Renditions); err != nil {
		return IngestOptions{}, err
	}
	switch stage {
	case stagePlan:
		if !options.DryRun {
			return IngestOptions{}, errors.New("plan only supports dry-run validation")
		}
	case stageBuild, stageUpload, stageWriteDB:
		if options.DryRun {
			return IngestOptions{}, fmt.Errorf("%s requires --dry-run=false", stage)
		}
	case stageIngest:
		if options.WriteDB && options.DryRun {
			return IngestOptions{}, errors.New("--write-db requires --dry-run=false")
		}
	}
	if (stage == stageWriteDB || options.WriteDB) && options.DatabaseURL == "" {
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

	seasonID, err := upsertMediaSeason(ctx, tx, options, result)
	if err != nil {
		return err
	}
	episodeID, err := upsertMediaEpisode(ctx, tx, seasonID, options, result)
	if err != nil {
		return err
	}
	if err := replaceMediaEpisodeVariants(ctx, tx, episodeID, result.Variants); err != nil {
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

func upsertMediaSeason(ctx context.Context, tx *sql.Tx, options IngestOptions, result HLSResult) (string, error) {
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
		result.CoverURL,
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

func upsertMediaEpisode(ctx context.Context, tx *sql.Tx, seasonID string, options IngestOptions, result HLSResult) (string, error) {
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
		RETURNING id::text
	`
	var episodeID string
	if err := tx.QueryRowContext(
		ctx,
		query,
		seasonID,
		options.Title,
		options.Subtitle,
		options.Description,
		result.CoverURL,
		result.MediaURL,
		result.DurationMs,
		nullableInt64(options.EpisodeNumber),
		options.EpisodeLabel,
		options.SourceKey,
		options.SourceHash,
	).Scan(&episodeID); err != nil {
		return "", fmt.Errorf("upsert media episode: %w", err)
	}
	return episodeID, nil
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

func replaceMediaEpisodeVariants(ctx context.Context, tx *sql.Tx, episodeID string, variants []HLSVariant) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_episode_variants WHERE media_episode_id = $1`, episodeID); err != nil {
		return fmt.Errorf("clear media episode variants: %w", err)
	}
	for _, variant := range variants {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO media_episode_variants (
				media_episode_id,
				variant_key,
				label,
				playlist_url,
				width,
				height,
				bandwidth_bps,
				codecs,
				segment_count,
				average_segment_ms,
				is_default,
				sort_order
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			episodeID,
			variant.Key,
			variant.Label,
			variant.PlaylistURL,
			nullableInt(variant.Width),
			nullableInt(variant.Height),
			nullableInt(variant.BandwidthBps),
			nullableString(variant.Codecs),
			nullableInt(variant.Segments),
			nullableInt64Value(variant.AverageSegmentMs),
			variant.IsDefault,
			variant.SortOrder,
		); err != nil {
			return fmt.Errorf("insert media episode variant %q: %w", variant.Key, err)
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
		AccessKeyID:     strings.TrimSpace(getenv("MEDIA_STORAGE_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(getenv("MEDIA_STORAGE_SECRET_ACCESS_KEY")),
		ForcePathStyle:  envOrDefault(getenv, "MEDIA_STORAGE_FORCE_PATH_STYLE", defaultStoragePathStyle),
		FFmpegBin:       envOrDefault(getenv, "FFMPEG_BIN", defaultFFmpegExecutable),
		FFprobeBin:      envOrDefault(getenv, "FFPROBE_BIN", defaultFFprobeExecutable),
	}
}

// HLSResult is the local ffmpeg output that later stages can upload or persist.
type HLSResult struct {
	OutputDir    string
	PlaylistPath string
	MediaURL     string
	CoverURL     string
	DurationMs   int64
	Variants     []HLSVariant
}

// HLSVariant records one generated rendition under the episode master playlist.
type HLSVariant struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	PlaylistPath     string `json:"playlistPath,omitempty"`
	PlaylistURL      string `json:"playlistUrl"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	BandwidthBps     int    `json:"bandwidthBps,omitempty"`
	Codecs           string `json:"codecs,omitempty"`
	Segments         int    `json:"segments,omitempty"`
	AverageSegmentMs int64  `json:"averageSegmentMs,omitempty"`
	IsDefault        bool   `json:"isDefault"`
	SortOrder        int    `json:"sortOrder"`
}

// RenditionSpec describes one supported HLS output ladder rung.
type RenditionSpec struct {
	Key            string
	Label          string
	TargetHeight   int
	BandwidthBps   int
	CRF            string
	MaxRate        string
	BufSize        string
	Profile        string
	Level          string
	Codecs         string
	Tune           string
	DisableBFrames bool
	IsDefault      bool
	SortOrder      int
}

// VideoMetadata is the small ffprobe result needed for quality checks.
type VideoMetadata struct {
	Width  int
	Height int
}

// HLSHealth summarizes the generated VOD playlist's segment structure.
type HLSHealth struct {
	Segments         int
	AverageSegmentMs int64
	MaxSegmentMs     int64
	MinSegmentMs     int64
}

// LoadExistingHLSResult rebuilds a typed result from an already generated local HLS directory.
func LoadExistingHLSResult(options IngestOptions) (HLSResult, error) {
	outputDir := resolveOutputDir(options)
	playlistPath := filepath.Join(outputDir, "master.m3u8")
	if err := requireExistingFile(playlistPath, "generated HLS master playlist"); err != nil {
		return HLSResult{}, err
	}

	durationMs, err := ProbeDurationMs(options.Storage.FFprobeBin, options.Input)
	if err != nil {
		return HLSResult{}, err
	}
	variants, err := discoverGeneratedVariants(options, outputDir)
	if err != nil {
		return HLSResult{}, err
	}

	return HLSResult{
		OutputDir:    outputDir,
		PlaylistPath: playlistPath,
		MediaURL:     publicMediaURL(options),
		CoverURL:     plannedCoverURL(options),
		DurationMs:   durationMs,
		Variants:     variants,
	}, nil
}

func discoverGeneratedVariants(options IngestOptions, outputDir string) ([]HLSVariant, error) {
	supported := supportedRenditionSpecs()
	variants := make([]HLSVariant, 0, len(supported))
	for _, key := range orderedSupportedRenditionKeys() {
		spec := supported[key]
		playlistPath := filepath.Join(outputDir, spec.Key, "index.m3u8")
		if _, err := os.Stat(playlistPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect generated %s playlist: %w", spec.Key, err)
		}
		metadata, err := ProbeVideoMetadata(options.Storage.FFprobeBin, playlistPath)
		if err != nil {
			return nil, fmt.Errorf("probe generated %s playlist: %w", spec.Key, err)
		}
		health, err := AnalyzeHLSPlaylist(playlistPath)
		if err != nil {
			return nil, fmt.Errorf("analyze generated %s playlist: %w", spec.Key, err)
		}
		variants = append(variants, HLSVariant{
			Key:              spec.Key,
			Label:            spec.Label,
			PlaylistPath:     playlistPath,
			PlaylistURL:      publicVariantURL(options, spec.Key),
			Width:            metadata.Width,
			Height:           metadata.Height,
			BandwidthBps:     spec.BandwidthBps,
			Codecs:           spec.Codecs,
			Segments:         health.Segments,
			AverageSegmentMs: health.AverageSegmentMs,
			IsDefault:        spec.IsDefault,
			SortOrder:        spec.SortOrder,
		})
	}
	if len(variants) == 0 {
		return nil, errors.New("generated HLS output has no recognized variant playlists")
	}
	hasDefault := false
	for _, variant := range variants {
		if variant.IsDefault {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		variants[0].IsDefault = true
	}
	return variants, nil
}

// GenerateHLS creates a multi-rendition VOD HLS output with a master playlist.
func GenerateHLS(options IngestOptions) (HLSResult, error) {
	outputDir := resolveOutputDir(options)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return HLSResult{}, fmt.Errorf("create hls output directory: %w", err)
	}

	sourceMetadata, err := ProbeVideoMetadata(options.Storage.FFprobeBin, options.Input)
	if err != nil {
		return HLSResult{}, err
	}
	specs, err := ResolveRenditionSpecs(options.Renditions)
	if err != nil {
		return HLSResult{}, err
	}
	specs, err = SelectRenditionSpecsForSource(sourceMetadata, specs)
	if err != nil {
		return HLSResult{}, err
	}

	variants := make([]HLSVariant, 0, len(specs))
	for _, spec := range specs {
		variantDir := filepath.Join(outputDir, spec.Key)
		if err := os.MkdirAll(variantDir, 0o755); err != nil {
			return HLSResult{}, fmt.Errorf("create hls variant directory %q: %w", spec.Key, err)
		}
		playlistPath := filepath.Join(variantDir, "index.m3u8")
		segmentPattern := filepath.Join(variantDir, "segment_%05d.ts")
		args := BuildFFmpegVariantHLSArgs(options.Input, playlistPath, segmentPattern, options.HLSSegment, spec)

		cmd := exec.Command(options.Storage.FFmpegBin, args...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return HLSResult{}, fmt.Errorf("ffmpeg hls generation failed for %s: %w: %s", spec.Key, err, strings.TrimSpace(string(output)))
		}

		variantMetadata, err := ProbeVideoMetadata(options.Storage.FFprobeBin, playlistPath)
		if err != nil {
			return HLSResult{}, fmt.Errorf("probe generated %s playlist: %w", spec.Key, err)
		}
		if err := ValidateGeneratedRendition(sourceMetadata, variantMetadata, spec); err != nil {
			return HLSResult{}, err
		}
		health, err := AnalyzeHLSPlaylist(playlistPath)
		if err != nil {
			return HLSResult{}, fmt.Errorf("analyze generated %s playlist: %w", spec.Key, err)
		}
		if err := ValidateHLSHealth(health, options.HLSSegment, spec); err != nil {
			return HLSResult{}, err
		}
		variants = append(variants, HLSVariant{
			Key:              spec.Key,
			Label:            spec.Label,
			PlaylistPath:     playlistPath,
			Width:            variantMetadata.Width,
			Height:           variantMetadata.Height,
			BandwidthBps:     spec.BandwidthBps,
			Codecs:           spec.Codecs,
			Segments:         health.Segments,
			AverageSegmentMs: health.AverageSegmentMs,
			IsDefault:        spec.IsDefault,
			SortOrder:        spec.SortOrder,
		})
	}

	masterPlaylistPath := filepath.Join(outputDir, "master.m3u8")
	if err := WriteMasterPlaylist(masterPlaylistPath, variants); err != nil {
		return HLSResult{}, err
	}

	durationMs, err := ProbeDurationMs(options.Storage.FFprobeBin, options.Input)
	if err != nil {
		return HLSResult{}, err
	}

	return HLSResult{
		OutputDir:    outputDir,
		PlaylistPath: masterPlaylistPath,
		DurationMs:   durationMs,
		Variants:     variants,
	}, nil
}

func supportedRenditionSpecs() map[string]RenditionSpec {
	return map[string]RenditionSpec{
		"720p-fast": {
			Key:            "720p-fast",
			Label:          "720p Fast",
			TargetHeight:   720,
			BandwidthBps:   1_600_000,
			CRF:            "25",
			MaxRate:        "1800k",
			BufSize:        "3600k",
			Profile:        "main",
			Level:          "3.1",
			Codecs:         "avc1.4d401f,mp4a.40.2",
			Tune:           "fastdecode",
			DisableBFrames: true,
			IsDefault:      true,
			SortOrder:      0,
		},
		"720p-high": {
			Key:          "720p-high",
			Label:        "720p High",
			TargetHeight: 720,
			BandwidthBps: 2_800_000,
			CRF:          "22",
			MaxRate:      "3000k",
			BufSize:      "6000k",
			Profile:      "high",
			Level:        "3.1",
			Codecs:       "avc1.64001f,mp4a.40.2",
			SortOrder:    1,
		},
		"1080p": {
			Key:          "1080p",
			Label:        "1080p",
			TargetHeight: 1080,
			BandwidthBps: 5_000_000,
			CRF:          "22",
			MaxRate:      "5200k",
			BufSize:      "10400k",
			Profile:      "high",
			Level:        "4.0",
			Codecs:       "avc1.640028,mp4a.40.2",
			SortOrder:    2,
		},
	}
}

func orderedSupportedRenditionKeys() []string {
	return []string{"720p-fast", "720p-high", "1080p"}
}

// ResolveRenditionSpecs validates user-facing rendition keys and maps them to encoding settings.
func ResolveRenditionSpecs(keys []string) ([]RenditionSpec, error) {
	supported := supportedRenditionSpecs()

	specs := make([]RenditionSpec, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	has720pBaseline := false
	for _, rawKey := range keys {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if key == "" {
			continue
		}
		if key == "720p" {
			key = "720p-fast"
		}
		spec, ok := supported[key]
		if !ok {
			return nil, fmt.Errorf("unsupported rendition %q; supported values are 720p-fast,720p-high,720p,1080p", rawKey)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if spec.TargetHeight == 720 {
			has720pBaseline = true
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, errors.New("--renditions must include at least one rendition")
	}
	if !has720pBaseline {
		return nil, errors.New("--renditions must include a 720p baseline playback variant")
	}
	return specs, nil
}

// SelectRenditionSpecsForSource skips variants above source resolution while preserving 720p as the minimum product baseline.
func SelectRenditionSpecsForSource(source VideoMetadata, specs []RenditionSpec) ([]RenditionSpec, error) {
	if source.Height < 720 {
		return nil, fmt.Errorf("source video height %dp is below the required 720p baseline", source.Height)
	}
	selected := make([]RenditionSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.TargetHeight > source.Height {
			continue
		}
		selected = append(selected, spec)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no requested renditions can be generated from source height %dp", source.Height)
	}
	if selected[0].TargetHeight != 720 {
		selected[0].IsDefault = true
	}
	return selected, nil
}

// ValidateGeneratedRendition fails fast when ffmpeg output does not match the requested quality rung.
func ValidateGeneratedRendition(source VideoMetadata, generated VideoMetadata, spec RenditionSpec) error {
	if generated.Height != spec.TargetHeight {
		return fmt.Errorf("generated %s height mismatch: expected %dp from source %dp, got %dp", spec.Key, spec.TargetHeight, source.Height, generated.Height)
	}
	if generated.Width <= 0 {
		return fmt.Errorf("generated %s width must be positive, got %d", spec.Key, generated.Width)
	}
	return nil
}

// AnalyzeHLSPlaylist reads segment duration information from a generated media playlist.
func AnalyzeHLSPlaylist(path string) (HLSHealth, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return HLSHealth{}, fmt.Errorf("read hls playlist: %w", err)
	}
	return ParseHLSHealth(string(content))
}

// ParseHLSHealth extracts EXTINF durations into a small validation summary.
func ParseHLSHealth(content string) (HLSHealth, error) {
	var totalMs int64
	var minMs int64
	var maxMs int64
	segments := 0
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		value := strings.TrimPrefix(line, "#EXTINF:")
		value = strings.TrimSuffix(value, ",")
		seconds, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return HLSHealth{}, fmt.Errorf("parse EXTINF duration %q: %w", value, err)
		}
		durationMs := int64(math.Round(seconds * 1000))
		if durationMs <= 0 {
			return HLSHealth{}, fmt.Errorf("EXTINF duration must be positive, got %dms", durationMs)
		}
		segments++
		totalMs += durationMs
		if minMs == 0 || durationMs < minMs {
			minMs = durationMs
		}
		if durationMs > maxMs {
			maxMs = durationMs
		}
	}
	if segments == 0 {
		return HLSHealth{}, errors.New("hls playlist has no EXTINF segments")
	}
	return HLSHealth{
		Segments:         segments,
		AverageSegmentMs: totalMs / int64(segments),
		MaxSegmentMs:     maxMs,
		MinSegmentMs:     minMs,
	}, nil
}

// ValidateHLSHealth catches segment structures that are likely to hurt high-speed playback.
func ValidateHLSHealth(health HLSHealth, targetSegmentSeconds int, spec RenditionSpec) error {
	targetMs := int64(targetSegmentSeconds * 1000)
	if health.Segments <= 0 {
		return fmt.Errorf("generated %s playlist has no segments", spec.Key)
	}
	if health.MaxSegmentMs > targetMs*2 {
		return fmt.Errorf("generated %s has unstable long segment: max=%dms target=%dms", spec.Key, health.MaxSegmentMs, targetMs)
	}
	// The final segment may be short; reject only very short average structure.
	if health.AverageSegmentMs < targetMs/2 {
		return fmt.Errorf("generated %s average segment is too short: average=%dms target=%dms", spec.Key, health.AverageSegmentMs, targetMs)
	}
	return nil
}

// BuildFFmpegHLSArgs returns the baseline HLS command used by tests and single-variant callers.
func BuildFFmpegHLSArgs(input string, playlistPath string, segmentPattern string, segmentSeconds int) []string {
	specs, _ := ResolveRenditionSpecs([]string{"720p-fast"})
	return BuildFFmpegVariantHLSArgs(input, playlistPath, segmentPattern, segmentSeconds, specs[0])
}

// BuildFFmpegVariantHLSArgs returns the ffmpeg command for one HLS rendition.
func BuildFFmpegVariantHLSArgs(input string, playlistPath string, segmentPattern string, segmentSeconds int, spec RenditionSpec) []string {
	args := []string{
		"-y",
		"-i", input,
		"-vf", fmt.Sprintf("scale=-2:%d", spec.TargetHeight),
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", spec.CRF,
		"-profile:v", spec.Profile,
		"-level", spec.Level,
		"-pix_fmt", "yuv420p",
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segmentSeconds),
		"-sc_threshold", "0",
	}
	if spec.Tune != "" {
		args = append(args, "-tune", spec.Tune)
	}
	if spec.DisableBFrames {
		args = append(args, "-bf", "0", "-refs", "1")
	}
	if spec.MaxRate != "" {
		args = append(args, "-maxrate", spec.MaxRate, "-bufsize", spec.BufSize)
	}
	args = append(args,
		"-c:a", "aac",
		"-b:a", "128k",
		"-hls_time", strconv.Itoa(segmentSeconds),
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", segmentPattern,
		playlistPath,
	)
	return args
}

// WriteMasterPlaylist writes the adaptive HLS master playlist for all generated variants.
func WriteMasterPlaylist(path string, variants []HLSVariant) error {
	if len(variants) == 0 {
		return errors.New("master playlist requires at least one variant")
	}
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:6\n")
	builder.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	for _, variant := range variants {
		if variant.Width <= 0 || variant.Height <= 0 {
			return fmt.Errorf("variant %q has invalid resolution %dx%d", variant.Key, variant.Width, variant.Height)
		}
		builder.WriteString(fmt.Sprintf(
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s\"\n",
			variant.BandwidthBps,
			variant.Width,
			variant.Height,
			variant.Codecs,
		))
		builder.WriteString(fmt.Sprintf("%s/index.m3u8\n", variant.Key))
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return fmt.Errorf("write master playlist: %w", err)
	}
	return nil
}

// ParseVideoMetadata parses ffprobe's compact width/height output.
func ParseVideoMetadata(output string) (VideoMetadata, error) {
	trimmed := firstNonEmptyLine(output)
	parts := strings.Split(trimmed, "x")
	if len(parts) != 2 {
		return VideoMetadata{}, fmt.Errorf("parse ffprobe video metadata %q", strings.TrimSpace(output))
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return VideoMetadata{}, fmt.Errorf("parse ffprobe width %q: %w", parts[0], err)
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return VideoMetadata{}, fmt.Errorf("parse ffprobe height %q: %w", parts[1], err)
	}
	if width <= 0 || height <= 0 {
		return VideoMetadata{}, fmt.Errorf("ffprobe video dimensions must be positive, got %dx%d", width, height)
	}
	return VideoMetadata{Width: width, Height: height}, nil
}

func firstNonEmptyLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// ProbeVideoMetadata reads the first video stream's width and height with ffprobe.
func ProbeVideoMetadata(ffprobeBin string, input string) (VideoMetadata, error) {
	cmd := exec.Command(
		ffprobeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=s=x:p=0",
		input,
	)
	output, err := cmd.Output()
	if err != nil {
		return VideoMetadata{}, fmt.Errorf("ffprobe video metadata failed: %w", err)
	}
	return ParseVideoMetadata(string(output))
}

// BuildLegacySingleBitrateHLSArgs is kept for old call-site compatibility.
func BuildLegacySingleBitrateHLSArgs(input string, playlistPath string, segmentPattern string, segmentSeconds int) []string {
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
	if options.SourceKey == "" && options.OutputDir == "" {
		return ""
	}
	return filepath.Join(resolveOutputDir(options), "master.m3u8")
}

func publicMediaURL(options IngestOptions) string {
	return joinURLPath(options.Storage.PublicBaseURL, sourceObjectKey(options), "hls", "master.m3u8")
}

func publicVariantURL(options IngestOptions, variantKey string) string {
	return joinURLPath(options.Storage.PublicBaseURL, sourceObjectKey(options), "hls", variantKey, "index.m3u8")
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

func nullableInt(value int) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(value), Valid: true}
}

func nullableInt64Value(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}

func nullableString(value string) sql.NullString {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: trimmed, Valid: true}
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
	fmt.Fprintln(w, "  plan       validate a media ingest request and print the expected pipeline summary")
	fmt.Fprintln(w, "  build-hls  generate local multi-rendition HLS assets without uploading or writing database rows")
	fmt.Fprintln(w, "  upload     upload or store an existing local HLS output using stable object keys")
	fmt.Fprintln(w, "  write-db   upsert episode-backed media metadata from an existing HLS output")
	fmt.Fprintln(w, "  ingest     composite command: legacy dry-run / full ingest, or custom staged pipeline via --stages")
}
