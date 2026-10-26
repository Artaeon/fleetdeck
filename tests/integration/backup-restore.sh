#!/usr/bin/env bash
# Real SQLite restore drill on a disposable project. No host services touched.
set -euo pipefail
binary=${FLEETDECK_BINARY:?Set FLEETDECK_BINARY to a built CLI}
base=$(mktemp -d)
name="backup-drill-$$"
cleanup() {
  docker compose -f "$base/project/docker-compose.yml" down --volumes >/dev/null 2>&1 || true
  rm -rf "$base"
}
trap cleanup EXIT
mkdir -p "$base/project/nested/state" "$base/control"
cat > "$base/config.toml" <<EOF
[server]
base_path = "$base/control"
[backup]
base_path = "$base/backups"
auto_snapshot = false
[audit]
enabled = false
EOF
cat > "$base/project/docker-compose.yml" <<EOF
name: $name
services:
  app:
    image: python:3.14-alpine
    command: [python3, -c, "import time; time.sleep(3600)"]
    volumes:
      - ./nested/state:/state
      - ./nginx.conf:/etc/probe.conf:ro
    network_mode: none
    security_opt: [no-new-privileges:true]
    cap_drop: [ALL]
EOF
printf 'public container configuration' > "$base/project/nginx.conf"
chmod 0644 "$base/project/nginx.conf"
cli() { "$binary" --config "$base/config.toml" "$@"; }
cli import "$name" --path "$base/project" --domain example.invalid
cli start "$name"
docker compose -f "$base/project/docker-compose.yml" exec -T app python3 - <<'PY'
import sqlite3
with sqlite3.connect('/state/example.sqlite3') as db:
    db.execute('CREATE TABLE evidence(value TEXT)')
    db.execute("INSERT INTO evidence VALUES ('before')")
PY
cli stop "$name"
cli backup create "$name" --skip-db
manifest=$(find "$base/backups/$name" -name manifest.json -print -quit)
id=$(basename "$(dirname "$manifest")")
printf 'stale journal' > "$base/project/nested/state/example.sqlite3-wal"
printf 'new file' > "$base/project/nested/state/created-after-backup"
cli rollback "$name" --backup-id "$id" --no-snapshot
docker compose -f "$base/project/docker-compose.yml" exec -T app python3 - <<'PY'
import sqlite3
from pathlib import Path
assert not Path('/state/created-after-backup').exists()
assert not Path('/state/example.sqlite3-wal').exists()
with sqlite3.connect('/state/example.sqlite3') as db:
    assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
    assert db.execute('SELECT value FROM evidence').fetchall() == [('before',)]
PY
docker compose -f "$base/project/docker-compose.yml" exec -T --user 65534 app python3 -c "assert open('/etc/probe.conf').read() == 'public container configuration'"
# Unsafe restore targets must fail before touching a live deployment or an
# unrelated host file, even when all archived configuration checksums match.
container=$(docker compose -f "$base/project/docker-compose.yml" ps -q app)
assert_running() {
  test "$(docker inspect --format '{{.State.Running}}' "$container")" = true
}
assert_rejected() {
  if cli rollback "$name" --backup-id "$id" --no-snapshot; then
    echo 'Unsafe restore was accepted' >&2
    exit 1
  fi
  assert_running
  test "$(cat "$base/outside")" = 'untouched'
}
printf 'untouched' > "$base/outside"
mv "$base/project/nginx.conf" "$base/original-config"
ln -s "$base/outside" "$base/project/nginx.conf"
assert_rejected
rm "$base/project/nginx.conf"
mv "$base/original-config" "$base/project/nginx.conf"

cp "$manifest" "$base/original-manifest.json"
for attack in source-traversal target-traversal host-volume unknown-type; do
  python3 - "$base/original-manifest.json" "$manifest" "$attack" <<'PYTHON'
import json, sys
manifest = json.load(open(sys.argv[1]))
component = manifest['components'][0]
attack = sys.argv[3]
if attack == 'source-traversal':
    component['path'] = '../outside'
elif attack == 'target-traversal':
    component['name'] = '../outside'
elif attack == 'host-volume':
    component.update(type='volume', name='/tmp (named volume)', path='volumes/namedvol_data.tar.gz')
else:
    component['type'] = 'unsupported'
with open(sys.argv[2], 'w') as output:
    json.dump(manifest, output)
PYTHON
  assert_rejected
done
cp "$base/original-manifest.json" "$manifest"
# Corrupt backups must fail before stopping the current working deployment.
printf 'corrupt' > "$(dirname "$manifest")/config/docker-compose.yml"
if cli rollback "$name" --backup-id "$id" --no-snapshot; then
  echo 'Corrupt backup was accepted' >&2
  exit 1
fi
test "$(docker compose -f "$base/project/docker-compose.yml" ps -q --status running | wc -l | tr -d ' ')" = 1
printf 'SQLite restore, stale-file removal, unsafe-path and corrupt-backup rejection passed.\n'
