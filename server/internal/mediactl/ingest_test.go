package mediactl

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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
	if options.MediaID != "media_uuid" {
		t.Fatalf("expected media id media_uuid, got %q", options.MediaID)
	}
	if options.OutputDir != "/tmp/watch-media/media_uuid/hls" {
		t.Fatalf("expected output dir, got %q", options.OutputDir)
	}
	if options.HLSSegment != 4 {
		t.Fatalf("expected hls segment 4, got %d", options.HLSSegment)
	}
	if got := strings.Join(options.Renditions, ","); got != "720p-fast,720p-high,1080p" {
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

func TestRunPlanPrintsDryRunSummary(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "frieren/season-01/episode-01.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"plan",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "葬送的芙莉莲",
	}, envLookup(nil), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mediactl plan summary") {
		t.Fatalf("expected plan summary, got %s", stdout.String())
	}
}

func TestRunUploadSupportsExistingHLSWithoutTitle(t *testing.T) {
	libraryRoot := t.TempDir()
	localRoot := t.TempDir()
	stagingDir := filepath.Join(t.TempDir(), "staging")
	input := writeLibraryFile(t, libraryRoot, "sample-show/season-01/episode-01.mp4")
	ffprobeStub := writeFFprobeStub(t)
	writeExistingHLSOutput(t, stagingDir, []string{"720p-fast", "1080p"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{
		"upload",
		"--input", input,
		"--library-root", libraryRoot,
		"--output-dir", stagingDir,
		"--dry-run=false",
	}, envLookup(map[string]string{
		"MEDIA_STORAGE_DRIVER":    "local",
		"MEDIA_LOCAL_ROOT":        localRoot,
		"MEDIA_PUBLIC_BASE_URL":   "http://127.0.0.1:9000/media/tmp",
		"MEDIA_OBJECT_KEY_PREFIX": "media",
		"FFPROBE_BIN":             ffprobeStub,
	}), &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d, stderr=%s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "mediactl upload completed") {
		t.Fatalf("expected upload summary, got %s", stdout.String())
	}
	finalMaster := filepath.Join(localRoot, "media", "sample-show", "season-01", "episode-01", "hls", "master.m3u8")
	if _, err := os.Stat(finalMaster); err != nil {
		t.Fatalf("expected uploaded master playlist at %q: %v", finalMaster, err)
	}
	if !strings.Contains(stdout.String(), `"mediaUrl": "http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/master.m3u8"`) {
		t.Fatalf("expected media url in upload summary, got %s", stdout.String())
	}
}

func TestRunWriteDBRequiresDatabaseURL(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "sample-show/season-01/episode-01.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"write-db",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
		"--dry-run=false",
	}, envLookup(nil), &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--write-db requires DATABASE_URL") {
		t.Fatalf("expected database url validation error, got %s", stderr.String())
	}
}

func TestRunIngestSupportsCustomStageSequence(t *testing.T) {
	libraryRoot := t.TempDir()
	localRoot := t.TempDir()
	stagingDir := filepath.Join(t.TempDir(), "staging")
	input := writeLibraryFile(t, libraryRoot, "sample-show/season-01/episode-01.mp4")
	ffprobeStub := writeFFprobeStub(t)
	writeExistingHLSOutput(t, stagingDir, []string{"720p-fast"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := Run([]string{
		"ingest",
		"--stages=upload,write-db",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
		"--output-dir", stagingDir,
		"--database-url", "postgres://app:app@127.0.0.1:1/anime_watch_dev?sslmode=disable",
		"--dry-run=false",
	}, envLookup(map[string]string{
		"MEDIA_STORAGE_DRIVER":    "local",
		"MEDIA_LOCAL_ROOT":        localRoot,
		"MEDIA_PUBLIC_BASE_URL":   "http://127.0.0.1:9000/media/tmp",
		"MEDIA_OBJECT_KEY_PREFIX": "media",
		"FFPROBE_BIN":             ffprobeStub,
	}), &stdout, &stderr)

	if exitCode == 0 {
		t.Fatalf("expected write-db to fail without a real database")
	}
	if !strings.Contains(stderr.String(), "connect database") {
		t.Fatalf("expected database connection failure after staged upload, got %s", stderr.String())
	}
	finalMaster := filepath.Join(localRoot, "media", "sample-show", "season-01", "episode-01", "hls", "master.m3u8")
	if _, err := os.Stat(finalMaster); err != nil {
		t.Fatalf("expected staged upload to happen before write-db failure: %v", err)
	}
}

func TestRunIngestRejectsPlanMixedWithMutations(t *testing.T) {
	libraryRoot := t.TempDir()
	input := writeLibraryFile(t, libraryRoot, "sample-show/season-01/episode-01.mp4")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{
		"ingest",
		"--stages=plan,upload",
		"--input", input,
		"--library-root", libraryRoot,
		"--title", "测试视频",
	}, envLookup(nil), &stdout, &stderr)

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "cannot mix plan with mutating stages") {
		t.Fatalf("expected stage validation error, got %s", stderr.String())
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
		"-crf 25",
		"-profile:v main",
		"-level 3.1",
		"-pix_fmt yuv420p",
		"-force_key_frames expr:gte(t,n_forced*6)",
		"-sc_threshold 0",
		"-tune fastdecode",
		"-bf 0",
		"-refs 1",
		"-maxrate 1800k",
		"-bufsize 3600k",
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
	if !strings.Contains(err.Error(), "must include a 720p baseline") {
		t.Fatalf("expected baseline error, got %v", err)
	}
}

func TestSelectRenditionSpecsSkipsAboveSourceHeight(t *testing.T) {
	specs, err := ResolveRenditionSpecs([]string{"720p-fast", "720p-high", "1080p"})
	if err != nil {
		t.Fatalf("resolve rendition specs: %v", err)
	}
	selected, err := SelectRenditionSpecsForSource(VideoMetadata{Width: 1280, Height: 720}, specs)
	if err != nil {
		t.Fatalf("select rendition specs: %v", err)
	}
	if len(selected) != 2 || selected[0].Key != "720p-fast" || selected[1].Key != "720p-high" {
		t.Fatalf("expected 720p variants, got %#v", selected)
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
			Key:          "720p-fast",
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
		"720p-fast/index.m3u8",
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,CODECS=\"avc1.640028,mp4a.40.2\"",
		"1080p/index.m3u8",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected master playlist to contain %q, got %s", want, got)
		}
	}
}

