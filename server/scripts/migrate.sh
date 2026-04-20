#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${SERVER_DIR}/migrations"

if ! command -v migrate >/dev/null 2>&1; then
  echo "error: migrate CLI not found"
  echo "install: brew install golang-migrate"
  exit 1
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "error: DATABASE_URL is not set"
  echo "example: export DATABASE_URL=postgres://user:pass@localhost:5432/watch_together?sslmode=disable"
  exit 1
fi

if [[ $# -lt 1 ]]; then
  echo "usage: ./scripts/migrate.sh <command> [args...]"
  echo "commands: up | down <n> | version | force <version>"
  exit 1
fi

command="$1"
shift || true

case "${command}" in
  up)
    migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" up
    ;;
  down)
    if [[ $# -lt 1 ]]; then
      echo "usage: ./scripts/migrate.sh down <n>"
      exit 1
    fi
    migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" down "$1"
    ;;
  version)
    migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" version
    ;;
  force)
    if [[ $# -lt 1 ]]; then
      echo "usage: ./scripts/migrate.sh force <version>"
      exit 1
    fi
    migrate -path "${MIGRATIONS_DIR}" -database "${DATABASE_URL}" force "$1"
    ;;
  *)
    echo "error: unsupported command '${command}'"
    echo "commands: up | down <n> | version | force <version>"
    exit 1
    ;;
esac
