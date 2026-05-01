package mediactl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIngestOptionsBuildsDryRunContract(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "violet-evergarden/season-01/episode-09.mp4")
	cover := writeTempFile(t, "cover.jpg")
	env := map[string]string{
		"MEDIA_STORAGE_DRIVER":           "local",
		"MEDIA_LOCAL_ROOT":               "/tmp/watch-media",
		"MEDIA_PUBLIC_BASE_URL":          "http://127.0.0.1:9000/media/tmp",
		"MEDIA_OBJECT_KEY_PREFIX":        "media",
		"MEDIA_STORAGE_FORCE_PATH_STYLE": "true",
		"FFMPEG_BIN":                     "/opt/homebrew/bin/ffmpeg",
		"FFPROBE_BIN":                    "/opt/homebrew/bin/ffprobe",
	}

	options, err := ParseIngestOptions([]string{
		"--media-id", "media_uuid",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "紫罗兰永恒花园",
		"--season-label", "第 1 季",
		"--episode-label", "第 09 集",
		"--tags", "healing, anime,healing",
		"--cover", cover,
		"--output-dir", "/tmp/watch-media/media_uuid/hls",
		"--hls-segment-seconds", "4",
		"--upload",
	}, envLookup(env), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse ingest options: %v", err)
	}

	if options.Input != input {
		t.Fatalf("expected input %q, got %q", input, options.Input)
	}
	if options.SourceKey != "violet-evergarden/season-01/episode-09.mp4" {
		t.Fatalf("expected source key, got %q", options.SourceKey)
	}
	if options.SourceHash != "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" {
		t.Fatalf("expected sha256 hash, got %q", options.SourceHash)
	}
	if options.SeasonSlug != "violet-evergarden" {
		t.Fatalf("expected season slug, got %q", options.SeasonSlug)
	}
	if options.SeasonNumber == nil || *options.SeasonNumber != 1 {
		t.Fatalf("expected season number 1, got %#v", options.SeasonNumber)
	}
	if options.EpisodeNumber == nil || *options.EpisodeNumber != 9 {
		t.Fatalf("expected episode number 9, got %#v", options.EpisodeNumber)
	}
	if options.Title != "紫罗兰永恒花园" {
		t.Fatalf("unexpected title %q", options.Title)
	}
	if got := strings.Join(options.Tags, ","); got != "healing,anime" {
		t.Fatalf("expected deduped tags, got %q", got)
	}
	if !options.Upload {
		t.Fatalf("expected upload request to be captured")
	}
	if options.MediaID != "media_uuid" {
		t.Fatalf("expected media id media_uuid, got %q", options.MediaID)
	}
	if options.OutputDir != "/tmp/watch-media/media_uuid/hls" {
		t.Fatalf("expected output dir, got %q", options.OutputDir)
	}
	if options.HLSSegment != 4 {
		t.Fatalf("expected hls segment 4, got %d", options.HLSSegment)
	}
	if !options.DryRun {
		t.Fatalf("expected dry run by default")
	}
	if options.Storage.LocalRoot != "/tmp/watch-media" {
		t.Fatalf("expected configured local root, got %q", options.Storage.LocalRoot)
	}
	if options.Storage.FFmpegBin != "/opt/homebrew/bin/ffmpeg" {
		t.Fatalf("expected configured ffmpeg path, got %q", options.Storage.FFmpegBin)
	}
	if options.Storage.FFprobeBin != "/opt/homebrew/bin/ffprobe" {
		t.Fatalf("expected configured ffprobe path, got %q", options.Storage.FFprobeBin)
	}
}

