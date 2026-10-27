#!/bin/sh
# Install root-owned 0755 as /usr/local/sbin/fleetdeck-ci-example.
# An exact sudoers rule must allow only these two commands. No wildcards.
set -eu
case "${SSH_ORIGINAL_COMMAND-}" in
  apply|status) action="$SSH_ORIGINAL_COMMAND" ;;
  *) exit 126 ;;
esac
exec /usr/bin/sudo -n /usr/bin/timeout --signal=TERM --kill-after=15s 35m \
  /usr/bin/env -i PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin HOME=/root \
  "SSH_ORIGINAL_COMMAND=$action" /usr/local/bin/fleetdeck \
  --config /etc/fleetdeck/ci/example.toml release receive --target example-staging
