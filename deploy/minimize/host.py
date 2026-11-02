#!/usr/bin/env python3
"""Explicit, reversible removal of optional peripheral services from a VPS."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import time

# Deliberately exclude SSH, networking, storage, Docker, logging and update agents.
OPTIONAL = {"ModemManager.service", "udisks2.service", "fwupd.service", "fwupd-refresh.timer"}
FIELDS = ("Id", "LoadState", "ActiveState", "UnitFileState", "FragmentPath")


def run(args, check=True):
    result = subprocess.run(args, capture_output=True, text=True, timeout=45)
    if check and result.returncode:
        raise RuntimeError("host service operation failed; inspect the private system journal")
    return result


def identity():
    return hashlib.sha256(Path("/etc/machine-id").read_bytes()).hexdigest()


def inspect_unit(name):
    result = run(["systemctl", "show", name, *["--property=" + f for f in FIELDS]])
    values = dict(line.split("=", 1) for line in result.stdout.splitlines() if "=" in line)
    return {key: values.get(key, "") for key in FIELDS}


def validate_plan(plan):
    if set(plan) != {"version", "host", "units"} or plan["version"] != 1 or plan["host"] != identity():
        raise ValueError("plan is invalid or belongs to another host")
    if not isinstance(plan["units"], dict) or not plan["units"] or not set(plan["units"]) <= OPTIONAL:
        raise ValueError("only explicitly selected optional peripheral services are supported")
    for name, before in plan["units"].items():
        if set(before) != set(FIELDS) or before["Id"] != name or before["LoadState"] != "loaded":
            raise ValueError("missing units and aliases cannot be changed")
        if before["ActiveState"] not in ("active", "inactive") or before["UnitFileState"] not in ("enabled", "disabled", "static"):
            raise ValueError("unsupported service state; review it manually")
        if not before["FragmentPath"].startswith(("/usr/lib/systemd/system/", "/lib/systemd/system/")):
            raise ValueError("operator-owned replacement units require manual review")


def write_state(directory, state):
    import tempfile
    data = json.dumps(state).encode()
    fd, name = tempfile.mkstemp(prefix=".state-", dir=directory)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(data)
            f.flush()
            os.fsync(f.fileno())
        os.replace(name, directory / "state.json")
        fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(fd)
        finally:
            os.close(fd)
    finally:
        if os.path.exists(name):
            os.unlink(name)


def read_private(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(fd, "rb") as f:
        st = os.fstat(f.fileno())
        if not stat.S_ISREG(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o077 or st.st_size > 65536:
            raise ValueError("plan/state must be a root-owned private regular file")
        return json.load(f)


def rollback(directory, state, if_pending=False):
    if if_pending and state["phase"] != "pending":
        return
    # Retain evidence and retry the complete restoration if any command fails.
    for name, before in state["plan"]["units"].items():
        run(["systemctl", "unmask", "--runtime", name])
        run(["systemctl", "unmask", name])
        if before["UnitFileState"] == "enabled":
            run(["systemctl", "enable", name])
        elif before["UnitFileState"] == "disabled":
            run(["systemctl", "disable", name])
        run(["systemctl", "start" if before["ActiveState"] == "active" else "stop", name])
        if inspect_unit(name) != before:
            raise RuntimeError("restored service state differs; manual recovery is required")
    state["phase"] = "rolled_back"
    write_state(directory, state)
    run(["systemctl", "stop", state["timer"] + ".timer"], check=False)


def apply(directory, plan, script):
    validate_plan(plan)
    if (directory / "state.json").exists():
        raise ValueError("use a fresh state directory; previous recovery evidence is retained")
    if any(inspect_unit(name) != before for name, before in plan["units"].items()):
        raise ValueError("host changed since review; regenerate the plan")
    token = hashlib.sha256(str(directory).encode()).hexdigest()[:16]
    state = {"plan": plan, "phase": "pending", "deadline": time.time() + 300,
             "timer": "fleetdeck-minimize-rollback-" + token}
    write_state(directory, state)
    # Arm recovery before the first mutation. No kernel/module changes are made.
    run(["systemd-run", "--unit=" + state["timer"], "--on-active=5m",
         "--timer-property=AccuracySec=1s", "--property=Restart=on-failure",
         "--property=RestartSec=15s", sys.executable, str(script), "rollback",
         "--state-dir", str(directory), "--if-pending"])
    try:
        # Stop triggers before their service. A timer can otherwise enter the
        # failed state while its target has already been masked.
        for name in sorted(plan["units"], key=lambda n: (not n.endswith(".timer"), n)):
            run(["systemctl", "mask", "--runtime", "--now", name])
            current = inspect_unit(name)
            if current["ActiveState"] == "failed" and current["UnitFileState"] == "masked-runtime":
                run(["systemctl", "reset-failed", name])
                current = inspect_unit(name)
            if current["UnitFileState"] != "masked-runtime" or current["ActiveState"] != "inactive":
                raise RuntimeError("optional service did not stop")
    except Exception:
        rollback(directory, state)
        raise
    return state


def confirm(directory, state):
    if state["phase"] != "pending" or time.time() >= state["deadline"] - 10:
        raise ValueError("confirmation window expired or no change is pending")
    for name in state["plan"]["units"]:
        current = inspect_unit(name)
        if current["UnitFileState"] != "masked-runtime" or current["ActiveState"] != "inactive":
            raise ValueError("service state changed; refusing confirmation")
    try:
        for name in state["plan"]["units"]:
            run(["systemctl", "mask", name])
            run(["systemctl", "unmask", "--runtime", name])
    except Exception:
        rollback(directory, state)
        raise
    state["phase"] = "confirmed"
    write_state(directory, state)
    run(["systemctl", "stop", state["timer"] + ".timer"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["plan", "apply", "confirm", "rollback"])
    parser.add_argument("--service", action="append", default=[], choices=sorted(OPTIONAL))
    parser.add_argument("--plan", type=Path)
    parser.add_argument("--state-dir", type=Path)
    parser.add_argument("--if-pending", action="store_true")
    args = parser.parse_args()
    if args.action == "plan":
        plan = {"version": 1, "host": identity(), "units": {n: inspect_unit(n) for n in args.service}}
        validate_plan(plan)
        print(json.dumps(plan, indent=2))
        return
    if os.geteuid() != 0 or not args.state_dir or not args.state_dir.is_absolute():
        raise ValueError("administration requires root and an absolute private state directory")
    script = Path(__file__).resolve()
    st = script.stat()
    if st.st_uid != 0 or st.st_mode & 0o022:
        raise ValueError("install this helper in a root-owned path before applying changes")
    # Every parent must also prevent replacing the recovery executable.
    for parent in script.parents:
        st = parent.stat()
        if st.st_uid != 0 or st.st_mode & 0o022:
            raise ValueError("recovery executable parent directories must be root-controlled")
    directory = args.state_dir
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    st = directory.lstat()
    if not stat.S_ISDIR(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o077:
        raise ValueError("state directory must be private and root-owned, not a symlink")
    for parent in directory.parents:
        st = parent.stat()
        if st.st_uid != 0 or st.st_mode & 0o022:
            raise ValueError("state directory parents must be root-controlled")
    fd = os.open(directory / "lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600)
    try:
        st = os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_uid != 0 or st.st_mode & 0o077:
            raise ValueError("invalid state lock")
        fcntl.flock(fd, fcntl.LOCK_EX)
        if args.action == "apply":
            if not args.plan:
                raise ValueError("a reviewed plan is required")
            state = apply(directory, read_private(args.plan), script)
        else:
            state = read_private(directory / "state.json")
            validate_plan(state["plan"])
            if args.action == "rollback":
                rollback(directory, state, args.if_pending)
            else:
                confirm(directory, state)
        print(json.dumps({"phase": state["phase"], "services": len(state["plan"]["units"])}))
    finally:
        os.close(fd)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, KeyError, subprocess.TimeoutExpired) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
