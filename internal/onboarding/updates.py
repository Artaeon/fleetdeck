# Operator-selected OS security updates. No container updates or automatic reboot.
import base64
import datetime
import fcntl
import hashlib
import json
import os
import pathlib
import re
import subprocess
import tempfile

POLICY = pathlib.Path('/etc/apt/apt.conf.d/99zz-fleetdeck-security')
TIMER = pathlib.Path('/etc/systemd/system/apt-daily-upgrade.timer.d/90-fleetdeck.conf')
STATE = pathlib.Path('/var/lib/fleetdeck/host-updates')

def run(args, timeout=60):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout,
                            env=dict(os.environ, LC_ALL='C', DEBIAN_FRONTEND='noninteractive'))
    if result.returncode:
        raise RuntimeError('command failed')
    return result.stdout.decode()

def require_real_parents(path):
    for parent in [path, *path.parents]:
        if parent.is_symlink():
            raise RuntimeError('linked managed path')
        if parent.exists() and (parent.stat().st_mode & 0o022 or parent.stat().st_uid != 0):
            raise RuntimeError('managed path writable by other users')

def put(path, data, mode=0o644):
    require_real_parents(path)
    path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.fleetdeck-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as f:
            f.write(data); f.flush(); os.fsync(f.fileno()); os.fchmod(f.fileno(), mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary): os.unlink(temporary)

