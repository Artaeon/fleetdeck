#!/bin/sh
# Root-only disposable CLI/proxy smoke test. Never runs server setup or changes
# host firewall rules. Docker restart is opt-in and requires an otherwise idle daemon.
set -eu
: "${FLEETDECK_BINARY:?set FLEETDECK_BINARY to the tested executable}"
[ "$(id -u)" = 0 ] || { echo 'run as root on a disposable test host'; exit 1; }
[ -z "$(docker ps -q --filter label=io.fleetdeck.proxy=true)" ] || { echo 'existing FleetDeck proxy; use a test daemon'; exit 1; }
base=$(mktemp -d /var/tmp/fleetdeck-cli.XXXXXXXX)
chmod 0755 "$base"
a=fi-a-$$
b=fi-b-$$
proxy=fi-proxy-$$
config=$base/config.toml
cat > "$config" <<CONFIG
[server]
base_path = "$base/projects"
[backup]
auto_snapshot = false
[audit]
enabled = false
CONFIG
mkdir -m 0755 "$base/projects"
fd() { "$FLEETDECK_BINARY" --config "$config" "$@"; }
cleanup() {
    docker rm -f "$proxy" >/dev/null 2>&1 || true
    for name in "$a" "$b"; do
        fd destroy "$name" --force --keep-repo >/dev/null 2>&1 || true
        docker network rm "fleetdeck-$name-edge" >/dev/null 2>&1 || true
    done
    rm -rf "$base"
}
trap cleanup EXIT INT TERM
proxy_start() {
    docker run -d --name "$proxy" --restart unless-stopped \
      --label io.fleetdeck.proxy=true -p 127.0.0.1:18443:443 \
      -v /var/run/docker.sock:/var/run/docker.sock:ro \
      traefik@sha256:30ff62491aacdb862ceb8c9edcf2627da638d7deafdd4b6dab32fe34f8aa3e90 \
      --api=false --providers.docker=true --providers.docker.exposedbydefault=false \
      --entrypoints.websecure.address=:443 >/dev/null
}
proxy_start
for name in "$a" "$b"; do
    fd create "$name" --domain "$name.example.test" --template static --profile static --skip-github
    # Offline test certificate only: do not request certificates for example.test.
    sed -i '/tls.certresolver=letsencrypt/d' "$base/projects/$name/docker-compose.yml"
    sed -i 's|image: nginx:alpine|image: nginx@sha256:a3554c0d0b48177b1413c26e01f695334428020d9df01243fbae3e11cfd17417|' "$base/projects/$name/docker-compose.yml"
    [ "$(stat -c %a "$base/projects/$name")" = 750 ]
    [ "$(getent passwd "fleetdeck-$name" | cut -d: -f7)" = /usr/sbin/nologin ]
    if id -nG "fleetdeck-$name" | tr ' ' '\n' | grep -qx docker; then exit 1; fi
    runuser -u "fleetdeck-$name" -- test -r "$base/projects/$name/public/index.html"
    if runuser -u "fleetdeck-$name" -- docker ps >/dev/null 2>&1; then echo 'project can control Docker'; exit 1; fi
    fd start "$name"
done
if runuser -u "fleetdeck-$a" -- test -r "$base/projects/$b/public/index.html"; then echo 'cross-project file access'; exit 1; fi
check_routes() {
    for name in "$a" "$b"; do
        ready=false
        for _attempt in $(seq 1 30); do
            if curl --noproxy '*' -skf --connect-timeout 2 --max-time 3 \
                --resolve "$name.example.test:18443:127.0.0.1" \
                "https://$name.example.test:18443/" | grep -q "Welcome to $name"; then
                ready=true; break
            fi
            sleep 1
        done
        [ "$ready" = true ] || { docker logs "$proxy"; exit 1; }
        docker exec "$name-nginx" sh -c 'grep -Eq "^NoNewPrivs:[[:space:]]+1$" /proc/1/status'
    done
}
check_routes
# Recreate (not merely restart) the proxy and exercise the recovery command.
docker rm -f "$proxy" >/dev/null
proxy_start
fd server reconnect-proxy
check_routes
if [ "${FLEETDECK_TEST_DOCKER_RESTART:-false}" = true ]; then
    # Refuse to restart Docker if any unrelated container exists.
    for name in $(docker ps -a --format '{{.Names}}'); do
        case "$name" in "$proxy"|"$a-nginx"|"$b-nginx") ;; *) echo 'unrelated container; refusing Docker restart'; exit 1 ;; esac
    done
    systemctl restart docker
    systemctl is-active --quiet fleetdeck-firewall
    nft list table inet fleetdeck >/dev/null
    check_routes
fi
echo 'CLI isolation passed: accounts, files, Docker denial, TLS routes, runtime privileges, proxy recreation.'
