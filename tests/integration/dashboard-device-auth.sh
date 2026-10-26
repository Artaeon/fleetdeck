#!/usr/bin/env bash
# Real CLI/TLS drill with ephemeral identities and an empty project database.
set -euo pipefail
binary=${FLEETDECK_BINARY:?Set FLEETDECK_BINARY to a built CLI}
base=$(mktemp -d)
pid=''
cleanup() {
  if [ -n "$pid" ]; then kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; fi
  rm -rf "$base"
}
trap cleanup EXIT
umask 077
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 1 \
  -subj /CN=disposable-test-ca -keyout "$base/ca.key" -out "$base/ca.pem" >/dev/null 2>&1
for name in server device-a device-b unlisted; do
  openssl req -new -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes \
    -subj "/CN=$name" -keyout "$base/$name.key" -out "$base/$name.csr" >/dev/null 2>&1
  if [ "$name" = server ]; then
    printf 'extendedKeyUsage=serverAuth\nsubjectAltName=IP:127.0.0.1\n' > "$base/extensions"
  else
    printf 'extendedKeyUsage=clientAuth\n' > "$base/extensions"
  fi
  openssl x509 -req -in "$base/$name.csr" -CA "$base/ca.pem" -CAkey "$base/ca.key" \
    -CAcreateserial -days 1 -extfile "$base/extensions" -out "$base/$name.pem" >/dev/null 2>&1
done
cat > "$base/config.toml" <<CONFIG
[server]
base_path = "$base/projects"
[audit]
enabled = false
CONFIG
export FLEETDECK_API_TOKEN FLEETDECK_METRICS_TOKEN
FLEETDECK_API_TOKEN=$(openssl rand -hex 32)
FLEETDECK_METRICS_TOKEN=$(openssl rand -hex 32)
if "$binary" --config "$base/config.toml" dashboard --addr 0.0.0.0:0 > "$base/refused.log" 2>&1; then
  echo 'Remote HTTP listener accepted' >&2; exit 1
fi
port=$(python3 - <<'PY'
import socket
with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    print(sock.getsockname()[1])
PY
)
"$binary" --config "$base/config.toml" dashboard --addr "127.0.0.1:$port" \
  --tls-cert "$base/server.pem" --tls-key "$base/server.key" --client-ca "$base/ca.pem" \
  --allow-client-cert "$base/device-a.pem" --allow-client-cert "$base/device-b.pem" > "$base/server.log" 2>&1 &
pid=$!
python3 - "$base" "$port" <<'PY'
import http.client, os, ssl, sys, time, urllib.parse
from pathlib import Path
base, port = Path(sys.argv[1]), int(sys.argv[2])
def request(device, path, token=None, cookie=None, form=None):
    ctx = ssl.create_default_context(cafile=str(base/'ca.pem'))
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    if device:
        ctx.load_cert_chain(str(base/(device+'.pem')), str(base/(device+'.key')))
    conn = http.client.HTTPSConnection('127.0.0.1', port, context=ctx, timeout=3)
    headers = {}
    if token: headers['Authorization'] = 'Bearer '+token
    if cookie: headers['Cookie'] = cookie
    if form:
        headers['Content-Type'] = 'application/x-www-form-urlencoded'
        headers['Origin'] = 'https://127.0.0.1:'+str(port)
    try:
        conn.request('POST' if form else 'GET', path, urllib.parse.urlencode(form) if form else None, headers)
        response = conn.getresponse()
        status, cookie, body = response.status, response.getheader('Set-Cookie'), response.read()
        return status, cookie, body
    finally:
        conn.close()
for _ in range(100):
    try:
        assert request('device-a', '/healthz')[0] == 200
        break
    except (OSError, http.client.HTTPException):
        time.sleep(.1)
else:
    raise SystemExit('Dashboard did not become ready')
admin, metrics = os.environ['FLEETDECK_API_TOKEN'], os.environ['FLEETDECK_METRICS_TOKEN']
for device in [None, 'unlisted']:
    try: request(device, '/api/projects', token=admin)
    except (OSError, http.client.HTTPException): pass
    else: raise AssertionError('Unauthorized device reached the API')
assert request('device-a', '/api/projects')[0] == 401
assert request('device-a', '/api/projects', token=admin)[0] == 200
assert request('device-a', '/metrics', token=metrics)[0] == 200
assert request('device-a', '/api/projects', token=metrics)[0] == 401
assert request('device-a', '/login', form={'token': metrics})[0] == 401
status, cookie, _ = request('device-a', '/login', form={'token': admin})
assert status == 302 and cookie and 'Secure' in cookie and 'HttpOnly' in cookie
session = cookie.split(';', 1)[0]
assert request('device-a', '/api/projects', cookie=session)[0] == 200
assert request('device-b', '/api/projects', cookie=session)[0] == 401
assert request('device-a', '/api/projects', cookie=session)[0] == 200
print('CLI listener policy, mTLS admission, device-bound sessions and monitoring scope passed.')
PY
