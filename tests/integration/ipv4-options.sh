#!/bin/sh
# Benign packet tests. No exploit traffic is generated.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
if [ "${1:-}" != --inside ]; then exec unshare --net --mount "$0" --inside; fi
mount --make-rprivate /
mkdir -p /run/netns
mount -t tmpfs tmpfs /run/netns
cleanup() {
    test -z "${server:-}" || kill "$server" 2>/dev/null || true
    ip netns pids outside 2>/dev/null | xargs -r kill 2>/dev/null || true
    ip netns del outside 2>/dev/null || true
}
trap cleanup EXIT INT TERM
ip link set lo up
ip netns add outside
ip link add public0 type veth peer name eth0 netns outside
ip link set public0 up
ip addr add 192.0.2.1/24 dev public0
ip -6 addr add 2001:db8::1/64 dev public0 nodad
ip netns exec outside ip link set lo up
ip netns exec outside ip link set eth0 up
ip netns exec outside ip addr add 192.0.2.2/24 dev eth0
ip netns exec outside ip -6 addr add 2001:db8::2/64 dev eth0 nodad
python3 -m http.server 18080 --bind :: >/dev/null 2>&1 &
server=$!
probe() {
    ip netns exec outside python3 - "$1" "$2" "$3" <<'PY'
import socket, sys
address, options, expected = sys.argv[1:]
s = socket.socket(socket.AF_INET6 if ':' in address else socket.AF_INET, socket.SOCK_STREAM)
s.settimeout(2)
if options == 'options':
    # Four harmless IPv4 NOP options exercise IHL > 5 without an exploit.
    s.setsockopt(socket.IPPROTO_IP, socket.IP_OPTIONS, bytes([1, 1, 1, 1]))
try:
    s.connect((address, 18080))
    s.sendall(b'GET / HTTP/1.0\r\n\r\n')
    ok = s.recv(16).startswith(b'HTTP/')
except OSError:
    ok = False
finally:
    s.close()
if ok != (expected == 'allow'):
    raise SystemExit(f'FAIL {address} {options}: expected {expected}, reachable={ok}')
print(f'PASS {address} {options} {expected}')
PY
}
i=0
until probe 192.0.2.1 ordinary allow >/dev/null 2>&1; do
    i=$((i+1)); test "$i" -lt 30; sleep 0.1
done
probe 192.0.2.1 options allow
probe 2001:db8::1 ordinary allow
nft -f "$ROOT/examples/security/ipv4-options.nft"
nft -f "$ROOT/examples/security/ipv4-options.nft"
probe 192.0.2.1 options deny
probe 192.0.2.1 ordinary allow
probe 2001:db8::1 ordinary allow
# Simulate rollback: ordinary traffic and the previously filtered option packet
# must both work again. Deleting only the owned table preserves all other policy.
nft delete table inet fleetdeck_packet_guard
probe 192.0.2.1 options allow
probe 192.0.2.1 ordinary allow
probe 2001:db8::1 ordinary allow
echo 'IPv4 option guard and rollback packet tests passed.'
