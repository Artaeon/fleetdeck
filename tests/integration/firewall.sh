#!/bin/sh
# Run as root on Linux: tests/integration/firewall.sh
# All interfaces and firewall tables live in disposable network/mount namespaces.
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
if [ "${1:-}" != --inside ]; then
    exec unshare --net --mount "$0" --inside
fi
mount --make-rprivate /
mkdir -p /run/netns
mount -t tmpfs tmpfs /run/netns
cleanup() {
    if [ -n "${host_pid:-}" ]; then kill "$host_pid" 2>/dev/null || true; fi
    for ns in outside app; do
        ip netns pids "$ns" 2>/dev/null | xargs -r kill 2>/dev/null || true
        ip netns del "$ns" 2>/dev/null || true
    done
}
trap cleanup EXIT INT TERM
ip link set lo up
ip netns add outside
ip netns add app
ip link add wan type veth peer name eth0 netns outside
ip link add br-fixture type veth peer name eth0 netns app
ip addr add 192.0.2.1/24 dev wan
ip -6 addr add 2001:db8:1::1/64 dev wan nodad
ip addr add 198.51.100.1/24 dev br-fixture
ip -6 addr add 2001:db8:2::1/64 dev br-fixture nodad
ip link set wan up
ip link set br-fixture up
ip netns exec outside ip link set lo up
ip netns exec outside ip link set eth0 up
ip netns exec outside ip addr add 192.0.2.2/24 dev eth0
ip netns exec outside ip -6 addr add 2001:db8:1::2/64 dev eth0 nodad
ip netns exec outside ip route add default via 192.0.2.1
ip netns exec outside ip -6 route add default via 2001:db8:1::1
ip netns exec app ip link set lo up
ip netns exec app ip link set eth0 up
ip netns exec app ip addr add 198.51.100.2/24 dev eth0
ip netns exec app ip -6 addr add 2001:db8:2::2/64 dev eth0 nodad
ip netns exec app ip route add default via 198.51.100.1
ip netns exec app ip -6 route add default via 2001:db8:2::1
sysctl -qw net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1
nft -f - <<'RULES'
table inet fixture_nat {
 chain prerouting {
  type nat hook prerouting priority dstnat; policy accept;
  iifname "wan" ip daddr 192.0.2.1 tcp dport {80,443,18080} dnat ip to 198.51.100.2:8080
  iifname "wan" ip6 daddr 2001:db8:1::1 tcp dport {80,443,18080} dnat ip6 to [2001:db8:2::2]:8080
 }
}
# Model Docker's permissive forwarding chain AFTER the FleetDeck filter.
table inet fixture_docker {
 chain forward { type filter hook forward priority 0; policy accept; }
}
RULES
ip netns exec app python3 -m http.server 8080 --bind :: >/dev/null 2>&1 &
ip netns exec outside python3 -m http.server 8081 --bind :: >/dev/null 2>&1 &
# Host input must remain unaffected (representing SSH / a native reverse proxy).
python3 -m http.server 18081 --bind :: >/dev/null 2>&1 &
host_pid=$!
probe() {
    ip netns exec "$1" python3 - "$2" "$3" "$4" <<'PY'
import socket, sys
address, port, expected = sys.argv[1:]
try:
    with socket.create_connection((address, int(port)), timeout=1) as s:
        s.sendall(b'GET / HTTP/1.0\r\n\r\n')
        ok = s.recv(16).startswith(b'HTTP/')
except OSError:
    ok = False
if ok != (expected == 'allow'):
    raise SystemExit(f'FAIL {address}:{port}: expected {expected}, reachable={ok}')
print(f'PASS {address}:{port} {expected}')
PY
}
# Bounded readiness check before enforcing rules; verify the forbidden port was reachable.
i=0
until probe outside 192.0.2.1 18080 allow >/dev/null 2>&1; do
    i=$((i+1)); [ "$i" -lt 20 ] || exit 1; sleep 0.1
done
nft -f "$ROOT/internal/bootstrap/docker-firewall.nft"
nft -f "$ROOT/internal/bootstrap/docker-firewall.nft" # repeat must remain idempotent
for address in 192.0.2.1 2001:db8:1::1; do
    probe outside "$address" 80 allow
    probe outside "$address" 443 allow
    probe outside "$address" 18080 deny
    probe outside "$address" 18081 allow
done
probe outside 198.51.100.2 8080 deny
probe outside 2001:db8:2::2 8080 deny
probe app 192.0.2.2 8081 allow
probe app 2001:db8:1::2 8081 allow
# An application on another Docker bridge must not use the host's published
# private port to bypass the project boundary. HTTP remains publicly routable.
ip link set wan name br-other
nft -f - <<'RULES'
add rule inet fixture_nat prerouting iifname "br-other" ip daddr 192.0.2.1 tcp dport {80,18080} dnat ip to 198.51.100.2:8080
add rule inet fixture_nat prerouting iifname "br-other" ip6 daddr 2001:db8:1::1 tcp dport {80,18080} dnat ip6 to [2001:db8:2::2]:8080
RULES
for address in 192.0.2.1 2001:db8:1::1; do
    probe outside "$address" 18080 deny
    probe outside "$address" 80 allow
done
kill "$host_pid"
echo 'Firewall packet tests passed (IPv4, IPv6, DNAT, direct routing, host input, egress, reload).'
