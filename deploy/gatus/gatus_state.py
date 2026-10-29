#!/usr/bin/env python3
"""Import local Gatus observations without changing checks or sending alerts."""
import argparse
import datetime as dt
import ipaddress
import json
import os
from pathlib import Path
import re
import stat
import tempfile
import urllib.parse
import urllib.request

MAX_BYTES = 2 << 20
API_PATH = "/api/v1/endpoints/statuses"


def timestamp(value):
    if not isinstance(value, str):
        raise ValueError("invalid timestamp")
    match = re.fullmatch(r"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d{1,9})?(Z|[+-]\d{2}:\d{2})", value)
    if not match:
        raise ValueError("invalid timestamp")
    fraction = ((match[2] or ".")[1:] + "000000")[:6]
    zone = "+00:00" if match[3] == "Z" else match[3]
    return dt.datetime.fromisoformat(match[1] + "." + fraction + zone)


def convert(entries, now, interval=60):
    if not isinstance(entries, list) or len(entries) > 100:
        raise ValueError("invalid endpoint inventory")
    if type(interval) is not int or not 1 <= interval <= 3600:
        raise ValueError("invalid interval")
    state = {"targets": [], "results": {}, "updated_at": now.isoformat()}
    seen = set()
    for entry in entries:
        if not isinstance(entry, dict):
            raise ValueError("invalid endpoint")
        name = entry.get("key")
        if not isinstance(name, str) or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}", name) or name in seen:
            raise ValueError("invalid endpoint key")
        seen.add(name)
        target = {"name": name, "interval": interval * 1_000_000_000, "timeout": 10_000_000_000}
        state["targets"].append(target)
        results = entry.get("results", [])
        if not isinstance(results, list) or len(results) > 1000:
            raise ValueError("invalid results")
        if not results:
            continue  # FleetDeck represents a target without a result as unknown.
        parsed = []
        for result in results:
            if not isinstance(result, dict) or type(result.get("success")) is not bool:
                raise ValueError("invalid result")
            when = timestamp(result.get("timestamp"))
            # Gatus external/push endpoints report success without conditions.
            conditions = result.get("conditionResults", [])
            if not isinstance(conditions, list) or len(conditions) > 64:
                raise ValueError("invalid conditions")
            if any(not isinstance(c, dict) or type(c.get("success")) is not bool for c in conditions):
                raise ValueError("invalid conditions")
            code, duration = result.get("status", 0), result.get("duration", 0)
            if type(code) is not int or not 0 <= code <= 599 or type(duration) is not int or not 0 <= duration <= 3_600_000_000_000:
                raise ValueError("invalid result values")
            errors = result.get("errors", [])
            if not isinstance(errors, list) or len(errors) > 64:
                raise ValueError("invalid errors")
            healthy = result["success"] and not errors and all(c["success"] for c in conditions)
            parsed.append((when, healthy, result))
        # Preserve the actual check time, including stale/future timestamps.
        # Never turn the import time into evidence of a fresh successful check.
        newest = max(t for t, _, _ in parsed)
        same_time = [(ok, r) for t, ok, r in parsed if t == newest]
        healthy = all(ok for ok, _ in same_time)
        result = next((r for ok, r in same_time if not ok), same_time[0][1])
        state["results"][name] = {
            "target": target,
            "status_code": result.get("status", 0),
            "response_time": result.get("duration", 0),
            "healthy": healthy,
            "checked_at": result["timestamp"],
        }
    return state


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("redirect refused")


def fetch(endpoint):
    parsed = urllib.parse.urlsplit(endpoint)
    if parsed.scheme not in ("http", "https") or parsed.username is not None or parsed.password is not None or parsed.query or parsed.fragment or parsed.path != API_PATH:
        raise ValueError("invalid endpoint")
    if not ipaddress.ip_address(parsed.hostname).is_loopback:
        raise ValueError("endpoint must use a literal loopback address")
    if parsed.port is not None and not 1 <= parsed.port <= 65535:
        raise ValueError("invalid port")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    with opener.open(endpoint, timeout=10) as response:
        if response.status != 200:
            raise ValueError("unexpected status")
        data = response.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES:
        raise ValueError("response too large")
    return json.loads(data)


def write_state(path, state):
    path = Path(path)
    info = path.parent.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_mode & 0o022:
        raise ValueError("state directory must be private and not linked")
    data = json.dumps(state).encode()
    if len(data) > MAX_BYTES:
        raise ValueError("state too large")
    fd, temporary = tempfile.mkstemp(prefix=".gatus-state-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def collect(endpoint, output, interval=60):
    now = dt.datetime.now(dt.timezone.utc)
    try:
        state = convert(fetch(endpoint), now, interval)
    except Exception:
        # Replace prior success immediately; do not emit URLs, response bodies,
        # condition expressions, hostnames, transport errors or credentials.
        write_state(output, {"targets": [], "results": {}, "updated_at": now.isoformat()})
        return False
    write_state(output, state)
    return True


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--endpoint", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--interval", type=int, default=60, help="Configured Gatus interval in seconds")
    args = parser.parse_args()
    if not collect(args.endpoint, args.output, args.interval):
        raise SystemExit("Gatus import failed; current health is unknown.")
