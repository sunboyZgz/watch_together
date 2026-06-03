# Shared

`shared/` is reserved for cross-client protocol or schema artifacts. The current implementation keeps the authoritative protocol definitions in the Go server and Android client code instead of this directory.

Current source-of-truth docs:

- [WebSocket protocol](../docs/websocket-protocol.md)
- [Backend API contract](../docs/backend-api-contract.md)

Add files here only when a shared artifact is consumed by more than one runtime.
