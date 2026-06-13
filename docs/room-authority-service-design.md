# Room Authority Service Design

Phase 13 defines the target `room-authority-service` boundary. Phase 14 implements a non-default RPC pilot under `rpc-pilot`. Phase 15 hardens that pilot with dynamic readiness, metrics, stable failure tests, and explicit lease-owner identity. Phase 16 promotes authority RPC into the local compose `app` development path, while production compose still keeps room authority inside `roomserver` by default. Phase 17/18 adds identity RPC to the same local app baseline; Phase 19 adds room metadata and membership RPC through `cmd/roomservice`; Phase 21 moves room lifecycle and recovery bootstrap reads behind room RPC. Phase 24 adds a production `authority-rpc-canary` profile without changing the production default. These changes do not change the authority contract. Android HTTP/WebSocket contracts remain unchanged.

## Goals

- Prepare a `room-authority-service` split that can be implemented in Phase 14 without changing Android HTTP/WebSocket payloads.
- Move authority decision logic toward a `roomId` actor model while keeping WebSocket connections and client-visible envelopes in `roomserver`.
- Preserve Phase 12 timeline ownership: `timelineservice` remains responsible for canonical result event generation, outbox writes, and recovery feed composition.
- Keep Kafka as a result log. Command inbox or command ingress is a later optional phase, not part of Phase 13 or Phase 14.

## Non-Goals

- Do not add `server/api/internal/v1/authority.proto` in Phase 13.
- Do not add `cmd/roomauthorityservice` or compose entries in Phase 13.
- Do not route production or local compose traffic through a new authority service in Phase 13.
- Do not route app/prod default traffic through authority RPC in Phase 14.
- Do not route production default traffic through authority RPC in Phase 16.
- Do not route production default traffic through authority RPC in Phase 24; only the explicit canary profile may do that.
- Do not move WebSocket connection objects, send queues, heartbeat state, or local client tables out of `roomserver`.
- Do not change Android HTTP/WebSocket request or response shapes.

## Phase 14 Implementation Status

Phase 14 adds generated internal authority RPC and `cmd/roomauthorityservice` for `server/compose.yaml --profile rpc-pilot`.

- `AUTHORITY_SERVICE_MODE=local` remains the default.
- `rpc-pilot` starts `roomauthorityservice` and uses `AUTHORITY_SERVICE_MODE=rpc`.
- `roomserver` remains the WebSocket and HTTP edge in every mode.
- `timelineservice` remains the owner of canonical result event generation and recovery feeds.
- Kafka remains a result log, not command ingress.

## Phase 15 Hardening Status

At the end of Phase 15, authority RPC was still non-default and focused on cutover readiness for `rpc-pilot`.

- `AUTHORITY_LEASE_INSTANCE_ID` is the Redis authority lease owner used by `roomserver` HTTP bootstrap when `AUTHORITY_SERVICE_MODE=rpc`.
- `roomauthorityservice` readiness checks Redis, NATS broadcast connectivity, timeline RPC readiness, room RPC readiness, and internal RPC availability.
- `roomauthorityservice` exposes authority RPC request count/latency/error metrics and authority apply accepted/rejected metrics.
- Phase 15 architecture tests kept app/prod local while only `roomserver-rpc-pilot` depended on `roomauthorityservice` and used authority RPC. Phase 16 updates that guard so local app now uses authority RPC and prod remains local.
- Failure tests cover authority RPC unavailable and stale authority responses so ingress forgets pending requests and returns stable client errors.

## Phase 16 Local App Status

Phase 16 makes local compose `app` the default full-RPC serviceized development path.

- `server/compose.yaml --profile app` starts `mediaservice`, `timelineservice`, and `roomauthorityservice`.
- App `roomserver` sets `ROOM_RUNTIME_MODE=distributed_authority`, `MEDIA_SERVICE_MODE=rpc`, `TIMELINE_SERVICE_MODE=rpc`, and `AUTHORITY_SERVICE_MODE=rpc`.
- App `roomserver` also sets `IDENTITY_SERVICE_MODE=rpc`, so register/login/token verification can cross the identity service boundary.
- App `roomserver` sets `ROOM_SERVICE_MODE=rpc`, so room create/join/detail and WebSocket membership checks can cross the room service boundary.
- App `roomserver` uses `AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-1` by default so HTTP bootstrap claims Redis authority leases for the service identity.
- App `roomserver /readyz` actively probes `authority_rpc` through `roomauthorityservice /readyz`; unavailable authority RPC makes roomserver readiness `not_ready`.
- `server/compose.prod.yaml` still keeps `AUTHORITY_SERVICE_MODE=local` by default.
- Bare `go run ./cmd/roomserver` keeps local adapters by default for fast single-process debugging.

