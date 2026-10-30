#!/bin/sh
# Dedicated disposable Linux/Docker runner only: refuses unrelated containers.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
: "${FLEETDECK_TRANSPORT_FIXTURE:?build tests/integration/transport_fixture.go first}"
test "$(id -u)" = 0
test -z "$(docker ps -q)" || { echo 'Requires an otherwise empty disposable Docker host'; exit 1; }
state=$(mktemp -d /var/tmp/fleetdeck-transport.XXXXXX)
name=fd-transport-$$
watch_pid=
rollout=
cleanup() {
    result=$?
    if test "$result" -ne 0 && test -f "$state/watcher.log"; then cat "$state/watcher.log"; fi
    if test -n "$watch_pid"; then kill "$watch_pid" 2>/dev/null || true; wait "$watch_pid" 2>/dev/null || true; fi
    if test -n "$rollout"; then python3 "$ROOT/deploy/transport/install.py" rollback --id "$rollout" || true; fi
    docker rm -f "$name-server" "$name-peer" "$name-new" "$name-unsafe" >/dev/null 2>&1 || true
    docker network rm "$name" >/dev/null 2>&1 || true
    rm -rf "$state"
}
trap cleanup EXIT INT TERM
docker network create "$name" >/dev/null
docker run -d --name "$name-server" --network "$name" --cap-drop ALL --security-opt no-new-privileges \
    -v "$FLEETDECK_TRANSPORT_FIXTURE:/fixture:ro" --entrypoint /fixture alpine:3.21 >/dev/null
docker run -d --name "$name-peer" --network "$name" --cap-drop ALL --security-opt no-new-privileges alpine:3.21 sleep 300 >/dev/null
pid=$(docker inspect --format '{{.State.Pid}}' "$name-server")
started=$(docker inspect --format '{{.State.StartedAt}}' "$name-server")
old_mptcp=$(nsenter -t "$pid" -n sysctl -n net.mptcp.enabled)
probe() {
    nsenter -t "$pid" -n -- python3 - "$1" "$2" <<'PY'
import socket, sys
options, expected = sys.argv[1:]
s=socket.socket();s.settimeout(1)
if options=='options':s.setsockopt(socket.IPPROTO_IP,socket.IP_OPTIONS,bytes([1,1,1,1]))
try:
 s.connect(('127.0.0.1',18080));s.sendall(b'GET / HTTP/1.0\r\n\r\n');ok=s.recv(16).startswith(b'HTTP/')
except OSError:ok=False
finally:s.close()
assert ok==(expected=='allow'),(options,expected,ok)
PY
}
dns() {
    nsenter -t "$pid" -n -- python3 - "$name-peer" <<'PY'
import socket, struct, sys
name=b''.join(bytes([len(x)])+x.encode() for x in sys.argv[1].split('.'))+b'\0'
query=struct.pack('!HHHHHH',1234,0x100,1,0,0,0)+name+struct.pack('!HH',1,1)
for kind in [socket.SOCK_DGRAM,socket.SOCK_STREAM]:
 with socket.socket(socket.AF_INET,kind) as s:
  s.settimeout(2);s.connect(('127.0.0.11',53))
  if kind==socket.SOCK_DGRAM:s.send(query);response=s.recv(4096)
  else:
   s.sendall(struct.pack('!H',len(query))+query)
   def exact(n):
    b=b''
    while len(b)<n:
     chunk=s.recv(n-len(b));assert chunk;b+=chunk
    return b
   response=exact(struct.unpack('!H',exact(2))[0])
  header=struct.unpack('!HHHHHH',response[:12]);assert header[0]==1234 and header[1]&15==0 and header[3]>0
print('Docker embedded DNS: UDP and TCP passed')
PY
}
i=0
until probe ordinary allow; do i=$((i+1));test "$i" -lt 20;sleep 0.1;done
probe options allow
dns
python3 "$ROOT/deploy/transport/namespace_guard.py" apply --state-dir "$state"
probe options deny
probe ordinary allow
dns
test "$(nsenter -t "$pid" -n sysctl -n net.mptcp.enabled)" = 0
# Reapplying must retain the original rollback snapshot, not overwrite it with 0.
python3 "$ROOT/deploy/transport/namespace_guard.py" apply --state-dir "$state"
python3 "$ROOT/deploy/transport/namespace_guard.py" rollback --state-dir "$state"
test "$(nsenter -t "$pid" -n sysctl -n net.mptcp.enabled)" = "$old_mptcp"
probe options allow
probe ordinary allow
dns
# The watcher repairs a removed owned table and covers a subsequent namespace.
python3 "$ROOT/deploy/transport/namespace_guard.py" watch --state-dir "$state" >"$state/watcher.log" 2>&1 &
watch_pid=$!
i=0
until nsenter -t "$pid" -n nft list table inet fleetdeck_transport_guard >/dev/null 2>&1; do
 i=$((i+1));test "$i" -lt 100;sleep 0.1
