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
ClientConnection write semaphore: context-aware per-socket write serialization
```

The broadcaster is responsible for:

```text
bounded fan-out concurrency
per-write timeout
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

## Current Policy

Defaults:

```text
broadcast concurrency limit: 64
write timeout: 3s
close slow client on broadcast write timeout: true
```

Timeout policy:

```text
if a client write exceeds the broadcast write timeout, count it as failed and timed out
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
duration
slowest user
slowest write duration
```

## What This Does Not Do Yet

This phase does not implement per-client outbound queues yet.

That remains a larger runtime change because queued writes change when a broadcast is considered delivered:

```text
direct write: broadcast latency measures actual socket writes
queued write: broadcast latency measures enqueue latency unless a delivery ack layer is added
```

## Next Broadcast Architecture Step

When room sizes or multi-instance needs require it, add:

```text
per-client outbound queue
latest-room-state coalescing
slow-client backpressure policy
room-scoped broadcaster interface
Redis pub/sub or stream fan-out for multi-instance delivery
metrics for queue depth and dropped/coalesced messages
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
