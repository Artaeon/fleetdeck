#!/usr/bin/env bash
# Only task-owned containers with volatile databases; no host ports or volumes.
set -euo pipefail
image=${1:?Usage: postgres-nonroot.sh IMAGE [EXPECTED_MAJOR]}
expected_major=${2:-17}
base=$(mktemp -d)
name="fleetdeck-pg-nonroot-$$"
cleanup() {
  docker rm -f -v "$name-source" "$name-restore" >/dev/null 2>&1 || true
  rm -rf "$base"
}
trap cleanup EXIT
for suffix in source restore; do
  docker run -d --name "$name-$suffix" --network none --read-only \
    --cap-drop ALL --security-opt no-new-privileges:true \
    --memory 256m --pids-limit 96 \
    --tmpfs /var/lib/postgresql/data:rw,nosuid,uid=70,gid=70,mode=0700 \
    --tmpfs /var/run/postgresql:rw,nosuid,uid=70,gid=70,mode=0750 \
    --tmpfs /tmp:rw,nosuid,noexec,size=16m \
    -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=fixture \
    -e POSTGRES_DB=fixture "$image" >/dev/null
  for attempt in $(seq 1 60); do
    # initdb briefly starts a temporary server; wait for the final PID 1 too.
    if [ "$(docker exec "$name-$suffix" cat /proc/1/comm 2>/dev/null || true)" = postgres ] &&
       docker exec "$name-$suffix" pg_isready -U fixture -d fixture >/dev/null 2>&1; then break; fi
    if [ "$attempt" = 60 ]; then echo 'Database did not become ready' >&2; exit 1; fi
    sleep 1
  done
  test "$(docker exec "$name-$suffix" id -u)" = 70
  version=$(docker exec "$name-$suffix" psql -X -At -U fixture -d fixture -c 'SHOW server_version_num')
  test "$((version / 10000))" -eq "$expected_major"
  docker exec "$name-$suffix" sh -c 'test ! -e /usr/local/bin/gosu'
done
docker exec "$name-source" psql -X -v ON_ERROR_STOP=1 -U fixture -d fixture \
  -c "CREATE TABLE proof(value text); INSERT INTO proof VALUES ('restored');" >/dev/null
docker exec "$name-source" pg_dump -U fixture -d fixture > "$base/database.sql"
docker exec -i "$name-restore" psql -X -v ON_ERROR_STOP=1 -U fixture -d fixture \
  < "$base/database.sql" >/dev/null
test "$(docker exec "$name-restore" psql -X -At -U fixture -d fixture -c 'SELECT value FROM proof')" = restored
echo 'Non-root startup, removed helper and isolated database restore passed.'
