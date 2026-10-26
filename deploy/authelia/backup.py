#!/usr/bin/env python3
"""Encrypt a consistent Authelia SQLite/config snapshot; no decryption key on host."""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import subprocess
import tarfile
import tempfile
import time
import uuid


def snapshot(config, database, recipient, output):
    config, database, recipient, output = map(Path, (config, database, recipient, output))
    if config.is_symlink() or not config.is_dir() or database.is_symlink() or not database.is_file():
        raise ValueError('configuration and database must be existing real paths')
    if output.is_symlink():
        raise ValueError('backup directory must not be a symlink')
    output.mkdir(mode=0o700, parents=True, exist_ok=True)
    if output.stat().st_mode & 0o077:
        raise ValueError('backup directory must be private (0700)')
    # Operators must take this same lock before replacing configuration/keys.
    lock = os.open(config / '.backup.lock', os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(lock, fcntl.LOCK_EX)
        with tempfile.TemporaryDirectory(prefix='.snapshot-', dir=output) as temp:
            stage = Path(temp)
            (stage / 'config').mkdir(mode=0o700)
            for path in sorted(config.rglob('*')):
                if path.name == '.backup.lock':
                    continue
                if path.is_symlink() or not (path.is_file() or path.is_dir()):
                    raise ValueError('configuration must contain only regular files/directories')
                target = stage / 'config' / path.relative_to(config)
                if path.is_dir():
                    target.mkdir(mode=0o700)
                else:
                    # Private staging tree; preserve bytes, not permissive source modes.
                    target.write_bytes(path.read_bytes())
                    target.chmod(0o600)
            started = time.monotonic()
            def progress(*_):
                if time.monotonic() - started > 60:
                    raise TimeoutError('database snapshot exceeded its deadline')
            source = sqlite3.connect(database.resolve().as_uri() + '?mode=ro', uri=True, timeout=5)
            destination = sqlite3.connect(stage / 'db.sqlite3')
            try:
                source.backup(destination, pages=128, progress=progress, sleep=0.1)
                if destination.execute('PRAGMA journal_mode=DELETE').fetchone() != ('delete',):
                    raise ValueError('snapshot must be a standalone SQLite database')
                if destination.execute('PRAGMA integrity_check').fetchone() != ('ok',):
                    raise ValueError('database integrity check failed')
            finally:
                destination.close()
                source.close()
            manifest = {'format': 1, 'created_unix': int(time.time()), 'files': {}}
            for path in [stage / 'db.sqlite3', *sorted((stage / 'config').rglob('*'))]:
                if path.is_file():
                    manifest['files'][str(path.relative_to(stage))] = hashlib.sha256(path.read_bytes()).hexdigest()
            (stage / 'manifest.json').write_text(json.dumps(manifest, sort_keys=True) + '\n')
            partial = stage / 'encrypted.age'
            proc = subprocess.Popen(['age', '-R', str(recipient), '-o', str(partial)], stdin=subprocess.PIPE, stderr=subprocess.PIPE)
            try:
                with tarfile.open(fileobj=proc.stdin, mode='w|gz') as archive:
                    for name in ['config', 'db.sqlite3', 'manifest.json']:
                        archive.add(stage / name, arcname=name)
                proc.stdin.close()
                if proc.wait(timeout=60):
                    raise RuntimeError('backup encryption failed')
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.wait()
                proc.stderr.close()
            partial.chmod(0o600)
            with partial.open('rb') as handle:
                os.fsync(handle.fileno())
            final = output / (time.strftime('%Y%m%dT%H%M%SZ', time.gmtime()) + '-' + uuid.uuid4().hex[:12] + '.tar.gz.age')
            os.rename(partial, final)
            directory = os.open(output, os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
            return final
    finally:
        os.close(lock)


if __name__ == '__main__':
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ['config', 'database', 'recipient', 'output']:
        parser.add_argument('--' + flag, required=True)
    args = parser.parse_args()
    try:
        result = snapshot(args.config, args.database, args.recipient, args.output)
    except Exception:
        raise SystemExit('Authelia backup failed; no partial archive published. Check paths, permissions, age recipient and SQLite health.')
    print('Encrypted Authelia snapshot created: ' + result.name)
