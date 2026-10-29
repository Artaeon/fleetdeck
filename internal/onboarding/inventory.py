# Read-only Linux inventory. Never read container environments, credentials,
# process arguments, unit contents, firewall expressions or application files.
import json
import os
import pathlib
import re
import shutil
import subprocess

MAX_OUTPUT = 1024 * 1024

def run(argv):
    result = subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                            timeout=15, env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL": "C"})
    if result.returncode or len(result.stdout) > MAX_OUTPUT:
        raise RuntimeError("probe failed")
    return result.stdout.decode("utf-8", errors="strict")

def collect():
    report = {"schema": 1, "complete": False, "os": "", "version": "", "containers": 0,
              "services": [], "listeners": [], "firewall_rules": 0,
              "existing_paths": [], "timers": [], "unknown": []}
    def probe(name, action):
        try:
            action()
        except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, KeyError, IndexError):
            report["unknown"].append(name)

    def platform():
        if os.geteuid() != 0:
            raise RuntimeError("root required")
        fields = {}
        for line in pathlib.Path('/etc/os-release').read_text().splitlines():
            key, sep, value = line.partition('=')
            if sep and key in ('ID', 'VERSION_ID'):
                value = value.strip('"\'')
                if not re.fullmatch(r'[a-z0-9._-]{1,32}', value):
                    raise ValueError("invalid OS metadata")
                fields[key] = value
        report['os'], report['version'] = fields['ID'], fields['VERSION_ID']
        if not pathlib.Path('/run/systemd/system').is_dir():
            raise RuntimeError('systemd required')
    probe('platform', platform)

    def containers():
        if shutil.which('docker', path='/usr/bin:/usr/sbin:/bin:/sbin'):
            rows = run(['docker', 'ps', '-a', '--format', '{{.ID}}']).splitlines()
            if any(not re.fullmatch(r'[a-f0-9]{12,64}', row) for row in rows):
                raise ValueError('invalid container inventory')
            report['containers'] = len(rows)
    probe('docker', containers)

    def services():
        active = run(['systemctl', 'list-units', '--type=service', '--state=running', '--no-legend', '--plain', '--no-pager'])
        enabled = run(['systemctl', 'list-unit-files', '--type=service', '--state=enabled', '--no-legend', '--no-pager'])
        names = set()
        for line in (active + '\n' + enabled).splitlines():
            fields = line.split()
            if not fields:
                continue
            if not re.fullmatch(r'[A-Za-z0-9_.@\\:-]+\.service', fields[0]):
                raise ValueError('invalid service inventory')
            names.add(fields[0])
        report['services'] = sorted(names)
    probe('services', services)

    def listeners():
        entries = set()
        for row in run(['ss', '-H', '-lntu']).splitlines():
            fields = row.split()
            if len(fields) < 6 or fields[0] not in ('tcp', 'udp'):
                raise ValueError('invalid socket inventory')
            address, sep, port = fields[4].rpartition(':')
            if not sep or not port.isdigit() or not 0 < int(port) < 65536:
                raise ValueError('invalid socket port')
            address = address.strip('[]')
            loopback = address.startswith('127.') or address in ('::1', '::ffff:127.0.0.1')
            entries.add((fields[0], int(port), loopback))
        report['listeners'] = [dict(protocol=p, port=n, loopback=l) for p,n,l in sorted(entries)]
    probe('listeners', listeners)

    def firewall():
        found = False
        for executable in ('nft', 'iptables-save', 'ip6tables-save'):
            if not shutil.which(executable, path='/usr/sbin:/usr/bin:/sbin:/bin'):
                continue
            found = True
            if executable == 'nft':
                rules = json.loads(run(['nft', '-j', 'list', 'ruleset']))['nftables']
                # Even empty operator tables are configuration we must preserve.
                report['firewall_rules'] += sum('table' in item or 'rule' in item for item in rules)
            else:
                report['firewall_rules'] += sum(line.startswith('-A ') or
                    (line.startswith(':') and ' ACCEPT ' not in line and ' - ' not in line)
                    for line in run([executable]).splitlines())
        if not found:
            raise RuntimeError('firewall inspection tools missing')
    probe('firewall', firewall)

    def paths():
        for name in ('/opt/fleetdeck', '/opt/traefik', '/etc/fleetdeck', '/var/lib/docker',
                     '/var/lib/containers', '/etc/nginx', '/etc/caddy', '/etc/apache2'):
            path = pathlib.Path(name)
            if path.is_symlink() or (path.exists() and (not path.is_dir() or any(path.iterdir()))):
                report['existing_paths'].append(name)
    probe('existing_data', paths)

    def timers():
        names = run(['systemctl', 'list-unit-files', '--type=timer', '--state=enabled', '--no-legend', '--no-pager'])
        for row in names.splitlines():
            name = row.split()[0]
            if not re.fullmatch(r'[A-Za-z0-9_.@\\:-]+\.timer', name):
                raise ValueError('invalid timer inventory')
            report['timers'].append(name)
    probe('timers', timers)
    report['complete'] = not report['unknown']
    return report

if __name__ == '__main__':
    print(json.dumps(collect(), separators=(',', ':')))
