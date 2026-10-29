#!/bin/sh
# Disposable server-side source deployment and recovery test. No host setup,
# firewall changes or real project data. Docker bind paths must exist on daemon host.
set -eu
umask 077
: "${FLEETDECK_BINARY:?provide the tested Linux binary}"
[ "$(id -u)" = 0 ] || exit 1
base=$(mktemp -d "${FLEETDECK_TEST_ROOT:-/var/tmp}/fleetdeck-source.XXXXXXXX")
name=fd-source-$$
config=$base/config.toml
mkdir -m 0750 "$base/projects" "$base/source"
cat > "$config" <<CONFIG
[server]
base_path = "$base/projects"
[backup]
base_path = "$base/backups"
[audit]
enabled = false
CONFIG
fd() { "$FLEETDECK_BINARY" --config "$config" "$@"; }
compose() { docker compose --project-name "$name" --file "$base/projects/$name/compose.yaml" "$@"; }
cleanup() {
    result=$?
    if [ "$result" != 0 ] && [ -f "$base/projects/$name/compose.yaml" ]; then
        compose ps -a || true
        compose logs --tail 20 || true
    fi
    if [ -f "$base/projects/$name/compose.yaml" ]; then compose down -v --remove-orphans >/dev/null 2>&1 || true; fi
    docker image ls --format '{{.Repository}}:{{.Tag}}' "fleetdeck-rollback/$name" | xargs -r docker image rm >/dev/null 2>&1 || true
    docker image rm "$name-app" >/dev/null 2>&1 || true
    userdel "fleetdeck-$name" >/dev/null 2>&1 || true
    rm -rf "$base"
}
trap cleanup EXIT INT TERM
cat > "$base/source/compose.yaml" <<'COMPOSE'
services:
  app:
    build: .
    env_file: .env
    security_opt: ['no-new-privileges:true']
    cap_drop: [ALL]
    volumes: [uploads:/uploads]
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:?required}
      POSTGRES_USER: fixture
      POSTGRES_DB: fixture
    security_opt: ['no-new-privileges:true']
    cap_drop: [NET_RAW]
    volumes: [database:/var/lib/postgresql/data]
    healthcheck:
      test: [CMD-SHELL, 'pg_isready -U fixture -d fixture']
      interval: 2s
      timeout: 2s
      retries: 30
volumes:
  uploads: {}
  database: {}
COMPOSE
cat > "$base/source/Dockerfile" <<'DOCKERFILE'
FROM alpine:3.21
WORKDIR /app
COPY . .
RUN test ! -f .env
USER 10001
HEALTHCHECK --interval=2s --timeout=2s --retries=10 CMD test -f /app/release.txt
CMD ["sh", "-c", "while :; do sleep 60; done"]
DOCKERFILE
printf 'POSTGRES_PASSWORD=test-fixture-only\nSECRET=server-fixture\n' > "$base/source/.env"
printf 'version-one\n' > "$base/source/release.txt"
chmod 0644 "$base/source/compose.yaml" "$base/source/Dockerfile" "$base/source/release.txt"
pack() { tar -C "$base/source" -cf "$base/source.tar" compose.yaml Dockerfile release.txt .env; }
receive() { fd deploy receive --name "$name" --domain "$name.example.test" --timeout 8m < "$base/source.tar"; }
pack
receive
[ "$(getent passwd "fleetdeck-$name" | cut -d: -f7)" = /usr/sbin/nologin ]
! id -nG "fleetdeck-$name" | tr ' ' '\n' | grep -qx docker
[ "$(stat -c %a "$base/projects/$name/.env")" = 600 ]
[ "$(compose exec -T app cat /app/release.txt)" = version-one ]
compose exec -T app sh -c 'test ! -f /app/.env'
# A direct receiver invocation must not trust a client's claimed scan. Keep the
# synthetic credential out of shell logs and verify no existing workload changes.
python3 - "$base/source/release.txt" <<'PY'
import hashlib,pathlib,sys
pathlib.Path(sys.argv[1]).write_text('ghp_'+hashlib.sha256(b'fleetdeck synthetic receiver test').hexdigest()[:36])
PY
pack
if receive > "$base/secret-rejection.log" 2>&1; then echo 'receiver accepted a potential secret'; exit 1; fi
grep -q 'potential secrets detected' "$base/secret-rejection.log"
[ "$(compose exec -T app cat /app/release.txt)" = version-one ]
python3 - "$base/source/release.txt" "$base/secret-rejection.log" <<'PY'
import pathlib,sys
assert pathlib.Path(sys.argv[1]).read_text() not in pathlib.Path(sys.argv[2]).read_text()
PY
printf 'version-one\n' > "$base/source/release.txt"
compose exec -T db psql -U fixture -d fixture -c "CREATE TABLE recovery_probe (value text); INSERT INTO recovery_probe VALUES ('before');" >/dev/null
volume=$(docker volume ls -q --filter "label=com.docker.compose.project=$name" --filter label=com.docker.compose.volume=uploads)
docker run --rm --network none -v "$volume:/data" alpine:3.21 sh -c 'echo before > /data/marker'
printf 'POSTGRES_PASSWORD=wrong-local-value\nSECRET=local-fixture\n' > "$base/source/.env"
printf 'version-two\n' > "$base/source/release.txt"
pack
receive
[ "$(compose exec -T app printenv SECRET)" = server-fixture ]
[ "$(compose exec -T app cat /app/release.txt)" = version-two ]
[ "$(compose exec -T app cat /uploads/marker)" = before ]
backup=$(python3 - "$base/projects/fleetdeck.db" <<'PY'
import sqlite3,sys
with sqlite3.connect(sys.argv[1]) as db:
 print(db.execute("select id from backups order by created_at desc limit 1").fetchone()[0])
PY
)
backup_path=$base/backups/$name/$backup
fd backup verify "$name" "$backup"
[ "$(cat "$backup_path/config/release.txt")" = version-one ]
compose exec -T db psql -U fixture -d fixture -c "INSERT INTO recovery_probe VALUES ('after');" >/dev/null
# Test data restoration deliberately only in this disposable fixture.
fd backup restore "$name" "$backup" --force --no-start
compose -f "$backup_path/runtime-images.json" up -d --no-build --pull never --wait --wait-timeout 90
[ "$(compose exec -T app cat /app/release.txt)" = version-one ]
[ "$(compose exec -T db psql -U fixture -d fixture -Atc 'select count(*) from recovery_probe')" = 1 ]
# An unavailable database must block the next update and preserve old source.
compose stop db >/dev/null
if receive > "$base/failed-receive.log" 2>&1; then echo 'deployment bypassed failed database backup'; exit 1; fi
[ "$(cat "$base/projects/$name/release.txt")" = version-one ]
echo 'Source deployment passed: secret rejection, account, first seed, secret preservation, verified backup, data recovery, old-image rollback, failed-backup refusal.'