def enabled(name):
    result = subprocess.run(['systemctl','is-enabled',name],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
    value = result.stdout.decode().strip()
    if value not in ('enabled','disabled'):
        raise RuntimeError('timer is masked or has an unfamiliar enablement state')
    return value=='enabled'

def active(name):
    result = subprocess.run(['systemctl','is-active',name],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
    value=result.stdout.decode().strip()
    if value not in ('active','inactive'):
        raise RuntimeError('timer activity is unknown')
    return value=='active'

def policy_for(distro, version):
    code = {('ubuntu','24.04'):'noble',('debian','12'):'bookworm',('debian','13'):'trixie'}.get((distro,version))
    if not code: raise RuntimeError('unsupported OS')
    origin = ('origin=Ubuntu,archive='+code+'-security') if distro=='ubuntu' else ('origin=Debian,label=Debian-Security,codename='+code+'-security')
    content = '''// Managed by FleetDeck. Explicit OS security updates only.
#clear Unattended-Upgrade::Allowed-Origins;
#clear Unattended-Upgrade::Origins-Pattern;
Unattended-Upgrade::Origins-Pattern { "%s"; };
Unattended-Upgrade::Automatic-Reboot "false";
Unattended-Upgrade::Remove-Unused-Kernel-Packages "false";
Unattended-Upgrade::Remove-New-Unused-Dependencies "false";
Unattended-Upgrade::Remove-Unused-Dependencies "false";
Unattended-Upgrade::MinimalSteps "true";
APT::Periodic::Enable "1";
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
''' % origin
    return origin,content.encode()

def verify_policy(origin):
    import apt_pkg
    apt_pkg.config.clear('')
    apt_pkg.init_config()
    c=apt_pkg.config
    if c.value_list('Unattended-Upgrade::Allowed-Origins') or c.value_list('Unattended-Upgrade::Origins-Pattern')!=[origin]:
        raise RuntimeError('another configuration overrides the allowed security origins')
    for key in ('Automatic-Reboot','Remove-Unused-Kernel-Packages','Remove-New-Unused-Dependencies','Remove-Unused-Dependencies'):
        if c.find_b('Unattended-Upgrade::'+key,True): raise RuntimeError('unsafe effective update policy')
    for key in ('Enable','Update-Package-Lists','Unattended-Upgrade'):
        if c.find_i('APT::Periodic::'+key,0)!=1: raise RuntimeError('periodic updates not enabled')

def restore(snapshot):
    # Called only with the private snapshot made by this tool.
    run(['systemctl','stop','apt-daily-upgrade.timer'])
    for name in (str(POLICY),str(TIMER)):
        previous=snapshot['files'][name]
        path=pathlib.Path(name)
        if previous is None:
            require_real_parents(path); path.unlink(missing_ok=True)
        else:
            put(path,base64.b64decode(previous['data']),previous['mode'])
    run(['systemctl','daemon-reload'])
    for name,states in snapshot['timers'].items():
        run(['systemctl','enable' if states['enabled'] else 'disable',name])
        run(['systemctl','start' if states['active'] else 'stop',name])

def main():
    if os.geteuid()!=0: raise RuntimeError('root required')
    values={}
    for line in pathlib.Path('/etc/os-release').read_text().splitlines():
        key,sep,value=line.partition('=')
        if sep and key in ('ID','VERSION_ID'): values[key]=value.strip('"\'')
    origin,content=policy_for(values.get('ID'),values.get('VERSION_ID'))
    mode,window=options['mode'],options['window']
    if not re.fullmatch(r'(?:[01][0-9]|2[0-3]):[0-5][0-9]',window): raise RuntimeError('invalid window')
    report={'os':values['ID'],'version':values['VERSION_ID'],'origin':origin,
            'window_utc':window,'automatic_reboot':False,'service_restarts_possible':True,
            'third_party_repositories':False,'application_images':False,
            'managed':(STATE/'current.json').exists(),'operation':mode}
    if mode=='plan':
        report['effective_policy'] = 'unmanaged'
        if report['managed']:
            try:
                verify_policy(origin)
                report['effective_policy'] = 'checked'
            except Exception:
                report['effective_policy'] = 'unknown_or_changed'
        print(json.dumps(report)); return
    if mode not in ('enable','disable'): raise RuntimeError('invalid operation')
    require_real_parents(STATE); STATE.mkdir(mode=0o700,parents=True,exist_ok=True)
    with (STATE/'lock').open('a') as lock:
        os.chmod(STATE/'lock',0o600); fcntl.flock(lock,fcntl.LOCK_EX)
        current=STATE/'current.json'
        if mode=='disable':
            require_real_parents(current)
            snapshot=json.loads(current.read_text())
            for name,digest in snapshot['installed'].items():
                require_real_parents(pathlib.Path(name))
                if hashlib.sha256(pathlib.Path(name).read_bytes()).hexdigest()!=digest:
                    raise RuntimeError('operator changed managed policy; refusing to overwrite it')
            restore(snapshot)
            current.unlink()
            report['managed']=False
            print(json.dumps(report)); return
        if current.exists(): raise RuntimeError('already managed; disable to restore original policy before changing the window')
        # Never overwrite unrelated files with the names reserved for this feature.
        for path in (POLICY,TIMER):
            require_real_parents(path)
            if path.exists(): raise RuntimeError('managed filename already exists without a recovery record')
        run(['dpkg-query','-W','-f=${Status}','unattended-upgrades'])
        import apt_pkg  # Verify this dependency BEFORE writes.
        if run(['dpkg','--audit']).strip(): raise RuntimeError('package state needs repair')
        if subprocess.run(['systemctl','is-active','--quiet','apt-daily-upgrade.service']).returncode==0:
            raise RuntimeError('package maintenance is already running')
        timer=('''# Managed by FleetDeck. Missed windows wait until the next scheduled window.
[Timer]
OnCalendar=
OnCalendar=*-*-* %s:00 UTC
RandomizedDelaySec=15m
Persistent=false
''' % window).encode()
        snapshot={'files':{str(POLICY):None,str(TIMER):None},'timers':{},'installed':{
            str(POLICY):hashlib.sha256(content).hexdigest(),str(TIMER):hashlib.sha256(timer).hexdigest()}}
        for name in ('apt-daily.timer','apt-daily-upgrade.timer'):
            snapshot['timers'][name]={'enabled':enabled(name),'active':active(name)}
        recovery=STATE/(datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')+'.json')
        put(recovery,json.dumps(snapshot).encode(),0o600)
        try:
            run(['systemctl','stop','apt-daily-upgrade.timer'])
            put(POLICY,content); put(TIMER,timer)
            verify_policy(origin)
            # A real dry run resolves packages but cannot install them. Keep logs private.
            with (STATE/'dry-run.log').open('wb') as log:
                os.chmod(STATE/'dry-run.log',0o600)
                result=subprocess.run(['unattended-upgrade','--dry-run'],stdout=log,stderr=log,timeout=300)
                if result.returncode: raise RuntimeError('security-upgrade dry run failed')
            run(['systemctl','daemon-reload'])
            run(['systemctl','enable','--now','apt-daily.timer','apt-daily-upgrade.timer'])
            if not all(active(name) for name in snapshot['timers']): raise RuntimeError('timers not active')
            put(current,json.dumps(snapshot).encode(),0o600)
        except Exception:
            restore(snapshot)
            raise
        report['managed']=True;report['recovery_record']=str(recovery);report['dry_run']='passed'
        print(json.dumps(report))

if __name__ == "__main__":
    try:
        main()
    except Exception:
        # Do not reflect private command diagnostics or APT configuration.
        raise SystemExit(1)