func TestParseIngestOptionsRequiresExistingInput(t *testing.T) {
	libraryRoot := t.TempDir()
	_, err := ParseIngestOptions([]string{
		"--input", filepath.Join(libraryRoot, "missing.mp4"),
		"--library-root", libraryRoot,
		"--title", "missing",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected missing input to fail")
	}
	if !strings.Contains(err.Error(), "--input file is not accessible") {
		t.Fatalf("expected input error, got %v", err)
	}
}

func TestParseIngestOptionsRequiresInputInsideLibraryRoot(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeTempFile(t, "episode-01.mp4")

	_, err := ParseIngestOptions([]string{
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected input outside library root to fail")
	}
	if !strings.Contains(err.Error(), "--input must be inside --library-root") {
		t.Fatalf("expected library containment error, got %v", err)
	}
}

func TestParseIngestOptionsRequiresSafeSourcePath(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "Violet Evergarden/season-01/episode-01.mp4")

	_, err := ParseIngestOptions([]string{
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected unsafe source path to fail")
	}
	if !strings.Contains(err.Error(), "must use lowercase letters") {
		t.Fatalf("expected source path component error, got %v", err)
	}
}

func TestRunIngestPrintsDryRunSummary(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "bocchi/season-01/episode-01.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"ingest",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "孤独摇滚!",
		"--tags", "music,comedy",
	}, envLookup(nil), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mediactl ingest dry-run summary") {
		t.Fatalf("expected dry-run summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"title": "孤独摇滚!"`) {
		t.Fatalf("expected title in summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"sourceKey": "bocchi/season-01/episode-01.mp4"`) {
		t.Fatalf("expected source key in summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ffmpegBin": "ffmpeg"`) {
		t.Fatalf("expected default ffmpeg in summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ffprobeBin": "ffprobe"`) {
		t.Fatalf("expected default ffprobe in summary, got %s", stdout.String())
	}
}

func TestParseIngestOptionsRequiresLibraryRoot(t *testing.T) {
	input := writeTempFile(t, "source.mp4")

	_, err := ParseIngestOptions([]string{
		"--input", input,
		"--title", "测试视频",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected missing library root to fail")
	}
	if !strings.Contains(err.Error(), "--library-root is required") {
		t.Fatalf("expected library root error, got %v", err)
	}
}

func TestParseIngestOptionsRequiresDatabaseURLForWriteDB(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "test-show/season-01/episode-01.mp4")

	_, err := ParseIngestOptions([]string{
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
		"--dry-run=false",
		"--write-db",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected missing database url to fail")
	}
	if !strings.Contains(err.Error(), "--write-db requires DATABASE_URL") {
		t.Fatalf("expected database url error, got %v", err)
	}
}

func TestBuildFFmpegHLSArgs(t *testing.T) {
	args := BuildFFmpegHLSArgs(
		"input.mp4",
		"/tmp/out/index.m3u8",
		"/tmp/out/segment_%05d.ts",
		6,
	)

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-i input.mp4",
		"-c:v h264",
		"-c:a aac",
		"-hls_time 6",
		"-hls_playlist_type vod",
		"-hls_segment_filename /tmp/out/segment_%05d.ts",
		"/tmp/out/index.m3u8",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected ffmpeg args to contain %q, got %q", want, joined)
		}
	}
}

func TestPlannedMediaURLUsesStableObjectKey(t *testing.T) {
	options := IngestOptions{
		SourceKey: "violet-evergarden/season-01/episode-09.mp4",
		Storage: StorageConfig{
			PublicBaseURL:   "http://127.0.0.1:9000/media/tmp/",
			ObjectKeyPrefix: "media",
		},
	}

	got := plannedMediaURL(options)
	want := "http://127.0.0.1:9000/media/tmp/media/violet-evergarden/season-01/episode-09/hls/index.m3u8"
	if got != want {
		t.Fatalf("expected media url %q, got %q", want, got)
	}
}

func TestRunUnknownCommandReturnsUsageError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"nope"}, envLookup(nil), &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("expected usage exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), `unknown command "nope"`) {
		t.Fatalf("expected unknown command error, got %s", stderr.String())
	}
}

func writeTempFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func writeLibraryFile(t *testing.T, root string, name string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create library dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
		t.Fatalf("write library file: %v", err)
	}
	return path
}

func envLookup(values map[string]string) EnvLookup {
	return func(name string) string {
		return values[name]
	}
}
