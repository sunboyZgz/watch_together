# Server Load Testing

> Purpose: define the first practical load-test entrypoint for multi-user synchronized watching.

## Tool Choice

Use `k6` for the current stage.

Reasons:

- Mature HTTP and WebSocket load-testing tool.
- Scriptable setup can register/login users, create a room, join viewers, and then hold WebSocket connections.
- Does not add runtime dependencies to the Go server.
- Easy to run locally before CI or dedicated load-test infrastructure exists.

This is not a replacement for Go unit tests or race tests. It is an operational pressure test for broadcast, heartbeat, reconnect-adjacent behavior, and control-event fan-out.

## Script

```text
server/scripts/load/ws_room_load.js
```

The script supports:

- single-room 2 / 5 / 10 / 20 / 100 viewer pressure
- automatic account registration/login
- automatic room creation when `MEDIA_ID` is provided
- existing-room mode when `ROOM_CODE` and `USER_TOKENS` are provided
- WebSocket `/ws` join flow with JWT
- host control loop for `play / pause / seek`
- heartbeat ack
- basic metrics for connect errors, protocol errors, join latency, room_state count, and control broadcasts

## Local Run

Start the server first:

```bash
cd server
go run ./cmd/roomserver
```

Run a 20-connection test with auto-created users and room:

```bash
k6 run \
  -e BASE_URL=http://127.0.0.1:8080 \
  -e MEDIA_ID=<existing_media_episode_id> \
  -e VUS=20 \
  -e DURATION=60s \
  -e HOLD_OPEN_MS=60000 \
  server/scripts/load/ws_room_load.js
```

Run the 100-room-target check:

```bash
k6 run \
  -e BASE_URL=http://127.0.0.1:8080 \
  -e MEDIA_ID=<existing_media_episode_id> \
  -e VUS=100 \
  -e DURATION=120s \
  -e HOLD_OPEN_MS=120000 \
  server/scripts/load/ws_room_load.js
```

Use an existing room and existing tokens:

```bash
k6 run \
  -e BASE_URL=http://127.0.0.1:8080 \
  -e ROOM_CODE=A7K2M9 \
  -e AUTO_REGISTER=false \
  -e USER_TOKENS='[{"userId":"user_a","token":"..."},{"userId":"user_b","token":"..."}]' \
  -e VUS=2 \
  server/scripts/load/ws_room_load.js
```

## Metrics To Watch

In k6 output:

- `ws_connect_errors`: should be 0.
- `ws_protocol_errors`: should be close to 0.
- `join_latency_ms`: p95 should stay below 2s locally.
- `room_state_received`: should be at least the number of joined clients.
- `control_broadcasts`: should grow with host controls multiplied by active clients.

In server logs when `DEBUG_SYNC=true`:

- `clients`
- `failed`
- `timed_out`
- `closed`
- `coalesced`
- `queue_pressure`
- `max_queue_depth`
- `duration_us`
- `slowest_user`

## Acceptance For Current Stage

Minimum acceptance:

- 20 clients, 60 seconds, no WebSocket connect errors.
- 20 clients, host control every 1.5 seconds, no sustained queue pressure.
- 100 clients, 120 seconds, no goroutine leak symptoms or mass timeout closure.

If 100 clients fail locally, record:

- hardware
- server config
- `BROADCAST_CONCURRENCY_LIMIT`
- `CLIENT_OUTBOX_CAPACITY`
- p95 join latency
- timeout/closed client count
- max queue depth

## Known Limits

- The script is single-room focused.
- It intentionally uses unique users per VU so it does not trigger active-room-device switch approval.
- It does not yet simulate slow clients.
- It does not yet simulate reconnect storms.
- It does not measure HLS segment pressure.

Those are separate follow-up tests after baseline room broadcast capacity is understood.
