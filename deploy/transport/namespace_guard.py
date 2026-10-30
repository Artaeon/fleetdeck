#!/usr/bin/env python3
"""Optional root-only exposure reduction for running Docker network namespaces.

Owns one nft table per namespace. Never restarts Docker or an application.
Does not fix the kernel, close existing sockets, or provide a zero-race start hook.
"""
import argparse
import fcntl
import hashlib
import json
import os
import pathlib
import re
import selectors
import signal
import stat
import subprocess
import time

TABLE = 'fleetdeck_transport_guard'
RULES = '''destroy table inet fleetdeck_transport_guard
table inet fleetdeck_transport_guard {
 chain ingress {
  type filter hook prerouting priority -310; policy accept;
  ip hdrlength > 5 counter drop
 }
 chain egress {
  type filter hook output priority -310; policy accept;
  ip hdrlength > 5 counter drop
 }
}
'''


def command(args, *, data=None, check=True, fd=None):
    r = subprocess.run(args, input=data, text=True, capture_output=True,
                       timeout=30, pass_fds=() if fd is None else (fd,))
    if check and r.returncode:
        # nft stderr can contain an operator's rules; keep it out of journals.
        raise RuntimeError('command failed: ' + args[0])
    return r


def in_namespace(fd, args, **kwargs):
    return command(['nsenter', '--net=/proc/self/fd/' + str(fd), '--'] + args,
                   fd=fd, **kwargs)