## Phase 24 Production Canary Status

Phase 24 adds a production canary path without making authority RPC the production default.

- `server/compose.prod.yaml --profile authority-rpc-canary` can start `roomauthorityservice`.
- A canary run must explicitly set `AUTHORITY_SERVICE_MODE=rpc`, `AUTHORITY_SERVICE_ADDR=http://roomauthorityservice:8090`, `AUTHORITY_LEASE_INSTANCE_ID=roomauthorityservice-prod-1`, `ROOM_RUNTIME_MODE=distributed_authority`, and `WS_CROSS_INSTANCE_BROADCAST_ENABLED=true`.
- `roomauthorityservice` depends on Redis, NATS, `roomservice`, and `timelineservice`; it does not receive `DATABASE_URL` or `ROOM_DATABASE_URL`.
- `server/scripts/verify_phase24.ps1` validates the Phase 23 full-RPC baseline, prod canary compose wiring, and authority failure semantics. `-RunSmoke` also runs the full-RPC multi-database HTTP/WebSocket smoke.
- Rollback remains explicit and simple: set `AUTHORITY_SERVICE_MODE=local` and stop the canary profile.

## Current Baseline

Today, `distributed_authority` uses these ownership boundaries:

- `roomserver` owns WebSocket ingress/egress, HTTP room bootstrap, the local connection table, accepted envelope broadcast, NATS control forwarding, and the authoritative `room.Manager` instance for rooms whose Redis lease points to the local instance.
- `roomservice` owns room metadata and membership RPC in compose app/prod paths, while realtime room runtime stays in `roomserver`.
- Phase 21 also routes authority recovery bootstrap reads and recoverable-room metadata through `roomservice`; `roomauthorityservice` does not directly read room tables.
- Redis stores authority leases, active-device leases, control request idempotency, seek rate limits, latest room snapshots, and user-level presence.
- NATS Core handles realtime WebSocket fan-out and non-authority control request/reply between roomserver instances.
- `timelineservice` owns canonical timeline result event construction and recovery-ready event feeds.
- `outboxworker` publishes timeline outbox rows to Kafka, and `derivedworker` publishes derived topics.

This baseline is the parity target for Phase 14 and the production rollback path after Phase 16.

## Target Boundary

`roomserver` remains the client edge:

- Accept HTTP room bootstrap and WebSocket connections.
- Authenticate users and decode/encode Android-facing envelopes.
- Maintain local WebSocket connection tables and send queues.
- Publish accepted envelopes to local clients and NATS broadcast.
- Forward control intent to the authority boundary and return stable protocol errors to clients.

Future `room-authority-service` owns authority decisions:

- Route each room to one logical `roomId` actor.
- Maintain authoritative playback state for actor-owned rooms.
- Validate authority epoch, active device, host permission, control sequence, idempotency, and seek rate limit.
- Apply accepted controls and produce the same client-visible accepted envelopes that `roomserver` produces today.
- Record accepted/rejected results through `timelineservice` before returning accepted control results.
- Coordinate recovery by claiming/renewing/completing Redis authority leases and rebuilding actor state from the timeline recovery feed.

`timelineservice` remains the result fact service:

- Generate canonical event id, event version, and occurrence time.
- Write `room_timeline_outbox`.
- Merge Kafka canonical events with unpublished outbox gaps for recovery.

## Actor Model

The target service should use one serial execution context per `roomId`. The actor is the only place that mutates recovered or live playback state for its room. Actor input order is the order accepted by the service, not Kafka order and not NATS delivery order.

Actor state includes:

- Current playback state equivalent to `room.State`.
- Current authority lease instance and epoch.
- Recent recovered accepted request ids needed to preserve idempotency.
- Optional local caches of active-device and rate-limit decisions, backed by Redis as the shared authority.

Actor activation:

- On first control or recovery request, load durable room metadata from the room-session boundary.
- If there is no active local actor state, recover from `timelineservice.ListRoomRecoveryEvents`.
- Complete Redis recovery only after actor state is registered and ready to answer control requests.

Actor passivation:

