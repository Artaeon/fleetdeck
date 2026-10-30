#!/usr/bin/env python3
"""Optional host bridge policy for reviewed, IPv4-only Docker proxy networks."""
import argparse
import fcntl
import hashlib
import ipaddress
import json
import os
import pathlib
import re
import stat
import subprocess
import time

TABLE = 'fleetdeck_tenant_guard'


def command(args, data=None):
    result = subprocess.run(args, input=data, text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError('command failed: ' + args[0])
    return result.stdout


def token(value, pattern, label):
    if not isinstance(value, str) or not re.fullmatch(pattern, value):
        raise ValueError('invalid ' + label)
    return value


def render(bridge, ports, table=TABLE):
    token(bridge, r'[a-zA-Z0-9_.-]{1,15}', 'bridge')
    token(table, r'[a-zA-Z0-9_]{1,40}', 'table')
    if not ports or len(ports) > 256:
        raise ValueError('expected 1..256 bridge ports')
    names = set()
    groups = {}
    for p in ports:
        token(p['port'], r'[a-zA-Z0-9_.-]{1,15}', 'port')
        token(p['mac'], r'[0-9a-f]{2}(:[0-9a-f]{2}){5}', 'MAC')
        token(p['group'], r'[a-zA-Z0-9_.-]{1,63}', 'group')
        ipaddress.IPv4Address(p['ip'])
        if p['port'] in names or not isinstance(p['proxy'], bool):
            raise ValueError('duplicate port or invalid proxy flag')
        names.add(p['port'])
        groups.setdefault(p['group'], []).append(p['port'])
    proxies = [p['port'] for p in ports if p['proxy']]
    if not proxies or len(proxies) > 4:
        raise ValueError('expected 1..4 reviewed proxy ports')
    q = lambda values: '{ ' + ', '.join(json.dumps(x) for x in sorted(values)) + ' }'
    lines = ['destroy table bridge ' + table, 'table bridge ' + table + ' {',
             ' chain ingress {', '  type filter hook prerouting priority -300; policy accept;',
             '  meta ibrname != ' + json.dumps(bridge) + ' return']
    for index, p in enumerate(ports):
        lines.append('  iifname ' + json.dumps(p['port']) + ' jump port_' + str(index))
    lines += ['  counter drop', ' }']
    for index, p in enumerate(ports):
        lines += [' chain port_' + str(index) + ' {',
                  '  ether saddr != ' + p['mac'] + ' counter drop',
                  '  ether type ip ip saddr != ' + p['ip'] + ' counter drop',
                  '  ether type arp arp saddr ether != ' + p['mac'] + ' counter drop',
                  '  ether type arp arp saddr ip != ' + p['ip'] + ' counter drop',
                  '  ether type != { ip, arp } counter drop', '  accept', ' }']
    lines += [' chain forward {', '  type filter hook forward priority -150; policy accept;',
              '  meta ibrname != ' + json.dumps(bridge) + ' return',
              '  meta obrname != ' + json.dumps(bridge) + ' return',
              '  iifname ' + q(proxies) + ' oifname ' + q(names) + ' accept',
              '  oifname ' + q(proxies) + ' iifname ' + q(names) + ' accept']
    for ports_in_group in groups.values():
        lines.append('  iifname ' + q(ports_in_group) + ' oifname ' + q(ports_in_group) + ' accept')
    return '\n'.join(lines + ['  counter drop', ' }', '}']) + '\n'


def read_config(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    with os.fdopen(fd) as f:
        s = os.fstat(f.fileno())
        if not stat.S_ISREG(s.st_mode) or s.st_uid != 0 or s.st_mode & 0o077:
            raise RuntimeError('configuration must be a private root-owned regular file')
        c = json.load(f)
    if set(c) != {'network', 'projects', 'proxy_containers'}:
        raise ValueError('unknown or missing configuration keys')
    token(c['network'], r'[a-zA-Z0-9_.-]{1,63}', 'network')
    if '__proxy' in c['projects']:
        raise ValueError('reserved project name')
    for key in ['projects', 'proxy_containers']:
        if not isinstance(c[key], list) or not c[key] or len(c[key]) != len(set(c[key])):
            raise ValueError('invalid allowlist')
        for x in c[key]:
            token(x, r'[a-zA-Z0-9_.-]{1,63}', key)
    return c


def snapshot(c):
    network = json.loads(command(['docker', 'network', 'inspect', c['network']]))[0]
    if network['Driver'] != 'bridge' or network['EnableIPv6'] or network.get('Internal'):
        raise RuntimeError('only reviewed IPv4 bridge proxy networks are supported')
    bridge = (network.get('Options') or {}).get('com.docker.network.bridge.name', 'br-' + network['Id'][:12])
    host_links = {x['ifindex']: x for x in json.loads(command(['ip', '-j', 'link', 'show']))}
    bridge_index = next(x['ifindex'] for x in host_links.values() if x['ifname'] == bridge)
    ports = []
    for cid, endpoint in network['Containers'].items():
        token(cid, r'[a-f0-9]{64}', 'container ID')
        # Never request Env, Cmd, mounts, or complete inspect output.
        fields = '{"pid":{{.State.Pid}},"started":{{json .State.StartedAt}},"name":{{json .Name}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"privileged":{{json .HostConfig.Privileged}},"caps":{{json .HostConfig.CapAdd}}}'
        d = json.loads(command(['docker', 'inspect', '--format', fields, cid]))
        proxy = d['name'].lstrip('/') in c['proxy_containers']
        group = d['project'] or ''
        if not proxy and group not in c['projects']:
            continue  # Unknown ports remain denied by the bridge-wide default.
        if d['privileged'] or any(x.upper().removeprefix('CAP_') in ['ALL', 'NET_ADMIN', 'SYS_ADMIN'] for x in (d['caps'] or [])):
            raise RuntimeError('unsafe container capabilities require separate review')
        if d['pid'] <= 0:
            continue
        fd = os.open('/proc/' + str(d['pid']) + '/ns/net', os.O_RDONLY)
        try:
            # Recheck identity after pinning the namespace to avoid PID reuse.
            current = command(['docker', 'inspect', '--format', '{{.State.Pid}} {{.State.StartedAt}}', cid]).strip()
            if current != str(d['pid']) + ' ' + d['started']:
                raise RuntimeError('container inventory changed; retry')
            args = ['nsenter', '--net=/proc/self/fd/' + str(fd), '--', 'ip', '-j', 'addr', 'show']
            r = subprocess.run(args, capture_output=True, text=True, pass_fds=(fd,), timeout=10)
            if r.returncode:
                raise RuntimeError('cannot resolve container bridge port')
            ip = str(ipaddress.IPv4Interface(endpoint['IPv4Address']).ip)
            link = next(x for x in json.loads(r.stdout) if any(a.get('local') == ip for a in x.get('addr_info', [])))
            peer = host_links[link['link_index']]
            if peer.get('master') not in [bridge, bridge_index]:
                raise RuntimeError('bridge port membership changed')
            ports.append({'port': peer['ifname'], 'mac': endpoint['MacAddress'], 'ip': ip,
                          'group': '__proxy' if proxy else group, 'proxy': proxy})
        finally:
            os.close(fd)
    return bridge, sorted(ports, key=lambda p: p['port'])


def policy_hash():
    r = subprocess.run(['nft', '-j', 'list', 'table', 'bridge', TABLE],
                       capture_output=True, text=True, timeout=10)
    if r.returncode:
        return None
    def stable(value):
        if isinstance(value, dict):
            return {k: stable(v) for k, v in value.items()
                    if k not in ['metainfo', 'handle', 'packets', 'bytes']}
        if isinstance(value, list):
            return [stable(x) for x in value
                    if not isinstance(x, dict) or 'metainfo' not in x]
        return value
    return hashlib.sha256(json.dumps(stable(json.loads(r.stdout)), sort_keys=True).encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['check', 'once', 'watch'])
    parser.add_argument('--config', required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise RuntimeError('root required')
    lock = os.open('/run/fleetdeck-tenant-guard.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    last = None
    applied = None
    while True:
        rules = render(*snapshot(read_config(args.config)))
        # Re-read actual policy existence so removing the owned table is repaired.
        current = policy_hash()
        if rules != last or current is None or current != applied:
            command(['nft', '--check', '-f', '-'], rules)
            if args.mode != 'check':
                command(['nft', '-f', '-'], rules)
                applied = policy_hash()
            last = rules
        if args.mode != 'watch':
            print('Bridge policy checked' if args.mode == 'check' else 'Bridge policy applied')
            return
        time.sleep(2)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        # Diagnostic values, container labels and credentials never enter journals.
        raise SystemExit('Bridge guard failed: ' + type(error).__name__)
