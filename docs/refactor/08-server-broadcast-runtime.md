# Server Broadcast Runtime

> Purpose: document the high-concurrency broadcast boundary introduced after the timeline business integration.

## Current Boundary

`websocket_handler.go` should not own fan-out mechanics.

The handler is responsible for:

```text
decode protocol message
call room control method
build protocol envelope
call broadcaster
```

The current code boundary is:

```text
roomBroadcaster interface: transport-level fan-out contract
boundedBroadcaster: current in-process implementation
broadcastConfig: named runtime policy knobs
clientWriter: minimal connection write/close boundary
ClientConnection outbox: per-client outbound queue
ClientConnection write semaphore: context-aware per-socket write serialization
protocol.Envelope OutboxCoalesceKey: identifies coalescable outbound messages
```

The broadcaster is responsible for:

```text
bounded fan-out concurrency
per-client enqueue timeout
slow client close policy
broadcast stats
future fan-out implementation swaps
```

## Mature Tooling

The bounded broadcaster uses:

```text
golang.org/x/sync/semaphore
```

This is a mature Go extended-library primitive and avoids hand-maintaining ad hoc concurrency counters.
The current semaphore is process-wide per `WebSocketHandler`, not per room. This protects the server from aggregate broadcast pressure across rooms.
The client connection also uses a one-slot semaphore for writes, so waiting behind another write can respect the same context timeout.
Broadcast now enqueues to each client outbox. Actual socket writes happen in `ClientConnection.RunWriteLoop`.
`room_state` messages are coalesced in each client outbox: if an older queued `room_state` exists, the newer one replaces it.

## Current Policy

Defaults:

```text
broadcast concurrency limit: 64
broadcast total timeout: 5s
client outbox capacity: 64
enqueue timeout: 3s
close slow client on broadcast enqueue timeout: true
single-process max websocket connections: unlimited by default
per-room max clients: unlimited by default
0 means unlimited for connection and room-member limits
```

Environment variables:

```text
WS_BROADCAST_CONCURRENCY_LIMIT
WS_BROADCAST_TIMEOUT_MS
WS_BROADCAST_ENQUEUE_TIMEOUT_MS
WS_CLIENT_OUTBOX_CAPACITY
WS_MAX_CONNECTIONS
ROOM_MAX_CLIENTS
```

Timeout policy:

```text
if the whole broadcast exceeds the total timeout, stop scheduling new clients
if a client outbox cannot accept a message before the enqueue timeout, count it as failed and timed out
close that client with websocket policy violation
do not hold the stats mutex while closing the websocket
if the parent broadcast context is canceled, count unscheduled clients as failed
```

Stats tracked:

```text
clients
failed clients
timed out clients
closed clients
coalesced clients
max queue depth
duration
slowest user
slowest enqueue duration
```

## Delivery Semantics

Broadcast completion now means the message has been accepted into each target client's outbox, not necessarily written to the network yet:

```text
room transition: committed before broadcast
broadcast success: queued for the target client
writer loop: eventually writes queued messages to the websocket
```

This is intentional for authoritative timeline state. A slow client should not block room state transitions or other clients.

## Coalescing Policy

Only messages that explicitly expose an outbox coalesce key are eligible for replacement.

Current coalescing:

```text
room_state: remove the older queued state and append the latest state to the tail
```

Current non-coalescing:

```text
play
pause
seek
set_playback_rate
ended
room_members_changed
heartbeat
error
clock_sync.pong
```

This keeps event-like protocol messages ordered while allowing high-value state snapshots to collapse to the latest authority state.

## Capacity Factors

The practical connection and room-size ceiling is affected by:

```text
machine memory
websocket library overhead
client outbox capacity
average message size
heartbeat frequency
operating-system file descriptor limits
network write throughput
```

Treat these as deployment and load-test inputs. Do not assume the per-client outbox model provides unlimited fan-out capacity.

## Next Broadcast Architecture Step

When room sizes or multi-instance needs require it, add:

```text
queue-full close or degradation policy
broader coalescing policy for future high-frequency state streams
slow-client backpressure policy
room-scoped broadcaster interface
Redis pub/sub or stream fan-out for multi-instance delivery
metrics export for queue depth and dropped/coalesced messages
```

## Redis Boundary

Redis may help with:

```text
multi-instance fan-out
presence summaries
control request deduplication windows
latest vector cache for reconnect/debug
```

Redis must not become the authoritative room timeline. The in-process room runtime still serializes accepted timeline transitions.