done
nsenter -t "$pid" -n nft delete table inet fleetdeck_transport_guard
docker run -d --name "$name-new" --network "$name" --cap-drop ALL alpine:3.21 sleep 300 >/dev/null
new_pid=$(docker inspect --format '{{.State.Pid}}' "$name-new")
i=0
until nsenter -t "$new_pid" -n nft list table inet fleetdeck_transport_guard >/dev/null 2>&1; do
 i=$((i+1));test "$i" -lt 100;sleep 0.1
done
probe options deny
probe ordinary allow
dns
kill "$watch_pid"; wait "$watch_pid" 2>/dev/null || true;watch_pid=
python3 "$ROOT/deploy/transport/namespace_guard.py" rollback --state-dir "$state"
probe options allow
test "$started" = "$(docker inspect --format '{{.State.StartedAt}}' "$name-server")"
test "$pid" = "$(docker inspect --format '{{.State.Pid}}' "$name-server")"
docker run -d --name "$name-unsafe" --network "$name" --cap-add NET_ADMIN alpine:3.21 sleep 300 >/dev/null
if python3 "$ROOT/deploy/transport/namespace_guard.py" apply --state-dir "$state" >"$state/refusal.log" 2>&1; then
 echo 'Unsafe privilege policy incorrectly accepted';exit 1
fi
# A newly introduced unsafe container must not prevent restoring saved policy
# in the original namespaces. Its own namespace has no snapshot and is skipped.
python3 "$ROOT/deploy/transport/namespace_guard.py" rollback --state-dir "$state"
probe options allow
echo 'Namespace packet policy, DNS, idempotence, watcher, rollback and unchanged application process: PASS'
docker rm -f "$name-unsafe" >/dev/null
# The production installer uses a separate systemd timer, not this shell's lifetime.
docker_pid=$(systemctl show -p MainPID --value docker.service)
rollout=fixture-expiry-$$
python3 "$ROOT/deploy/transport/install.py" apply --id "$rollout" --timeout 15
probe options deny
i=0
until python3 - "$rollout" <<'PY'
import json,pathlib,sys
p=pathlib.Path('/var/lib/fleetdeck/transport-rollouts')/sys.argv[1]/'state.json'
raise SystemExit(0 if json.loads(p.read_text())['phase']=='rolled-back' else 1)
PY
do i=$((i+1));test "$i" -lt 60;sleep 1;done
probe options allow
if python3 "$ROOT/deploy/transport/install.py" confirm --id "$rollout" >"$state/expired.log" 2>&1; then
 echo 'Expired rollout was incorrectly confirmed';exit 1
fi
rollout=fixture-confirm-$$
python3 "$ROOT/deploy/transport/install.py" apply --id "$rollout" --timeout 60
python3 "$ROOT/deploy/transport/install.py" confirm --id "$rollout"
python3 "$ROOT/deploy/transport/install.py" rollback --id "$rollout" --automatic
probe options deny
python3 "$ROOT/deploy/transport/install.py" rollback --id "$rollout"
rollout=
probe options allow
dns
test "$docker_pid" != 0
test "$docker_pid" = "$(systemctl show -p MainPID --value docker.service)"
test "$started" = "$(docker inspect --format '{{.State.StartedAt}}' "$name-server")"
test "$pid" = "$(docker inspect --format '{{.State.Pid}}' "$name-server")"
echo 'Installer timer expiry, expired confirmation refusal, stale callback, manual rollback and unchanged Docker: PASS'
