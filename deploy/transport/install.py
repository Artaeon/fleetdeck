#!/usr/bin/env python3
"""Opt-in first installation with an independent systemd rollback timer.

Run on the reviewed Docker host as root. Never restarts Docker or applications.
Existing installations require explicit operator review; they are not overwritten.
"""
import argparse
import fcntl
import json
import os
import pathlib
import re
import shutil
import subprocess
import time

import namespace_guard as guard

UNIT = 'fleetdeck-transport-guard.service'
UNIT_PATH = pathlib.Path('/etc/systemd/system') / UNIT
ROOT = pathlib.Path('/var/lib/fleetdeck/transport-rollouts')


def run(args, **kwargs):
    return guard.command(args, **kwargs)


def active(unit):
    return run(['systemctl', 'is-active', '--quiet', unit], check=False).returncode == 0


def helper(d, action):
    return run(['/usr/bin/python3', str(d / 'namespace_guard.py'), action,
                '--state-dir', str(d / 'namespaces')])


def inventory():
    return {c['id']: [c['pid'], c['started']] for c in guard.containers()}


def validate(d, state, compare_processes=True):
    if not active(UNIT):
        raise RuntimeError('namespace guard service is not running')
    if compare_processes and (not active('docker.service') or run([
            'systemctl', 'show', '-p', 'MainPID', '--value', 'docker.service'
    ]).stdout.strip() != state['docker_pid']):
        raise RuntimeError('Docker process changed during trial')
    current = inventory() if compare_processes else {}
    if compare_processes and any(current.get(cid) != process for cid, process in state['containers'].items()):
        raise RuntimeError('an application process changed during trial')
    helper(d, 'verify')


def save(d, state):
    guard.atomic_json(d / 'state.json', state)


def rollback(d, state):
    if state['phase'] == 'rolled-back':
        return
    # No Requires edges to Docker or applications. Stopping this watcher must
    # never propagate a stop/restart to a workload.
    run(['systemctl', 'stop', UNIT])
    helper(d, 'rollback')
    run(['systemctl', 'disable', UNIT], check=False)
    UNIT_PATH.unlink(missing_ok=True)
    run(['systemctl', 'daemon-reload'])
    state['phase'] = 'rolled-back'
    save(d, state)
    run(['systemctl', 'stop', state['timer']], check=False)
    print('Original container namespace policy restored.', flush=True)


def apply(d, timeout):
    if UNIT_PATH.exists() or UNIT_PATH.is_symlink() or active(UNIT):
        raise RuntimeError('existing guard installation requires explicit review')
    if run(['systemctl', 'show', '-p', 'LoadState', '--value', UNIT]).stdout.strip() != 'not-found':
        raise RuntimeError('guard unit already provided elsewhere')
    for previous in ROOT.glob('*/state.json'):
        if guard.read_state(previous)['phase'] not in ['rolled-back', 'confirmed']:
            raise RuntimeError('another rollout is unfinished')
    groups = guard.pinned_groups()
    for group in groups:
        os.close(group['fd'])
    docker_pid = run(['systemctl', 'show', '-p', 'MainPID', '--value', 'docker.service']).stdout.strip()
    if not active('docker.service') or not docker_pid.isdigit() or docker_pid == '0':
        raise RuntimeError('requires a running systemd Docker service')
    d.mkdir(mode=0o700)
    for name in ['install.py', 'namespace_guard.py']:
        shutil.copyfile(pathlib.Path(__file__).with_name(name), d / name)
        os.chmod(d / name, 0o600)
    state = {'phase': 'preparing', 'docker_pid': docker_pid, 'containers': inventory(),
             'timer': 'fleetdeck-transport-rollback-' + d.name + '.timer'}
    save(d, state)
    run(['systemd-run', '--collect', '--unit=' + state['timer'][:-6],
         '--on-active=' + str(timeout) + 's', '--timer-property=AccuracySec=1s',
         '/usr/bin/python3', str(d / 'install.py'), 'rollback', '--id', d.name, '--automatic'])
    try:
        unit = '''[Unit]
Description=FleetDeck container transport exposure reduction
After=docker.service
[Service]
Type=simple
ExecStart=/usr/bin/python3 {directory}/namespace_guard.py watch --state-dir {directory}/namespaces
Restart=on-failure
RestartSec=5s
TimeoutStopSec=45s
UMask=0077
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths={directory}
ProtectHome=true
PrivateTmp=true
ProtectKernelModules=true
ProtectKernelLogs=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
MemoryMax=256M
TasksMax=32
[Install]
WantedBy=multi-user.target
'''.format(directory=d)
        fd = os.open(UNIT_PATH, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o644)
        with os.fdopen(fd, 'w') as stream:
            stream.write(unit)
            stream.flush()
            os.fsync(stream.fileno())
        run(['systemctl', 'daemon-reload'])
        # Apply synchronously so confirmation cannot race the watcher's first scan.
        helper(d, 'apply')
        run(['systemctl', 'start', UNIT])
        validate(d, state)
        state['phase'] = 'trial'
        save(d, state)
    except BaseException:
        rollback(d, state)
        raise
    print('Trial active; independent automatic rollback is armed. Verify application access before confirm.', flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['apply', 'confirm', 'rollback', 'status'])
    parser.add_argument('--id', required=True)
    parser.add_argument('--timeout', type=int, default=300)
    parser.add_argument('--automatic', action='store_true')
    args = parser.parse_args()
    if os.geteuid() != 0 or not re.fullmatch('[a-z0-9-]{1,40}', args.id) or not 15 <= args.timeout <= 600:
        raise SystemExit('requires root, a safe rollout ID and a 15–600 second timeout')
    os.umask(0o077)
    guard.private_dir(ROOT)
    fd = os.open(ROOT / 'lock', os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'w') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        d = ROOT / args.id
        if args.action == 'apply':
            apply(d, args.timeout)
            return
        guard.private_dir(d)
        state = guard.read_state(d / 'state.json')
        if args.action == 'rollback':
            if args.automatic and state['phase'] == 'confirmed':
                return  # An already queued timer cannot undo a confirmed trial.
            rollback(d, state)
        elif args.action == 'confirm':
            if state['phase'] != 'trial' or not active(state['timer']):
                raise RuntimeError('trial is no longer available for confirmation')
            validate(d, state)
            run(['systemctl', 'enable', UNIT])
            state['phase'] = 'confirmed'
            state['confirmed_at'] = time.time()
            save(d, state)
            run(['systemctl', 'stop', state['timer']])
            print('Verified guard enabled for future boots.', flush=True)
        else:
            if state['phase'] in ['trial', 'confirmed']:
                validate(d, state, compare_processes=state['phase'] == 'trial')
            print('Rollout state:', state['phase'])


if __name__ == '__main__':
    main()
