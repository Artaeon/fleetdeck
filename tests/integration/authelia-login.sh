#!/usr/bin/env bash
# Disposable real Authelia + browser test. No published ports or real identities.
set -euo pipefail
binary=${FLEETDECK_BINARY:?Set FLEETDECK_BINARY to a Linux CLI}
source_dir=$(cd "$(dirname "$0")" && pwd)
base=$(mktemp -d)
name="fleetdeck-oidc-$$"
authelia=authelia/authelia:4.39.28@sha256:bd97cff4fcbf715b5ff1f9ae286afbe6033afce385302520b0368122d43a6f54
browser=mcr.microsoft.com/playwright:v1.64.1-noble
cleanup() {
  local status=$?
  if [ "$status" -ne 0 ]; then
    docker logs --tail 15 "$name-auth" >&2 2>/dev/null || true
    if [ -f "$base/dashboard.log" ]; then tail -n 15 "$base/dashboard.log" >&2; fi
  fi
  docker rm -f "$name-proxy" "$name-auth" "$name-browser" >/dev/null 2>&1 || true
  docker network rm "$name" >/dev/null 2>&1 || true
  rm -rf "$base"
}
trap cleanup EXIT
umask 077
mkdir -p "$base/projects" "$base/data"
cp "$binary" "$base/fleetdeck"
cp "$source_dir/authelia-login.mjs" "$base/test.mjs"
"$binary" dashboard setup-authelia --issuer https://auth.example.test:8443 \
  --public-url https://admin.example.test:8443 --output "$base/setup"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj /CN=auth.example.test \
  -addext 'subjectAltName=DNS:auth.example.test,DNS:admin.example.test' \
  -keyout "$base/tls.key" -out "$base/tls.pem" >/dev/null 2>&1
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$base/signing.pem" >/dev/null 2>&1
python3 - "$base" <<'PY'
import base64, hashlib, secrets, sys
from pathlib import Path
p=Path(sys.argv[1]); password=secrets.token_hex(32); salt=secrets.token_bytes(16)
encode=lambda x:base64.b64encode(x).decode().rstrip('=').replace('+','.')
digest='$pbkdf2-sha512$310000$'+encode(salt)+'$'+encode(hashlib.pbkdf2_hmac('sha512',password.encode(),salt,310000))
(p/'password').write_text(password)
(p/'users.yml').write_text('users:\n  testadmin:\n    displayname: Test Admin\n    password: "'+digest+'"\n    email: testadmin@example.test\n    groups: [fleetdeck-admins]\n')
key='\n'.join('        '+line for line in (p/'signing.pem').read_text().splitlines())
(p/'authelia.yml').write_text('''server:
  address: tcp://127.0.0.1:9091/
log:
  level: warn
authentication_backend:
  file:
    path: /work/users.yml
access_control:
  default_policy: deny
session:
  secret: "'''+secrets.token_hex(32)+'''"
  cookies:
    - domain: example.test
      authelia_url: https://auth.example.test:8443
      default_redirection_url: https://admin.example.test:8443
storage:
  encryption_key: "'''+secrets.token_hex(32)+'''"
  local:
    path: /data/db.sqlite3
notifier:
  filesystem:
    filename: /data/notifications.txt
identity_validation:
  reset_password:
    jwt_secret: "'''+secrets.token_hex(32)+'''"
webauthn:
  enable_passkey_login: true
  attestation_conveyance_preference: none
  selection_criteria:
    discoverability: required
    user_verification: required
identity_providers:
  oidc:
    hmac_secret: "'''+secrets.token_hex(32)+'''"
    jwks:
      - key_id: test-signing-key
        algorithm: RS256
        use: sig
        key: |
'''+ '\n'.join('  '+line for line in key.splitlines())+'\n')
fleet=(p/'setup/fleetdeck-oidc.toml').read_text().replace(str(p),'/work')
(p/'fleetdeck.toml').write_text('[server]\nbase_path = "/work/projects"\n[audit]\nenabled = false\n'+fleet)
PY
cat > "$base/Caddyfile" <<'CADDY'
{
  admin off
  auto_https off
}
https://auth.example.test:8443 {
  tls /work/tls.pem /work/tls.key
  reverse_proxy 127.0.0.1:9091
}
https://admin.example.test:8443 {
  tls /work/tls.pem /work/tls.key
  reverse_proxy 127.0.0.1:8420
}
CADDY
docker network create "$name" >/dev/null
docker run -d --name "$name-browser" --network "$name" \
  --network-alias auth.example.test --network-alias admin.example.test \
  --cap-drop=ALL --security-opt=no-new-privileges:true --pids-limit=256 --memory=1g --shm-size=256m \
  -v "$base:/work" "$browser" sleep infinity >/dev/null
docker run --rm --network none -v "$base:/work:ro" -v "$base/data:/data" \
  "$authelia" authelia config validate --config /work/authelia.yml --config /work/setup/authelia-client.yml
docker run -d --name "$name-auth" --network "container:$name-browser" --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges:true --pids-limit=128 --memory=256m \
  -v "$base:/work:ro" -v "$base/data:/data" "$authelia" \
  authelia --config /work/authelia.yml --config /work/setup/authelia-client.yml >/dev/null
docker run -d --name "$name-proxy" --network "container:$name-browser" --read-only \
  --cap-drop=ALL --security-opt=no-new-privileges:true --pids-limit=64 --memory=128m \
  --tmpfs /data --tmpfs /config -v "$base:/work:ro" caddy:2 \
  caddy run --config /work/Caddyfile --adapter caddyfile >/dev/null
docker exec "$name-browser" npm install --prefix /work/npm --ignore-scripts --no-audit --no-fund playwright@1.64.1 >/dev/null
docker exec "$name-browser" node -e 'const https=require("https"),fs=require("fs");let n=0;function check(){https.get("https://auth.example.test:8443/.well-known/openid-configuration",{ca:fs.readFileSync("/work/tls.pem")},r=>{r.resume();if(r.statusCode===200)process.exit(0);retry()}).on("error",retry)}function retry(){if(++n>100)process.exit(1);setTimeout(check,200)}check()'
docker exec -d "$name-browser" sh -c 'SSL_CERT_FILE=/work/tls.pem FLEETDECK_TRUST_PROXY_IPS=127.0.0.1 /work/fleetdeck --config /work/fleetdeck.toml dashboard > /work/dashboard.log 2>&1'
docker exec "$name-browser" node /work/test.mjs
