#!/usr/bin/env bash
# Disposable PostgreSQL drill: no external networking or existing project data.
set -euo pipefail
binary=${FLEETDECK_BINARY:?Set FLEETDECK_BINARY to a built CLI}
base=$(mktemp -d)
name="database-drill-$$"
cleanup() {
  docker compose -f "$base/project/docker-compose.yml" down --volumes >/dev/null 2>&1 || true
  rm -rf "$base"
}
trap cleanup EXIT
mkdir -p "$base/project" "$base/control"
cat > "$base/config.toml" <<CONFIG
[server]
base_path = "$base/control"
[backup]
base_path = "$base/backups"
auto_snapshot = false
[audit]
enabled = false
CONFIG
cat > "$base/project/docker-compose.yml" <<COMPOSE
name: $name
services:
  db:
    image: postgres:17-alpine
    container_name: $name-db
    environment:
      POSTGRES_DB: fixture
      POSTGRES_HOST_AUTH_METHOD: trust
    volumes: [data:/var/lib/postgresql/data]
    network_mode: none
    mem_limit: 256m
    pids_limit: 64
    healthcheck:
      test: [CMD-SHELL, 'pg_isready -U postgres -d fixture']
      interval: 2s
      timeout: 2s
      retries: 30
  app:
    image: postgres:17-alpine
    command: [sleep, '3600']
    network_mode: none
    user: postgres
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    mem_limit: 32m
    pids_limit: 16
volumes:
  data: {}
COMPOSE
printf 'POSTGRES_USER=postgres\nPOSTGRES_DB=fixture\n' > "$base/project/.env"
chmod 0600 "$base/project/.env"
cli() { "$binary" --config "$base/config.toml" "$@"; }
compose() { docker compose -f "$base/project/docker-compose.yml" "$@"; }
sql() { compose exec -T db psql -X --set=ON_ERROR_STOP=on -U postgres -d fixture -Atc "$1"; }
cli import "$name" --path "$base/project" --domain example.invalid
cli start "$name" --timeout 90s
sql "CREATE TABLE proof(value text); INSERT INTO proof VALUES ('before');"
cli backup create "$name" --skip-volumes
manifest=$(find "$base/backups/$name" -name manifest.json -print -quit)
id=$(basename "$(dirname "$manifest")")
cli backup verify "$name" "$id"
app=$(compose ps -q app)
sql "INSERT INTO proof VALUES ('after');"
printf 'POSTGRES_USER=missing_role\nPOSTGRES_DB=fixture\n' > "$base/project/.env"
# A bad dump must fail both manual backup and the required pre-rollback snapshot.
if cli backup create "$name" --skip-volumes; then
  echo 'Failed database dump was reported as a successful backup' >&2; exit 1
fi
if cli rollback "$name" --backup-id "$id"; then
  echo 'Rollback continued without its required database snapshot' >&2; exit 1
fi
test "$(docker inspect --format '{{.State.Running}}' "$app")" = true
test "$(sql 'SELECT count(*) FROM proof')" = 2
test "$(find "$base/backups/$name" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')" = 1
# Successful logical import into a clean database; only disposable data is removed.
printf 'POSTGRES_USER=postgres\nPOSTGRES_DB=fixture\n' > "$base/project/.env"
sql 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
cli rollback "$name" --backup-id "$id" --no-snapshot
test "$(sql 'SELECT value FROM proof')" = before
test -n "$(compose ps -q --status running app)"
# Valid gzip, invalid SQL: verification alone cannot detect this failure.
python3 - "$manifest" <<'PY'
import gzip, json, pathlib, sys
manifest = pathlib.Path(sys.argv[1])
components = json.loads(manifest.read_text())['components']
dump = next(c for c in components if c['type'] == 'database')
with gzip.open(manifest.parent / dump['path'], 'wb') as output:
    output.write(b'SELECT nonexistent_fixture_column;\nCREATE TABLE must_not_run(id int);\n')
PY
if cli rollback "$name" --backup-id "$id" --no-snapshot; then
  echo 'SQL import failure was reported as successful restore' >&2; exit 1
fi
test -z "$(compose ps -q --status running app)"
test "$(sql "SELECT to_regclass('public.must_not_run') IS NULL")" = t
printf 'PostgreSQL dump failure, safety snapshot refusal, restore and SQL failure tests passed.\n'
