# Server Timeline Business Integration

> Purpose: summarize how the generic timeline vector is now bound to room/media business behavior.

## Layering

```text
realtime.TimelineVector
  generic timing math

room.Room / room.State
  room id, media id, host policy, media bounds, ended policy, intended playback rate

transport roomSyncView
  HTTP/WebSocket compatibility response fields
```

The timeline package remains media-agnostic. The room layer gives the vector business meaning.

## Media Binding

Runtime rooms can now carry:

```text
mediaId
mediaDurationMs optional
```

`mediaDurationMs` becomes the timeline end bound:

```text
TimelineBounds{StartMs: 0, EndMs: mediaDurationMs}
```

When duration is known:

```text
seek beyond duration clamps to duration
derived playing position clamps to duration
room_state exposes mediaDurationMs
```

When duration is unknown, the timeline remains unbounded except for the default non-negative start.

## Business Policies Implemented

```text
host-only controls remain unchanged
play derives server position and ignores client position hint
pause derives server position before freezing
seek uses requested target and clamps to media duration when known
rate change updates intended playbackRate; paused vectors remain paused
ended wraps the generic StopAt transition with reason media_end
play after ended at media end does not auto-replay
replay is an explicit client flow: seek to 0, then play
media metadata refresh for the same media does not reset the vector
media change creates a new paused vector at position 0 with reason media_change
```

## Replay And Next Episode Boundary

The server should not guess whether an ended room should replay the same media or advance to the next episode.

Recommended explicit flows:

```text
replay current media:
  client sends seek(positionMs=0)
  client sends play

play next episode:
  client calls a future room media-change API with the next mediaId
  server creates a new vector at position 0, velocity 0, reason media_change
  client or host then sends play if auto-start is desired
```

This keeps product choice on the client/UI side while the server remains the authoritative executor of explicit timeline changes.

## HTTP/Runtime Integration

HTTP room creation and room detail bootstrap now pass media duration into the runtime room registry:

```text
RegisterCreatedRoomWithMedia(roomCode, hostUserID, mediaID, durationMs)
```

Join-by-code also refreshes runtime media metadata when the persistent store returns it.

## Compatibility View

`room.State` is still an internal runtime state. External responses go through `newRoomSyncView`:

```text
paused = velocity == 0
mediaDurationMs = current bound, if known
```

This keeps compatibility fields out of the generic timeline package.

## Remaining Business Integration Work

```text
persisted media switching API
buffering policy that may pause/resume the room timeline
progress persistence from authoritative timeline snapshots
multi-instance fan-out with Redis pub/sub or streams
request id deduplication and stale-control policy
full migration from legacy control broadcasts to room_state
```
