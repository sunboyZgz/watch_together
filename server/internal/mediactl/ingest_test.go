package mediactl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseIngestOptionsBuildsDryRunContract(t *testing.T) {
	input := writeTempFile(t, "source.mp4")
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
	_, err := ParseIngestOptions([]string{
		"--input", filepath.Join(t.TempDir(), "missing.mp4"),
		"--title", "missing",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected missing input to fail")
	}
	if !strings.Contains(err.Error(), "--input file is not accessible") {
		t.Fatalf("expected input error, got %v", err)
	}
}

func TestRunIngestPrintsDryRunSummary(t *testing.T) {
	input := writeTempFile(t, "source.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"ingest",
		"--input", input,
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
	if !strings.Contains(stdout.String(), `"ffmpegBin": "ffmpeg"`) {
		t.Fatalf("expected default ffmpeg in summary, got %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ffprobeBin": "ffprobe"`) {
		t.Fatalf("expected default ffprobe in summary, got %s", stdout.String())
	}
}

func TestParseIngestOptionsRequiresStableOutputForNonDryRun(t *testing.T) {
	input := writeTempFile(t, "source.mp4")

	_, err := ParseIngestOptions([]string{
		"--input", input,
		"--title", "测试视频",
		"--dry-run=false",
	}, envLookup(nil), &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected missing media id to fail")
	}
	if !strings.Contains(err.Error(), "--media-id is required") {
		t.Fatalf("expected media id error, got %v", err)
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

func envLookup(values map[string]string) EnvLookup {
	return func(name string) string {
		return values[name]
	}
}