def private_dir(path):
    path.mkdir(parents=True, mode=0o700, exist_ok=True)
    s = path.lstat()
    if not stat.S_ISDIR(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o077:
        raise RuntimeError('state directory must be a root-only directory')


def read_state(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd) as f:
        s = os.fstat(f.fileno())
        if not stat.S_ISREG(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o077:
            raise RuntimeError('unsafe state file')
        return json.load(f)


def atomic_json(path, value):
    temporary = path.with_name(path.name + '.new')
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as f:
        os.fchmod(f.fileno(), 0o600)
        json.dump(value, f)
        f.flush()
        os.fsync(f.fileno())
    os.replace(temporary, path)


def containers():
    ids = command(['docker', 'ps', '--no-trunc', '-q']).stdout.split()
    if not all(re.fullmatch('[a-f0-9]{64}', x) for x in ids):
        raise RuntimeError('invalid container inventory')
    if not ids:
        return []
    # Only request non-secret fields; never collect Config.Env.
    fields = '{{json .Id}} {{json .State.Pid}} {{json .HostConfig.Privileged}} {{json .HostConfig.CapAdd}} {{json .State.StartedAt}}'
    result = []
    for line in command(['docker', 'inspect', '--format', fields] + ids).stdout.splitlines():
        dec = json.JSONDecoder()
        values = []
        while line.strip():
            value, end = dec.raw_decode(line.lstrip())
            values.append(value)
            line = line.lstrip()[end:]
        cid, pid, privileged, caps, started = values
        if not isinstance(pid, int) or pid <= 0:
            continue
        result.append({'id': cid, 'pid': pid, 'started': started, 'unsafe': privileged or any(
            x.upper().removeprefix('CAP_') in ['NET_ADMIN', 'SYS_ADMIN', 'ALL'] for x in (caps or []))})
    return result


def pinned_groups(validate=True):
    groups = {}
    host = os.stat('/proc/1/ns/net').st_ino
    try:
        for c in containers():
            try:
                fd = os.open('/proc/' + str(c['pid']) + '/ns/net', os.O_RDONLY)
            except FileNotFoundError:
                continue  # Container exited between inspect and open.
            inode = os.fstat(fd).st_ino
            current = command(['docker', 'inspect', '--format', '{{.State.Pid}} {{.State.StartedAt}}', c['id']], check=False)
            if current.returncode or current.stdout.strip() != str(c['pid']) + ' ' + c['started']:
                os.close(fd)
                continue  # Never act on a PID recycled after the initial inspect.
            if inode == host:
                os.close(fd)
                if validate:
                    raise RuntimeError('host-network containers require a separate host policy')
                continue
            if validate and c['unsafe']:
                os.close(fd)
                raise RuntimeError('privileged/network-admin container requires separate review')
            if inode in groups:
                os.close(fd)
                groups[inode]['ids'].append(c['id'])
            else:
                groups[inode] = {'fd': fd, 'ids': [c['id']], 'inode': inode}
        return list(groups.values())
    except BaseException:
        for group in groups.values():
            os.close(group['fd'])
        raise


def snapshot(group, boot):
    fd = group['fd']
    old = in_namespace(fd, ['nft', 'list', 'table', 'inet', TABLE], check=False)
    if old.returncode and 'No such file or directory' not in old.stderr:
        raise RuntimeError('cannot snapshot existing namespace policy')
    setting = in_namespace(fd, ['sysctl', '-n', 'net.mptcp.enabled'], check=False)
    if setting.returncode and 'No such file or directory' not in setting.stderr:
        raise RuntimeError('cannot read MPTCP setting')
    value = setting.stdout.strip() if setting.returncode == 0 else None
    if value not in ['0', '1', None]:
        raise RuntimeError('unexpected MPTCP setting')
    return {'boot': boot, 'inode': group['inode'], 'ids': group['ids'],
            'mptcp': value, 'rules': 'destroy table inet ' + TABLE + '\n' + (old.stdout if old.returncode == 0 else '')}


def policy_hash(fd):
    r = in_namespace(fd, ['nft', '-j', 'list', 'table', 'inet', TABLE], check=False)
    if r.returncode:
        return None
    def stable(value):
        if isinstance(value, dict):
            return {k: stable(v) for k, v in value.items() if k not in ['metainfo', 'handle', 'packets', 'bytes']}
        if isinstance(value, list):
            return [stable(x) for x in value if not isinstance(x, dict) or 'metainfo' not in x]
        return value
    return hashlib.sha256(json.dumps(stable(json.loads(r.stdout)), sort_keys=True).encode()).hexdigest()


def apply(group, state_dir, boot):
    key = hashlib.sha256((boot + ':' + str(group['inode'])).encode()).hexdigest()
    path = state_dir / (key + '.json')
    old = read_state(path) if path.exists() else None
    if old is None or not set(old['ids']).intersection(group['ids']):
        old = snapshot(group, boot)
        # Snapshot is durable before any mutation. A rollback only targets a
        # namespace still occupied by an original container, never a recycled PID.
        atomic_json(path, old)
    fd = group['fd']
    source_hash = hashlib.sha256(RULES.encode()).hexdigest()
    if old.get('policy_source') != source_hash or policy_hash(fd) != old.get('applied_hash'):
        in_namespace(fd, ['nft', '--check', '-f', '-'], data=RULES)
        in_namespace(fd, ['nft', '-f', '-'], data=RULES)
        old['policy_source'] = source_hash
        old['applied_hash'] = policy_hash(fd)
        atomic_json(path, old)
    if old['mptcp'] is not None:
        in_namespace(fd, ['sysctl', '-qw', 'net.mptcp.enabled=0'])


def reconcile(state_dir, rollback=False, verify=False):
    boot = pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip()
    groups = pinned_groups(validate=not rollback)
    try:
        if rollback:
            saved = [read_state(p) for p in state_dir.glob('*.json')]
            for group in groups:
                match = [s for s in saved if s['boot'] == boot and s['inode'] == group['inode']
                         and set(s['ids']).intersection(group['ids'])]
                if not match:
                    continue
                old = match[0]
                in_namespace(group['fd'], ['nft', '-f', '-'], data=old['rules'])
                if old['mptcp'] is not None:
                    in_namespace(group['fd'], ['sysctl', '-qw', 'net.mptcp.enabled=' + old['mptcp']])
        elif verify:
            source_hash = hashlib.sha256(RULES.encode()).hexdigest()
            for group in groups:
                key = hashlib.sha256((boot + ':' + str(group['inode'])).encode()).hexdigest()
                old = read_state(state_dir / (key + '.json'))
                if (old.get('policy_source') != source_hash or
                        not old.get('applied_hash') or
                        policy_hash(group['fd']) != old['applied_hash']):
                    raise RuntimeError('namespace policy is not current')
                if old['mptcp'] is not None and in_namespace(
                        group['fd'], ['sysctl', '-n', 'net.mptcp.enabled']).stdout.strip() != '0':
                    raise RuntimeError('namespace MPTCP setting is not disabled')
        else:
            for group in groups:
                apply(group, state_dir, boot)
        return len(groups)
    finally:
        for group in groups:
            os.close(group['fd'])


def locked_reconcile(state_dir, rollback=False, verify=False):
    fd = os.open(state_dir / 'lock', os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        return reconcile(state_dir, rollback, verify)


def watch(state_dir):
    # Subscribe before the first scan. Periodic reconciliation also handles a
    # lost event. The interval/event delivery is not an atomic container-start gate.
    process = subprocess.Popen(['docker', 'events', '--filter', 'type=container',
                                '--filter', 'event=start', '--format', '{{.Actor.ID}}'],
                               stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ)
    try:
        while process.poll() is None:
            count = locked_reconcile(state_dir)
            print('Protected container network namespaces:', count, flush=True)
            ready = selector.select(timeout=30)
            if ready:
                # Drain a bounded event chunk. Never interpolate event text into a command.
                if not os.read(process.stdout.fileno(), 65536):
                    break
        raise RuntimeError('Docker event stream ended; supervision must restart the watcher')
    finally:
        selector.close()
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['apply', 'rollback', 'watch', 'verify'])
    parser.add_argument('--state-dir', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise SystemExit('requires root on the reviewed Docker host')
    def terminate(signum, frame):
        raise SystemExit(0)
    signal.signal(signal.SIGTERM, terminate)
    os.umask(0o077)
    private_dir(args.state_dir)
    if args.action == 'watch':
        watch(args.state_dir)
    else:
        print('Container network namespaces processed:',
              locked_reconcile(args.state_dir, args.action == 'rollback', args.action == 'verify'))


if __name__ == '__main__':
    main()