- Do not move WebSocket connections.
- Allow actor state to expire only when there is no active room authority lease or after explicit room cleanup.
- Passivation must not delete Redis authority, presence, idempotency, or timeline state.

## Proposed Internal RPC Shape

Phase 13 documented the contract. Phase 14 converts it into generated internal ConnectRPC for the non-default pilot.

Authority lifecycle:

```text
GetRoomAuthority(roomId) -> {found, instanceId, epoch, status, leaseUntilMs}
ClaimRoomAuthority(roomId, instanceId) -> {claimed, lease}
RenewRoomAuthority(roomId, instanceId, epoch) -> {renewed, lease}
BeginRoomRecovery(roomId, instanceId) -> {started, lease}
CompleteRoomRecovery(roomId, instanceId, epoch) -> {completed, lease}
```

Control application:

```text
ApplyRoomControl({
  sourceInstanceId,
  roomId,
  userId,
  deviceId,
  connectionId,
  type,
  payload,
  requestId,
  seq,
  expectedAuthorityEpoch,
  requestedAtMs
}) -> {
  accepted,
  type,
  payload,
  seq,
  authorityEpoch,
  error
}
```

Rules:

- `type` and `payload` are the original Android-facing WebSocket envelope fields.
- `payload` in a successful response is the same accepted envelope payload shape that clients receive today.
- `error` uses existing stable strings such as `room authority unavailable`, `room authority recovering`, `room timeline unavailable`, `control idempotency unavailable`, `control rate limited`, and `only host can control playback`.
- Accepted responses are returned only after timeline result recording succeeds.
- The service must reject stale `expectedAuthorityEpoch` and must not accept results from an old actor epoch.

Recovery handoff:

```text
RecoverRoomAuthority(roomId, instanceId, reason) -> {
  recovered,
  lease,
  state,
  acceptedRequestIds
}
```

This can be an internal helper rather than a public RPC if Phase 14 keeps recovery inside the service process.

## Failure Semantics

- Timeline failure is fail-closed for accepted controls: no accepted response, no accepted idempotency finalization, no broadcast instruction.
- Recovery feed failure does not complete an authority epoch and does not register recovered actor state.
- Redis authority lease mismatch, expired epoch, or recovering status returns stable authority errors.
- Duplicate accepted `requestId` returns the original accepted envelope.
- Duplicate pending `requestId` returns `room authority processing`.
- Non-authority ingress must not accept a stale authority response.
- NATS failure remains an ingress transport failure until Phase 14 replaces or wraps it with authority RPC.

## Migration Plan

Phase 13:

- Keep this design and current docs in sync with the implemented system.
- Do not add generated authority proto or a runtime service.
- Use current NATS control forwarding as the behavioral parity baseline.

Phase 14:

- Add generated internal authority RPC and local/RPC adapter parity tests.
- Add non-default `AUTHORITY_SERVICE_MODE=rpc`.
- Implement `cmd/roomauthorityservice` behind `rpc-pilot`, while keeping roomserver-local authority as the app/prod default during Phase 14.
- Prove accepted/rejected control, recovery, idempotency, active-device, seek-rate, stale-epoch, and timeline-failure parity.

Phase 15:

- Harden `rpc-pilot` readiness, metrics, identity naming, and failure semantics.
- Keep app/prod defaults on local authority during Phase 15.

Phase 16:

- Make local compose `app` default to authority RPC together with media and timeline RPC.
- Add an app full-RPC smoke gate that verifies HTTP bootstrap, WebSocket join, accepted controls, authority metrics, and timeline outbox writes.
- Keep production compose on local authority by default.

Phase 17+:

- Decide whether command inbox or command ingress is necessary.
- Introduce durable command audit only if product or operations requirements justify it.

Phase 24:

- Keep production authority local by default.
- Add the `authority-rpc-canary` prod profile and verify it with compose guards.
- Prove timeout, unavailable, stale response, timeline failure, and stale-epoch semantics before any production default cutover.

## Acceptance Criteria

- Documentation clearly states that `room authority` is still in `roomserver` today.
- Documentation clearly states that Phase 13 is design-only.
- Local compose app and rpc-pilot include `roomauthorityservice`; production compose does not include it by default.
- No generated authority proto or runtime service skeleton is added in Phase 13.
- Kafka is consistently described as a result log, not command ingress.
- Android HTTP/WebSocket contracts remain unchanged.
