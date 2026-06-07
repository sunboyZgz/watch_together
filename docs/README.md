# Documentation

This directory is the durable source of truth for the current `watch_together` implementation. It describes what is in the code today, not planned work or historical refactor notes.

## Start Here

- [Overview](./overview.md) - product scope, runtime architecture, and module boundaries.
- [Setup And Configuration](./setup-and-configuration.md) - local services, server config, Android flavor config, and runtime commands.
- [Backend API Contract](./backend-api-contract.md) - HTTP envelope, authentication, media, room, progress, and playback endpoints.
- [WebSocket Protocol](./websocket-protocol.md) - room sync event envelope, state model, and control rules.
- [Runtime Boundaries](./runtime-boundaries.md) - current state ownership and Phase 1 statelessness boundary markers.
- [Database Ownership](./database-ownership.md) - logical table ownership registry, architecture checks, and future physical split checklist.
- [Media Operations](./media-operations.md) - `mediactl`, storage drivers, HLS generation, and delivery modes.
- [Android Client](./android-client.md) - app flow, client modules, player behavior, and configuration.
- [Data Model](./data-model.md) - PostgreSQL tables, Redis cache boundary, and media ID semantics.
- [Contributing](./contributing.md) - repository conventions and documentation maintenance rules.

## Repository-Level References

Some implementation-specific docs live beside the code they describe:

- [server/README.md](../server/README.md)
- [server/deploy/README.md](../server/deploy/README.md)
- [server/migrations/README.md](../server/migrations/README.md)
- [android/README.md](../android/README.md)

## Documentation Rules

- Treat code, migrations, and configuration examples as the primary source of truth.
- Do not document planned or rejected features as implemented behavior.
- Keep one primary document per topic and link to it instead of duplicating large sections.
- Move short-lived agent briefs, phase closeouts, TODO lists, and investigation notes out of `docs/` when they no longer explain the current system.