func TestResolveRenditionSpecsMapsLegacy720pToFastVariant(t *testing.T) {
	specs, err := ResolveRenditionSpecs([]string{"720p", "1080p"})
	if err != nil {
		t.Fatalf("resolve rendition specs: %v", err)
	}
	if specs[0].Key != "720p-fast" {
		t.Fatalf("expected legacy 720p alias to map to 720p-fast, got %#v", specs[0])
	}
}

func TestParseHLSHealth(t *testing.T) {
	health, err := ParseHLSHealth(`#EXTM3U
#EXT-X-VERSION:6
#EXTINF:6.000000,
segment_00000.ts
#EXTINF:5.500000,
segment_00001.ts
#EXT-X-ENDLIST
`)
	if err != nil {
		t.Fatalf("parse hls health: %v", err)
	}
	if health.Segments != 2 {
		t.Fatalf("expected two segments, got %#v", health)
	}
	if health.AverageSegmentMs != 5750 {
		t.Fatalf("expected average 5750ms, got %#v", health)
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

func writeExistingHLSOutput(t *testing.T, outputDir string, variants []string) {
	t.Helper()
	master := "#EXTM3U\n#EXT-X-VERSION:6\n"
	for _, variant := range variants {
		master += "#EXT-X-STREAM-INF:BANDWIDTH=2000000,RESOLUTION=1280x720,CODECS=\"avc1.4d401f,mp4a.40.2\"\n"
		master += variant + "/index.m3u8\n"

		playlistPath := filepath.Join(outputDir, variant, "index.m3u8")
		mustWriteMediactlPlaylist(t, playlistPath, "#EXTM3U\n#EXT-X-VERSION:6\n#EXTINF:6.000000,\nsegment_00000.ts\n#EXTINF:6.000000,\nsegment_00001.ts\n#EXT-X-ENDLIST\n")
		mustWriteMediactlPlaylist(t, filepath.Join(outputDir, variant, "segment_00000.ts"), "segment-a")
		mustWriteMediactlPlaylist(t, filepath.Join(outputDir, variant, "segment_00001.ts"), "segment-b")
	}
	mustWriteMediactlPlaylist(t, filepath.Join(outputDir, "master.m3u8"), master)
}

func writeFFprobeStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(t.TempDir(), "ffprobe-stub.cmd")
		script := `@echo off
set "args=%*"
echo %args% | findstr /C:"stream=width,height" >nul
if not errorlevel 1 (
  echo %args% | findstr /C:"1080p" >nul
  if not errorlevel 1 (
    echo 1920x1080
  ) else (
    echo 1280x720
  )
) else (
  echo 12.0
)
`
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write ffprobe stub: %v", err)
		}
		return path
	}
	path := filepath.Join(t.TempDir(), "ffprobe-stub.sh")
	script := `#!/bin/sh
args="$*"
case "$args" in
  *stream=width,height*)
    case "$args" in
      *1080p*)
        printf "1920x1080\n"
        ;;
      *)
        printf "1280x720\n"
        ;;
    esac
    ;;
  *)
    printf "12.0\n"
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write ffprobe stub: %v", err)
	}
	return path
}

func mustWriteMediactlPlaylist(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
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
