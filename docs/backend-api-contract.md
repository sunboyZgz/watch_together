# Backend API Contract

The HTTP API is implemented in `server/internal/transport` and registered in `server/internal/app/server.go`.

## Envelope

Success responses use:

```json
{
  "data": {},
  "meta": {
    "requestId": "local"
  }
}
```

Paged success responses add `meta.page`:

```json
{
  "data": {},
  "meta": {
    "requestId": "local",
    "page": {
      "limit": 20,
      "nextCursor": null
    }
  }
}
```

Errors use:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "invalid request body",
    "details": {}
  },
  "meta": {
    "requestId": "local"
  }
}
```

Most authenticated endpoints require:

```text
Authorization: Bearer <accessToken>
```

## Health

### `GET /healthz`

Returns plain text:

```text
ok
```

## Auth

### `POST /auth/register`

Request:

```json
{
  "account": "alice",
  "password": "password",
  "nickname": "Alice"
}
```

Response status: `201 Created`.

```json
{
  "data": {
    "user": {
      "id": "uuid",
      "account": "alice",
      "nickname": "Alice",
      "avatarSeed": "alice",
      "avatarUrl": null
    },
    "accessToken": "jwt"
  },
  "meta": { "requestId": "local" }
}
```

### `POST /auth/login`

Request:

```json
{
  "account": "alice",
  "password": "password"
}
```

Response status: `200 OK`, with the same body shape as registration.

## Home

### `GET /home/summary`

Requires a bearer token.

Response:

```json
{
  "data": {
    "user": {
      "nickname": "Alice",
      "avatarSeed": "alice",
      "avatarUrl": null
    },
    "lastWatched": {
      "mediaItemId": "episode-uuid",
      "title": "Episode title",
      "coverUrl": null,
      "lastPositionSeconds": 120,
      "durationSeconds": 1800
    },
    "continueWatching": []
  },
  "meta": { "requestId": "local" }
}
```

## Media Catalog

### `GET /media/tags`

Does not require a bearer token in the current handler.

Response:

```json
{
  "data": {
    "featuredTags": [
      { "id": "uuid", "slug": "anime", "name": "Anime" }
    ],
    "allTags": [
      { "id": "uuid", "slug": "anime", "name": "Anime" }
    ]
  },
  "meta": { "requestId": "local" }
}
```

### `GET /media/items`

Requires a bearer token.

Query parameters:

- `query`: optional text search.
- `tag`: optional tag slug.
- `limit`: optional positive integer.
- `cursor`: optional pagination cursor.

Response:

```json
{
  "data": {
    "items": [
      {
        "id": "episode-uuid",
        "title": "Episode title",
        "subtitle": null,
        "description": null,
        "coverUrl": null,
        "mediaUrl": "http://127.0.0.1:8080/media/playback/episode-uuid/master.m3u8?expires=...&sig=...",
        "durationMs": 1800000,
        "seasonLabel": "Season 1",
        "episodeLabel": "Episode 1",
        "tags": [
          { "slug": "anime", "name": "Anime" }
        ]
      }
    ]
  },
  "meta": {
    "requestId": "local",
    "page": {
      "limit": 20,
      "nextCursor": null
    }
  }
}
```

`items[].id` is currently `media_episodes.id`.

## Rooms

### `POST /rooms`

Requires a bearer token. Creates a persistent room, registers it in the runtime room manager, and returns the initial sync state.

Request:

```json
{
  "mediaItemId": "episode-uuid"
}
```

Response status: `201 Created`.

```json
{
  "data": {
    "room": {
      "id": "room-uuid",
      "roomCode": "ABC123",
      "hostUserId": "user-uuid",
      "mediaItemId": "episode-uuid",
      "status": "active"
    },
    "media": {
      "id": "episode-uuid",
      "title": "Episode title",
      "subtitle": null,
      "mediaUrl": "http://127.0.0.1:8080/media/playback/episode-uuid/master.m3u8?expires=...&sig=...",
      "durationMs": 1800000,
      "seasonLabel": "Season 1",
      "episodeLabel": "Episode 1"
    },
    "roomState": {
      "mediaDurationMs": 1800000,
      "paused": true,
      "positionMs": 0,
      "velocity": 0,
      "serverTimeMs": 0,
      "reason": "init",
      "playbackRate": 1,
      "ended": false,
      "seq": 0
    }
  },
  "meta": { "requestId": "local" }
}
```

### `POST /rooms/{roomCode}/join`

Requires a bearer token. Writes or restores the current user as an active member of the room. The Android client calls this before WebSocket `join_room`.

Response:

```json
{
  "data": {
    "room": {
      "id": "room-uuid",
      "roomCode": "ABC123",
      "hostUserId": "host-user-uuid",
      "mediaItemId": "episode-uuid",
      "status": "active"
    },
    "member": {
      "userId": "user-uuid",
      "nickname": "Alice",
      "avatarSeed": "alice",
      "avatarUrl": null,
      "role": "member"
    }
  },
  "meta": { "requestId": "local" }
}
```

### `GET /rooms/{roomCode}`

Requires a bearer token and active room membership.

Response:

```json
{
  "data": {
    "room": {
      "id": "room-uuid",
      "roomCode": "ABC123",
      "hostUserId": "host-user-uuid",
      "mediaItemId": "episode-uuid",
      "status": "active"
    },
    "media": {
      "id": "episode-uuid",
      "title": "Episode title",
      "subtitle": null,
      "mediaUrl": "http://127.0.0.1:8080/media/playback/episode-uuid/master.m3u8?expires=...&sig=...",
      "durationMs": 1800000,
      "seasonLabel": "Season 1",
      "episodeLabel": "Episode 1"
    },
    "members": [
      {
        "userId": "host-user-uuid",
        "nickname": "Host",
        "avatarSeed": "host",
        "avatarUrl": null,
        "role": "host"
      }
    ]
  },
  "meta": { "requestId": "local" }
}
```

## Progress

### `PUT /me/media-progress/{mediaItemId}`

Requires a bearer token. The path `mediaItemId` is currently a `media_episodes.id`.

Request:

```json
{
  "lastPositionSeconds": 120,
  "durationSeconds": 1800,
  "completed": false,
  "completionSource": null
}
```

Response:

```json
{
  "data": {
    "mediaItemId": "episode-uuid",
    "lastPositionSeconds": 120,
    "durationSeconds": 1800,
    "completed": false,
    "lastWatchedAt": "2026-06-03T00:00:00Z"
  },
  "meta": { "requestId": "local" }
}
```

## Playback And Media Auth

### `GET /media/playback/{episodeId}/{assetPath}`

Accepts `GET` and `HEAD`. The URL must include valid `expires` and `sig` query parameters generated by the server. For `signed_redirect` and `nginx_auth_request`, clients normally start with `master.m3u8`.

### `/media/internal/auth`

Used by Nginx `auth_request` mode. It checks the `wt_media_access` cookie and `X-Original-URI`; direct clients should not call it.

## Common Error Codes

- `VALIDATION_ERROR`: malformed input, wrong method, invalid cursor, or invalid route path.
- `UNAUTHORIZED`: missing/invalid token, invalid credentials, or missing user.
- `FORBIDDEN`: room membership required.
- `NOT_FOUND`: room or media item not found.
- `CONFLICT`: duplicate account or unable to generate a room code.
- `INTERNAL_ERROR`: service unavailable or unexpected backend failure.
