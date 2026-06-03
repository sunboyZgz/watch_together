# Android Client

The Android app lives in `android/` and is implemented with Kotlin, Jetpack Compose, OkHttp, and AndroidX Media3 ExoPlayer.

## App Entry

`MainActivity` launches `WatchTogetherApp`, which coordinates:

1. Login screen and login dialog.
2. Home page.
3. Video selection page.
4. Room theater/player page.

The app stores the successful auth session locally through `AuthSessionStore` and uses the JWT access token for protected HTTP and WebSocket calls.

## Main Packages

```text
android/app/src/main/java/com/example/watch_together/
|-- auth/        login client, auth models, persisted auth session
|-- config/      BuildConfig-derived runtime URLs and environment flags
|-- pages/       business screens: login, home, video selection, room
|-- sync/        room HTTP client, WebSocket client, session controller, sync coordinator
|-- sync/protocol/ Android-side protocol models
|-- ui/player/   Media3 player adapter, player shell, cache, and playback models
|-- ui/theme/    Compose theme
```

## Implemented Flow

- `LoginPage` and `LoginDialog` call `POST /auth/login`.
- `HomePage` calls `GET /home/summary` with `Authorization: Bearer <token>`.
- `VideoSelectionPage` calls `GET /media/tags` and `GET /media/items`.
- Creating a room passes the selected episode ID as `mediaItemId` to `POST /rooms`.
- Joining a room first calls `POST /rooms/{roomCode}/join`, then connects to `/ws` and sends `join_room`.
- `PlayerScreen` assembles the room session controller, sync coordinator, player adapter, and room theater UI.
- `RoomTheaterPage` owns the business page shell.
- `PlayerCoreShell` owns the player viewport and controls.

## Player Behavior

The player uses Media3 ExoPlayer with HLS support.

Implemented behavior:

- Loads backend-provided `mediaUrl` rather than constructing storage URLs in Android.
- Uses a custom load-control configuration for larger HLS buffer windows.
- Uses Media3 `SimpleCache` under `cacheDir/watch_together_media_cache` with a 512 MB LRU limit.
- Supports HLS ahead prefetch into the same cache for better high-speed and rejoin behavior.
- Uses a custom overlay instead of Media3's native controls.
- Supports play/pause, seek, playback-rate selection, fullscreen, and quality selection.
- Uses drift correction with speed nudge first and seek fallback for larger drift.
- Writes low-frequency progress through `PUT /me/media-progress/{mediaItemId}`.

Playback progress writes are not part of the real-time WebSocket authority model.

## URL Rewriting

For local emulator development, Android can rewrite loopback media URLs. In the `local` flavor, `REWRITE_LOOPBACK_MEDIA_URLS` defaults to `true`, so backend media URLs using `127.0.0.1`, `localhost`, or `0.0.0.0` can be rewritten to the configured `MEDIA_BASE_URL` host such as `10.0.2.2`.

Production builds default this behavior to `false`; production playback should use the exact URL returned by the backend.

## Sync Boundary

Android follows the backend room authority state:

- Host controls emit WebSocket control events.
- Viewers apply server events and room state snapshots.
- `seq` guards stale control events.
- Heartbeat messages are acknowledged with `heartbeat_ack`.
- Room detail HTTP data supplies user-facing room and media metadata.
- WebSocket messages supply live position, pause state, playback rate, ended state, and sequence.

## Tests

The current Android test tree includes coverage for app config, protocol decoding, room sync coordination, room theater page behavior, player cookie bridging, and player page boundaries.
