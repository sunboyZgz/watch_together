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
	if got := strings.Join(options.Renditions, ","); got != "720p,1080p" {
		t.Fatalf("expected default renditions, got %q", got)
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
		"-vf scale=-2:720",
		"-c:v libx264",
		"-preset veryfast",
		"-crf 24",
		"-profile:v main",
		"-level 3.1",
		"-pix_fmt yuv420p",
		"-force_key_frames expr:gte(t,n_forced*6)",
		"-sc_threshold 0",
		"-tune fastdecode",
		"-bf 0",
		"-refs 1",
		"-maxrate 2200k",
		"-bufsize 4400k",
		"-c:a aac",
		"-b:a 128k",
		"-hls_time 6",
		"-hls_playlist_type vod",
		"-hls_flags independent_segments",
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
	want := "http://127.0.0.1:9000/media/tmp/media/violet-evergarden/season-01/episode-09/hls/master.m3u8"
	if got != want {
		t.Fatalf("expected media url %q, got %q", want, got)
	}
}

func TestResolveRenditionSpecsRequires720pBaseline(t *testing.T) {
	_, err := ResolveRenditionSpecs([]string{"1080p"})
	if err == nil {
		t.Fatalf("expected missing 720p to fail")
	}
	if !strings.Contains(err.Error(), "must include 720p") {
		t.Fatalf("expected baseline error, got %v", err)
	}
}

func TestSelectRenditionSpecsSkipsAboveSourceHeight(t *testing.T) {
	specs, err := ResolveRenditionSpecs([]string{"720p", "1080p"})
	if err != nil {
		t.Fatalf("resolve rendition specs: %v", err)
	}
	selected, err := SelectRenditionSpecsForSource(VideoMetadata{Width: 1280, Height: 720}, specs)
	if err != nil {
		t.Fatalf("select rendition specs: %v", err)
	}
	if len(selected) != 1 || selected[0].Key != "720p" {
		t.Fatalf("expected only 720p, got %#v", selected)
	}
}

func TestSelectRenditionSpecsRejectsBelow720p(t *testing.T) {
	specs, err := ResolveRenditionSpecs([]string{"720p"})
	if err != nil {
		t.Fatalf("resolve rendition specs: %v", err)
	}
	_, err = SelectRenditionSpecsForSource(VideoMetadata{Width: 854, Height: 480}, specs)
	if err == nil {
		t.Fatalf("expected below-baseline source to fail")
	}
	if !strings.Contains(err.Error(), "below the required 720p baseline") {
		t.Fatalf("expected baseline height error, got %v", err)
	}
}

func TestWriteMasterPlaylist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.m3u8")
	err := WriteMasterPlaylist(path, []HLSVariant{
		{
			Key:          "720p",
			Width:        1280,
			Height:       720,
			BandwidthBps: 2_000_000,
			Codecs:       "avc1.4d401f,mp4a.40.2",
		},
		{
			Key:          "1080p",
			Width:        1920,
			Height:       1080,
			BandwidthBps: 5_000_000,
			Codecs:       "avc1.640028,mp4a.40.2",
		},
	})
	if err != nil {
		t.Fatalf("write master playlist: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read master playlist: %v", err)
	}
	got := string(content)
	for _, want := range []string{
		"#EXTM3U",
		"#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720,CODECS=\"avc1.4d401f,mp4a.40.2\"",
		"720p/index.m3u8",
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,CODECS=\"avc1.640028,mp4a.40.2\"",
		"1080p/index.m3u8",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected master playlist to contain %q, got %s", want, got)
		}
	}
}

func TestParseVideoMetadata(t *testing.T) {
	metadata, err := ParseVideoMetadata("1920x1080\n")
	if err != nil {
		t.Fatalf("parse video metadata: %v", err)
	}
	if metadata.Width != 1920 || metadata.Height != 1080 {
		t.Fatalf("unexpected metadata %#v", metadata)
	}
}

func TestParseVideoMetadataUsesFirstNonEmptyLine(t *testing.T) {
	metadata, err := ParseVideoMetadata("1280x720\n\n1280x720\n")
	if err != nil {
		t.Fatalf("parse video metadata: %v", err)
	}
	if metadata.Width != 1280 || metadata.Height != 720 {
		t.Fatalf("unexpected metadata %#v", metadata)
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
