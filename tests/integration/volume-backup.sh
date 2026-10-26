#!/usr/bin/env bash
# Disposable volume inventory/restore drill; no public ports or real data.
set -euo pipefail
binary=${FLEETDECK_BINARY:?Set FLEETDECK_BINARY to a built CLI}
base=$(mktemp -d)
name="volume-drill-$$"
cleanup() {
  if [ -f "$base/compose.original" ]; then cp "$base/compose.original" "$base/project/docker-compose.yml"; fi
  docker compose -f "$base/project/docker-compose.yml" down --volumes >/dev/null 2>&1 || true
  docker volume rm "$name-external" "$name-missing" >/dev/null 2>&1 || true
  rm -rf "$base"
}
trap cleanup EXIT
mkdir -p "$base/project/tmp" "$base/control"
cat > "$base/config.toml" <<CONFIG
[server]
base_path = "$base/control"
[backup]
base_path = "$base/backups"
auto_snapshot = false
[audit]
enabled = false
CONFIG
printf 'DATA_PATH=./tmp\n' > "$base/project/.env"
cat > "$base/project/docker-compose.yml" <<COMPOSE
name: $name
services:
  app:
    image: python:3.14-alpine
    command: [sleep, '3600']
    network_mode: none
    mem_limit: 64m
    pids_limit: 32
    volumes:
      - data:/data
      - custom:/custom
      - external:/external
      - readonly:/readonly:ro
      - type: bind
        source: \${DATA_PATH}
        target: /state
  worker:
    image: python:3.14-alpine
    command: [sleep, '3600']
    network_mode: none
    mem_limit: 32m
    pids_limit: 16
    volumes:
      - data:/data
volumes:
  data: {}
  custom:
    name: $name-custom
  external:
    external: true
    name: $name-external
  readonly: {}
COMPOSE
cp "$base/project/docker-compose.yml" "$base/compose.original"
docker volume create "$name-external" >/dev/null
cli() { "$binary" --config "$base/config.toml" "$@"; }
compose() { docker compose -f "$base/project/docker-compose.yml" "$@"; }
cli import "$name" --path "$base/project" --domain example.invalid
cli start "$name"
compose exec -T app python3 - <<'PY'
import os
from pathlib import Path
for root in ['/data', '/custom', '/external', '/state']:
    path = Path(root) / 'proof'
    path.write_text('before')
    os.chown(path, 65534, 65534)
    path.chmod(0o600)
PY
cli stop "$name"
cli backup create "$name"
manifest=$(find "$base/backups/$name" -name manifest.json -print -quit)
id=$(basename "$(dirname "$manifest")")
python3 - "$manifest" "$name" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
volumes = [c for c in json.loads(path.read_text())['components'] if c['type'] == 'volume']
assert len(volumes) == 4, volumes
named = {c['name'] for c in volumes if 'named volume' in c['name']}
assert named == {sys.argv[2]+suffix+' (named volume)' for suffix in ['_data', '-custom', '-external']}, named
assert sum(c.get('restore_path') == 'tmp' for c in volumes) == 1
for component in volumes:
    assert len(component['checksum']) == 64
    assert (path.parent / component['path']).stat().st_mode & 0o777 == 0o600
PY
cli backup verify "$name" "$id"
cli start "$name"
compose exec -T app python3 - <<'PY'
from pathlib import Path
for root in ['/data', '/custom', '/external', '/state']:
    (Path(root) / 'proof').write_text('after')
    (Path(root) / 'stale').write_text('remove on restore')
PY
app=$(compose ps -q app)
assert_running() { test "$(docker inspect --format '{{.State.Running}}' "$app")" = true; }
# A required missing volume must fail, without Docker silently creating it.
python3 - "$base/project/docker-compose.yml" "$name" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
text = path.read_text().replace('    volumes:\n', '    volumes:\n      - missing:/missing\n', 1)
text = text.replace('volumes:\n  data:', 'volumes:\n  missing:\n    name: '+sys.argv[2]+'-missing\n  data:')
path.write_text(text)
PY
if cli backup create "$name"; then echo 'Missing volume accepted' >&2; exit 1; fi
if docker volume inspect "$name-missing" >/dev/null 2>&1; then echo 'Missing volume was created' >&2; exit 1; fi
cp "$base/compose.original" "$base/project/docker-compose.yml"
printf 'DATA_PATH=./missing\n' > "$base/project/.env"
if cli backup create "$name"; then echo 'Missing bind accepted' >&2; exit 1; fi
printf 'DATA_PATH=./tmp\n' > "$base/project/.env"
assert_running
test "$(find "$base/backups/$name" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')" = 1
# A different valid tar.gz must fail its recorded checksum before stopping apps.
archive=$(python3 -c 'import json,pathlib,sys;p=pathlib.Path(sys.argv[1]);print(p.parent/next(c["path"] for c in json.loads(p.read_text())["components"] if c["type"]=="volume"))' "$manifest")
cp "$archive" "$base/original.tar.gz"
python3 - "$archive" <<'PY'
import io, sys, tarfile
with tarfile.open(sys.argv[1], 'w:gz') as archive:
    entry = tarfile.TarInfo('proof')
    entry.size = 7
    archive.addfile(entry, io.BytesIO(b'changed'))
PY
if cli rollback "$name" --backup-id "$id" --no-snapshot; then echo 'Replaced archive accepted' >&2; exit 1; fi
assert_running
mv "$base/original.tar.gz" "$archive"
cli rollback "$name" --backup-id "$id" --no-snapshot
compose exec -T --user 65534 app python3 - <<'PY'
from pathlib import Path
for root in ['/data', '/custom', '/external', '/state']:
    assert (Path(root) / 'proof').read_text() == 'before', root
    assert not (Path(root) / 'stale').exists(), root
PY
printf 'Resolved volume names, deduplication, missing-data refusal, archive digests and actual restore passed.\n'
