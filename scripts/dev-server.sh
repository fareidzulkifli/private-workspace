#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${ROOT_DIR}/.env.local"

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf "%s" "$value"
}

load_env_file() {
  local line key value

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    line="$(trim "$line")"

    [[ -z "$line" || "${line:0:1}" == "#" ]] && continue
    [[ "$line" == export\ * ]] && line="${line#export }"

    if [[ "$line" != *=* ]]; then
      echo "Invalid line in .env.local: $line" >&2
      exit 1
    fi

    key="$(trim "${line%%=*}")"
    value="$(trim "${line#*=}")"

    if [[ ! "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
      echo "Invalid environment variable name in .env.local: $key" >&2
      exit 1
    fi

    if [[ ${#value} -ge 2 ]]; then
      if [[ ("${value:0:1}" == "'" && "${value: -1}" == "'") || ("${value:0:1}" == '"' && "${value: -1}" == '"') ]]; then
        value="${value:1:${#value}-2}"
      fi
    fi

    export "$key=$value"
  done < "$ENV_FILE"
}

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing .env.local. Copy .env.local.example to .env.local and fill in the local values." >&2
  exit 1
fi

load_env_file

export APP_ENV="${APP_ENV:-development}"
export APP_BASE_URL="${APP_BASE_URL:-http://localhost:4000}"
export HTTP_ADDR="${HTTP_ADDR:-127.0.0.1:4000}"
export SQLITE_PATH="${SQLITE_PATH:-./data/private-workspace.db}"
export MIGRATIONS_DIR="${MIGRATIONS_DIR:-./migrations}"
export COOKIE_SECURE="${COOKIE_SECURE:-false}"
export GOCACHE="${GOCACHE:-${ROOT_DIR}/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-${ROOT_DIR}/.cache/go-mod}"

: "${APP_SECRET:?Missing APP_SECRET in .env.local}"
: "${ADMIN_EMAIL:?Missing ADMIN_EMAIL in .env.local}"
: "${ADMIN_PASSWORD_HASH:?Missing ADMIN_PASSWORD_HASH in .env.local}"

cd "$ROOT_DIR"

echo "Starting Private Workspace at ${APP_BASE_URL}"
echo "SQLite database: ${SQLITE_PATH}"

exec go run ./cmd/server
