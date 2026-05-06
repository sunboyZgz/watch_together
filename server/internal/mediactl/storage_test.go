package mediactl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadIngestAssetsLocalCopiesHLSAndCoverToStablePaths(t *testing.T) {
	stagingRoot := t.TempDir()
	localRoot := t.TempDir()

	masterPath := filepath.Join(stagingRoot, "master.m3u8")
	variantPath := filepath.Join(stagingRoot, "720p-fast", "index.m3u8")
	segmentPath := filepath.Join(stagingRoot, "720p-fast", "segment_00000.ts")
	coverPath := filepath.Join(t.TempDir(), "cover.png")

	mustWriteMediactlFile(t, masterPath, "#EXTM3U\n")
	mustWriteMediactlFile(t, variantPath, "#EXTM3U\n")
	mustWriteMediactlFile(t, segmentPath, "segment")
	mustWriteMediactlFile(t, coverPath, "cover")

	options := IngestOptions{
		SourceKey: "sample-show/season-01/episode-01.mp4",
		Cover:     coverPath,
		Storage: StorageConfig{
			Driver:          "local",
			LocalRoot:       localRoot,
			PublicBaseURL:   "http://127.0.0.1:9000/media/tmp",
			ObjectKeyPrefix: "media",
		},
	}
	result := HLSResult{
		OutputDir:    stagingRoot,
		PlaylistPath: masterPath,
		Variants: []HLSVariant{
			{Key: "720p-fast", PlaylistPath: variantPath},
		},
	}

	stored, err := UploadIngestAssets(context.Background(), options, result)
	if err != nil {
		t.Fatalf("upload ingest assets: %v", err)
	}

	wantPlaylistPath := filepath.Join(localRoot, "media", "sample-show", "season-01", "episode-01", "hls", "master.m3u8")
	if stored.PlaylistPath != wantPlaylistPath {
		t.Fatalf("expected stored playlist path %q, got %q", wantPlaylistPath, stored.PlaylistPath)
	}
	if _, err := os.Stat(wantPlaylistPath); err != nil {
		t.Fatalf("expected final master playlist to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localRoot, "media", "sample-show", "season-01", "episode-01", "hls", "720p-fast", "segment_00000.ts")); err != nil {
		t.Fatalf("expected final segment to exist: %v", err)
	}
	if stored.MediaURL != "http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/master.m3u8" {
		t.Fatalf("unexpected media url %q", stored.MediaURL)
	}
	if stored.CoverURL != "http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/cover/cover.png" {
		t.Fatalf("unexpected cover url %q", stored.CoverURL)
	}
	if stored.Variants[0].PlaylistURL != "http://127.0.0.1:9000/media/tmp/media/sample-show/season-01/episode-01/hls/720p-fast/index.m3u8" {
		t.Fatalf("unexpected variant playlist url %q", stored.Variants[0].PlaylistURL)
	}
}

func TestLoadStorageConfigIncludesRemoteCredentials(t *testing.T) {
	config := LoadStorageConfig(envLookup(map[string]string{
		"MEDIA_STORAGE_DRIVER":            "minio",
		"MEDIA_STORAGE_ENDPOINT":          "http://127.0.0.1:9001",
		"MEDIA_STORAGE_BUCKET":            "watch-together-media",
		"MEDIA_STORAGE_REGION":            "auto",
		"MEDIA_STORAGE_ACCESS_KEY_ID":     "minioadmin",
		"MEDIA_STORAGE_SECRET_ACCESS_KEY": "minioadmin",
	}))

	if config.AccessKeyID != "minioadmin" {
		t.Fatalf("expected access key id, got %q", config.AccessKeyID)
	}
	if config.SecretAccessKey != "minioadmin" {
		t.Fatalf("expected secret access key to load")
	}
}

func TestNewS3StorageUploaderRejectsMissingRemoteConfig(t *testing.T) {
	_, err := newStorageUploader(StorageConfig{Driver: "minio"})
	if err == nil {
		t.Fatalf("expected missing remote config to fail")
	}
	if !strings.Contains(err.Error(), "MEDIA_STORAGE_ENDPOINT") {
		t.Fatalf("expected endpoint validation error, got %v", err)
	}
}

func mustWriteMediactlFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
