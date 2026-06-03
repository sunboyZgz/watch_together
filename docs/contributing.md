# Contributing

This repository is a small monorepo. Keep changes scoped, implementation-driven, and easy for the next developer to verify.

## Repository Boundaries

- `android/`: Android Kotlin client.
- `server/`: Go backend, `mediactl`, migrations, Compose, deployment files.
- `docs/`: durable documentation for current behavior.
- `media/`: local media workspace.
- `shared/`: reserved shared protocol/schema area.
- `scripts/`: repository helper scripts.
- `windows/`: reserved Windows client area.

Do not add new top-level directories unless they have a long-term ownership boundary.

## Development Conventions

- Prefer small, focused changes.
- Update docs when behavior, configuration, commands, or payloads change.
- Keep schema changes in SQL migrations under `server/migrations/`.
- Keep Android runtime endpoint values configurable through Gradle/local properties.
- Do not document planned work as implemented behavior.

## Branch Names

Recommended branch prefixes:

```text
feat/<short-description>
fix/<short-description>
infra/<short-description>
docs/<short-description>
refactor/<short-description>
```

Use lowercase words separated by `-`.

## Commit Messages

Use short conventional-style messages:

```text
feat: add room detail endpoint
fix: reject stale seek events
docs: update media ingest guide
infra: add production compose defaults
refactor: split player shell
chore: update dependencies
```

## Documentation Maintenance

The docs directory should stay concise and current:

- Keep one primary document per topic.
- Remove phase notes, agent briefs, temporary TODOs, and duplicated explanations when they no longer help explain the current code.
- Link to code-adjacent READMEs for implementation-specific details instead of copying them.
- When uncertain, state the uncertainty rather than inventing behavior.
