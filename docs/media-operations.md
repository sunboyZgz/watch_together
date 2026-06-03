# Media Operations

Media ingestion and playback delivery are implemented in `server/cmd/mediactl`, `server/internal/mediactl`, and `server/internal/transport/media_delivery.go`.

## Mediactl Commands

Run from `server/`:

```bash
go run ./cmd/mediactl <command>
```

Supported commands:

- `plan`: validate input and print the expected HLS/storage layout.
- `build-hls`: generate local multi-rendition HLS assets.
- `upload`: store or upload an existing local HLS output.
- `write-db`: upsert episode-backed metadata from an existing HLS output.
- `ingest`: legacy dry-run/full ingest command, or staged pipeline via `--stages`.

Example staged ingest:

```bash
cd server
go run ./cmd/mediactl ingest \
  --stages=build-hls,upload,write-db \
  --library-root ../media/raw \
  --input ../media/raw/sample-show/season-01/episode-01.mp4 \
  --title "Sample Episode" \
  --season-label "Season 1" \
  --episode-label "Episode 1" \
  --tags test,anime \
  --dry-run=false
```

## Source Layout

`mediactl` derives `sourceKey`, season slug, season number, and episode number from the input path relative to `--library-root`.

Expected pattern:

```text
<season-slug>/season-XX/episode-XX.ext
```

Path components must use lowercase letters, numbers, dot, dash, or underscore.

## Important Flags

```text
--input                  source video file
--library-root           root used to derive source_key
--title                  required media title
--subtitle               optional subtitle
--description            optional description
--category               optional category
--original-title         optional original title
--production-team        optional production/studio text
--search-aliases         comma-separated search aliases
--season-label           optional display label
--episode-label          optional display label
--tags                   comma-separated tag slugs or names
--cover                  optional cover image path
--output-dir             optional local HLS output directory
--hls-segment-seconds    segment target, default 6
--renditions             default 720p-fast,720p-high,1080p
--write-db               upsert PostgreSQL metadata
--database-url           overrides DATABASE_URL
--dry-run                defaults true for legacy ingest
```

`--stages` supports `plan`, `build-hls`, `upload`, and `write-db`. `plan` cannot be mixed with mutating stages.

## HLS Generation

Generated HLS uses FFmpeg and FFprobe. Supported rendition keys:

- `720p-fast`, default playback variant.
- `720p-high`.
- `720p`, alias for `720p-fast`.
- `1080p`.

The source video must be at least 720p high. Renditions above the source resolution are skipped. Generated playlists are validated for expected resolution and segment health.

## Storage Drivers

`MEDIA_STORAGE_DRIVER` supports:

- `local`: copies generated HLS and optional cover into `MEDIA_LOCAL_ROOT`.
- `minio`: uploads to an S3-compatible endpoint.
- `s3`: uploads to an S3-compatible endpoint.

Stable object key shape:

```text
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExtension}/hls/master.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExtension}/hls/{variant}/index.m3u8
{MEDIA_OBJECT_KEY_PREFIX}/{sourceKeyWithoutExtension}/cover/cover.jpg
```

The public media URL stored in PostgreSQL is built from `MEDIA_PUBLIC_BASE_URL` and the object key.

## Database Writes

`write-db` and `ingest --write-db` upsert:

- `media_seasons`
- `media_episodes`
- `media_episode_variants`
- `media_tags`
- `media_season_tags`

Re-running with the same source path updates the same season/source rows instead of creating duplicate episode metadata.

## Playback Delivery Modes

The HTTP API returns server-signed `/media/playback/{episodeId}/master.m3u8?...` URLs. The server then serves that URL according to `MEDIA_DELIVERY_MODE`.

### `signed_redirect`

Only `master.m3u8` is supported. The server validates the signed playback URL and redirects to the raw `media_url` from PostgreSQL.

### `minio_presign`

The server derives the object key from `media_url`, reads HLS playlists from object storage, rewrites playlist entries to signed playback URLs or presigned object URLs, and redirects segment requests to presigned object URLs.

### `nginx_auth_request`

The server validates the signed playback URL, issues a short-lived `wt_media_access` cookie, and redirects to the Nginx-served object path. Nginx calls `/media/internal/auth` with the original URI so the Go server can validate the cookie signature.

Production Compose is designed around this mode so MinIO stays private and public HLS access goes through Nginx.
