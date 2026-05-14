# Server Clock Sync Phase 5

> Purpose: document the lightweight WebSocket clock sync contract added after the server timeline vector model.

## Why This Exists

Room timeline vectors include `serverTimeMs`. Clients need a cheap way to estimate server wall time so they can derive the current room position locally:

```text
currentPositionMs = positionMs + velocity * (estimatedServerNowMs - serverTimeMs)
```

Clock sync is only for estimating server time. It does not change room state.

## Messages

Client sends:

```json
{
  "type": "clock_sync.ping",
  "payload": {
    "clientSendMonoMs": 100000
  }
}
```

Server replies:

```json
{
  "type": "clock_sync.pong",
  "payload": {
    "serverTimeMs": 1710000000000,
    "clientSendMonoMs": 100000
  }
}
```

## Boundary

The server should reply quickly:

```text
no room lookup
no database work
no Redis work
no broadcast
no seq increment
```

Heartbeat remains a separate liveness contract. Clock sync pings are not heartbeat acknowledgements.

## Client Use

A client can estimate offset with a standard round-trip calculation:

```text
clientReceiveMonoMs = monotonic time when pong arrives
rttMs = clientReceiveMonoMs - clientSendMonoMs
estimatedServerAtReceiveMs = serverTimeMs + rttMs / 2
serverOffsetMs = estimatedServerAtReceiveMs - clientReceiveMonoMs
```

Then for any room vector:

```text
estimatedServerNowMs = clientNowMonoMs + serverOffsetMs
```

Clients should sample multiple times and prefer a low-RTT sample or a smoothed offset.

## Acceptance

```text
[x] Protocol constants exist for clock_sync.ping and clock_sync.pong.
[x] Ping payload carries clientSendMonoMs.
[x] Pong payload returns serverTimeMs and the original clientSendMonoMs.
[x] Handler returns pong without room or storage work.
[x] Heartbeat and clock sync remain separate.
```
