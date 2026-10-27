#!/usr/bin/env python3
"""Test CI transport in a disposable container, using synthetic credentials.

Usage: python3 scripts/test_ci_ssh.py /path/to/linux-fleetdeck [linux/amd64]
Requires Docker, ssh and ssh-keygen. The binary must match the selected platform.
No existing server settings or application containers are changed.
"""

import json
import os
import pathlib
import subprocess
import sys
import tempfile
import uuid

REPO = pathlib.Path(__file__).resolve().parents[1]
BINARY = pathlib.Path(sys.argv[1]).resolve()
PLATFORM = sys.argv[2] if len(sys.argv) > 2 else "linux/amd64"
assert BINARY.is_file()
assert PLATFORM in ("linux/amd64", "linux/arm64")
CONTAINER = "fleetdeck-ci-test-" + uuid.uuid4().hex[:12]
os.umask(0o077)


def call(args, *, check=True, timeout=60, data=None):
    result = subprocess.run(args, input=data, capture_output=True, timeout=timeout)
    if check and result.returncode:
        error = result.stderr.decode(errors="replace")[-2000:]
        raise RuntimeError(f"Fixture command failed ({args[0]}): {error}")
    return result


def execute(*args):
    return call(["docker", "exec", CONTAINER, *args])


def install(source, destination, mode):
    call(["docker", "cp", str(source), CONTAINER + ":" + destination])
    execute("chown", "root:root", destination)
    execute("chmod", mode, destination)


def test_transport(directory):
    call([
        "docker", "run", "-d", "--platform", PLATFORM, "--name", CONTAINER,
        "--label", "io.fleetdeck.test=ci-policy", "-p", "127.0.0.1::2222",
        "ubuntu:24.04", "sleep", "1800",
    ], timeout=180)
    call([
        "docker", "exec", CONTAINER, "sh", "-c",
        "apt-get update -qq && DEBIAN_FRONTEND=noninteractive "
        "apt-get install -y -qq --no-install-recommends openssh-server sudo",
    ], timeout=600)
    call([
        "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C",
        "synthetic-fixture", "-f", str(directory / "id"),
    ])
    execute("useradd", "-m", "-s", "/bin/sh", "fleetdeck-ci-example")
    execute(
        "mkdir", "-p", "/run/sshd", "/etc/ssh/fleetdeck-ci", "/etc/fleetdeck/ci",
        "/opt/fleetdeck", "/var/log/fleetdeck",
    )
    install(BINARY, "/usr/local/bin/fleetdeck", "755")
    examples = REPO / "deploy/ci"
    install(examples / "target.example.toml", "/etc/fleetdeck/ci/example.toml", "600")
    install(examples / "forced-command.example.sh", "/usr/local/sbin/fleetdeck-ci-example", "755")
    install(examples / "sudoers.example", "/etc/sudoers.d/fleetdeck-ci", "440")
    execute("/usr/sbin/visudo", "-cf", "/etc/sudoers.d/fleetdeck-ci")
    authorized = directory / "authorized"
    authorized.write_text("restrict " + (directory / "id.pub").read_text())
    install(authorized, "/etc/ssh/fleetdeck-ci/fleetdeck-ci-example", "644")
    config = directory / "sshd_config"
    config.write_text(
        "Port 2222\nListenAddress 0.0.0.0\n"
        "HostKey /etc/ssh/ssh_host_ed25519_key\nUsePAM yes\n"
        "PermitRootLogin no\nPasswordAuthentication no\n"
        "KbdInteractiveAuthentication yes\n"
        "AuthenticationMethods publickey,keyboard-interactive:pam\n"
        "AllowUsers fleetdeck-ci-example\n"
        "Subsystem sftp internal-sftp\n"
        + (examples / "sshd.example.conf").read_text()
    )
    install(config, "/etc/ssh/sshd_config", "600")
    execute("/usr/sbin/sshd", "-t")
    execute("/usr/sbin/sshd")
    endpoint = call(["docker", "port", CONTAINER, "2222/tcp"]).stdout.decode().strip()
    port = endpoint.rsplit(":", 1)[1]
    # Trust is provisioned through the test's container owner, never ssh-keyscan.
    host_key = execute("cat", "/etc/ssh/ssh_host_ed25519_key.pub").stdout.decode()
    known_hosts = directory / "known_hosts"
    known_hosts.write_text("[127.0.0.1]:" + port + " " + host_key)
    ssh = [
        "ssh", "-F", "/dev/null", "-i", str(directory / "id"), "-p", port,
        "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none",
        "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + str(known_hosts),
        "-o", "ConnectTimeout=5",
    ]
    host = "fleetdeck-ci-example@127.0.0.1"
    status = json.loads(call(ssh + [host, "status"]).stdout)
    assert status == {"target": "example-staging", "current_release_id": None}
    for command in [
        "id", "sudo id", "apply; id", "status --target other",
        "scp -t /tmp/file", "/usr/lib/openssh/sftp-server",
    ]:
        assert call(ssh + [host, command], check=False).returncode != 0
    assert call(ssh + ["-W", "127.0.0.1:2222", host], check=False).returncode != 0
    assert call(ssh + ["-s", host, "sftp"], check=False, data=b"").returncode != 0
    rejected = call(ssh + [host, "apply"], check=False, data=b'{"command":"id"}')
    assert rejected.returncode != 0
    # General sudo remains unavailable even after a hypothetical shell escape.
    denied = call([
        "docker", "exec", "-u", "fleetdeck-ci-example", CONTAINER,
        "sudo", "-n", "/bin/id",
    ], check=False)
    assert denied.returncode != 0
    ci = execute(
        "/usr/sbin/sshd", "-T", "-C",
        "user=fleetdeck-ci-example,host=localhost,addr=127.0.0.1",
    ).stdout.decode()
    for setting in [
        "authenticationmethods publickey\n", "disableforwarding yes\n",
        "permittty no\n", "permituserenvironment no\n",
    ]:
        assert setting in ci
    personal = execute(
        "/usr/sbin/sshd", "-T", "-C", "user=operator,host=localhost,addr=127.0.0.1",
    ).stdout.decode()
    assert "authenticationmethods publickey,keyboard-interactive:pam\n" in personal
    print("PASS: bounded SSH status, shell/SCP/SFTP/forwarding/sudo denial, personal MFA policy")


with tempfile.TemporaryDirectory(prefix="fleetdeck-ci-ssh-") as temporary:
    try:
        test_transport(pathlib.Path(temporary))
    finally:
        subprocess.run(
            ["docker", "rm", "-f", CONTAINER],
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
