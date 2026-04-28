package mediactl

import (
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
	MediaID      string        `json:"mediaId,omitempty"`
	Input        string        `json:"input"`
	Title        string        `json:"title"`
	SeasonLabel  string        `json:"seasonLabel,omitempty"`
	EpisodeLabel string        `json:"episodeLabel,omitempty"`
	Tags         []string      `json:"tags,omitempty"`
	Cover        string        `json:"cover,omitempty"`
	OutputDir    string        `json:"outputDir,omitempty"`
	HLSSegment   int           `json:"hlsSegmentSeconds"`
	Upload       bool          `json:"upload"`
	DryRun       bool          `json:"dryRun"`
	Storage      StorageConfig `json:"storage"`
}

// IngestSummary reports either planned or completed local ingest work.
type IngestSummary struct {
	IngestOptions
	HLSPlaylistPath string `json:"hlsPlaylistPath,omitempty"`
	DurationMs      int64  `json:"durationMs,omitempty"`
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
		return printIngestSummary(stdout, "mediactl ingest dry-run summary:", summary, "next stages are not implemented yet: upload, database upsert")
	}

	result, err := GenerateHLS(options)
	if err != nil {
		return err
	}
	summary.HLSPlaylistPath = result.PlaylistPath
	summary.DurationMs = result.DurationMs
	return printIngestSummary(stdout, "mediactl ingest completed:", summary, "next stages are not implemented yet: upload, database upsert")
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
	options := IngestOptions{DryRun: true, HLSSegment: defaultHLSSegmentTime}
	flags.StringVar(&options.MediaID, "media-id", "", "stable media id used for local object key layout")
	flags.StringVar(&options.Input, "input", "", "source video file path")
	flags.StringVar(&options.Title, "title", "", "media title")
	flags.StringVar(&options.SeasonLabel, "season-label", "", "season display label")
	flags.StringVar(&options.EpisodeLabel, "episode-label", "", "episode display label")
	flags.StringVar(&tags, "tags", "", "comma-separated tag slugs or names")
	flags.StringVar(&options.Cover, "cover", "", "optional cover image path")
	flags.StringVar(&options.OutputDir, "output-dir", "", "optional HLS output directory")
	flags.IntVar(&options.HLSSegment, "hls-segment-seconds", defaultHLSSegmentTime, "HLS segment duration in seconds")
	flags.BoolVar(&options.Upload, "upload", false, "request upload in later ingest stages")
	flags.BoolVar(&options.DryRun, "dry-run", true, "print planned ingest work without mutating files or database")

	if err := flags.Parse(args); err != nil {
		return IngestOptions{}, err
	}
	if flags.NArg() > 0 {
		return IngestOptions{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	options.Input = strings.TrimSpace(options.Input)
	options.MediaID = strings.TrimSpace(options.MediaID)
	options.Title = strings.TrimSpace(options.Title)
	options.SeasonLabel = strings.TrimSpace(options.SeasonLabel)
	options.EpisodeLabel = strings.TrimSpace(options.EpisodeLabel)
	options.Cover = strings.TrimSpace(options.Cover)
	options.OutputDir = strings.TrimSpace(options.OutputDir)
	options.Tags = splitTags(tags)
	options.Storage = LoadStorageConfig(getenv)

	if options.Input == "" {
		return IngestOptions{}, errors.New("--input is required")
	}
	if options.Title == "" {
		return IngestOptions{}, errors.New("--title is required")
	}
	if options.HLSSegment < 4 || options.HLSSegment > 6 {
		return IngestOptions{}, errors.New("--hls-segment-seconds must be between 4 and 6")
	}
	if !options.DryRun && options.MediaID == "" && options.OutputDir == "" {
		return IngestOptions{}, errors.New("--media-id is required when --dry-run=false unless --output-dir is provided")
	}
	if err := requireExistingFile(options.Input, "--input"); err != nil {
		return IngestOptions{}, err
	}
	if options.Cover != "" {
		if err := requireExistingFile(options.Cover, "--cover"); err != nil {
			return IngestOptions{}, err
		}
	}

	return options, nil
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
		"-c:v", "h264",
		"-c:a", "aac",
		"-hls_time", strconv.Itoa(segmentSeconds),
		"-hls_playlist_type", "vod",
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
	return filepath.Join(options.Storage.LocalRoot, options.Storage.ObjectKeyPrefix, options.MediaID, "hls")
}

func plannedPlaylistPath(options IngestOptions) string {
	if options.OutputDir == "" && options.MediaID == "" {
		return ""
	}
	return filepath.Join(resolveOutputDir(options), "index.m3u8")
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
