#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${SERVER_DIR}/migrations"

if [[ $# -lt 1 ]]; then
  echo "usage: ./scripts/new_migration.sh <snake_case_name>"
  exit 1
fi

name="$1"
timestamp="$(date '+%Y%m%d%H%M%S')"
up_file="${MIGRATIONS_DIR}/${timestamp}_${name}.up.sql"
down_file="${MIGRATIONS_DIR}/${timestamp}_${name}.down.sql"

mkdir -p "${MIGRATIONS_DIR}"

cat > "${up_file}" <<'SQL'
-- Write forward migration here.
SQL

cat > "${down_file}" <<'SQL'
-- Write rollback migration here.
SQL

echo "created:"
echo "  ${up_file}"
echo "  ${down_file}"
