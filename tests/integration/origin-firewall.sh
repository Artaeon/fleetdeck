#!/bin/sh
# Root/Linux only. Pass an nft file rendered with --interface wan.
# Everything runs in disposable network and mount namespaces.
set -eu
if [ "${1:-}" != --inside ]; then
 rules=$(realpath "$1")
 exec unshare --net --mount "$0" --inside "$rules"
fi
rules=$2
mount --make-rprivate /
mkdir -p /run/netns
mount -t tmpfs tmpfs /run/netns
cleanup() {
 kill "${listener:-}" 2>/dev/null || true
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
for ns in outside app; do
 ip netns exec "$ns" ip link set lo up
 ip netns exec "$ns" ip link set eth0 up
done
ip netns exec outside ip addr add 192.0.2.2/24 dev eth0
ip netns exec outside ip -6 addr add 2001:db8:1::2/64 dev eth0 nodad
ip netns exec outside ip route add default via 192.0.2.1
ip netns exec outside ip -6 route add default via 2001:db8:1::1
ip netns exec app ip addr add 198.51.100.2/24 dev eth0
ip netns exec app ip -6 addr add 2001:db8:2::2/64 dev eth0 nodad
ip netns exec app ip route add default via 198.51.100.1
ip netns exec app ip -6 route add default via 2001:db8:2::1
sysctl -qw net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1
# Host HTTP, HTTPS and SSH-shaped listener. No existing server is touched.
python3 - <<'PY' >/dev/null 2>&1 &
import http.server,threading,socket,time
class Server(http.server.ThreadingHTTPServer): address_family=socket.AF_INET6
for port in [22,80,443]:
 s=Server(('::',port),http.server.SimpleHTTPRequestHandler)
 threading.Thread(target=s.serve_forever,daemon=True).start()
time.sleep(300)
PY
listener=$!
ip netns exec app python3 -m http.server 8080 --bind :: >/dev/null 2>&1 &
probe() {
 ip netns exec outside python3 - "$1" "$2" "$3" <<'PY'
import socket,sys
address,port,expected=sys.argv[1:]
try:
 with socket.create_connection((address,int(port)),timeout=1) as s:
  s.sendall(b'GET / HTTP/1.0\r\n\r\n');ok=s.recv(16).startswith(b'HTTP/')
except OSError:ok=False
if ok!=(expected=='allow'):raise SystemExit(f'{address}:{port}: expected {expected}, reachable={ok}')
print(f'PASS {address}:{port} {expected}')
PY
}
for address in 192.0.2.1 2001:db8:1::1; do
 i=0; until probe "$address" 80 allow >/dev/null 2>&1; do
  i=$((i+1)); test "$i" -lt 20; sleep .1
 done
done
nft -f "$rules"
nft -f "$rules"
for address in 192.0.2.1 2001:db8:1::1; do
 probe "$address" 80 deny
 probe "$address" 443 deny
 probe "$address" 22 allow
done
# Add synthetic provider membership ONLY inside this disposable namespace.
nft add element inet fleetdeck_origin cloudflare4 '{ 192.0.2.0/24 }'
nft add element inet fleetdeck_origin cloudflare6 '{ 2001:db8:1::/64 }'
for address in 192.0.2.1 2001:db8:1::1; do
 probe "$address" 80 allow
 probe "$address" 443 allow
done
nft -f - <<'RULES'
table inet fixture_nat {
 chain prerouting {
  type nat hook prerouting priority dstnat; policy accept;
  iifname "wan" ip daddr 192.0.2.1 tcp dport {80,443} dnat ip to 198.51.100.2:8080
  iifname "wan" ip6 daddr 2001:db8:1::1 tcp dport {80,443} dnat ip6 to [2001:db8:2::2]:8080
 }
}
RULES
for address in 192.0.2.1 2001:db8:1::1; do
 probe "$address" 80 allow
 probe "$address" 443 allow
done
nft -f "$rules"
for address in 192.0.2.1 2001:db8:1::1; do
 probe "$address" 80 deny
 probe "$address" 443 deny
 probe "$address" 22 allow
done
echo 'Origin host/DNAT tests passed for both IP families; SSH preserved and reload closed old provider access.'
