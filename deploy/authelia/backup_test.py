import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import sqlite3
import subprocess
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('backup', Path(__file__).with_name('backup.py'))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


@unittest.skipUnless(shutil.which('age') and shutil.which('age-keygen'), 'age tools required')
class BackupTest(unittest.TestCase):
    def test_wal_config_roundtrip_and_failure_preserve_existing_backup(self):
        with tempfile.TemporaryDirectory() as root:
            p = Path(root)
            config = p / 'config'
            config.mkdir(mode=0o700)
            (config / 'storage.key').write_text(os.urandom(32).hex())
            key = subprocess.run(['age-keygen'], capture_output=True, check=True).stdout
            recipient = subprocess.run(['age-keygen', '-y'], input=key, capture_output=True, check=True).stdout
            (p / 'recipient').write_bytes(recipient)
            db = sqlite3.connect(p / 'live.sqlite3')
            db.execute('PRAGMA journal_mode=WAL')
            db.execute('CREATE TABLE proof(value TEXT)')
            db.execute("INSERT INTO proof VALUES ('committed')")
            db.commit()
            output = backup.snapshot(config, p / 'live.sqlite3', p / 'recipient', p / 'backups')
            db.execute("INSERT INTO proof VALUES ('uncommitted')")
            raw = subprocess.run(['age', '-d', '-i', '-', str(output)], input=key, capture_output=True, check=True).stdout
            with tarfile.open(fileobj=io.BytesIO(raw), mode='r:gz') as archive:
                manifest = json.load(archive.extractfile('manifest.json'))
                for name, digest in manifest['files'].items():
                    data = archive.extractfile(name).read()
                    self.assertEqual(hashlib.sha256(data).hexdigest(), digest)
                (p / 'restored.sqlite3').write_bytes(archive.extractfile('db.sqlite3').read())
                self.assertEqual(archive.extractfile('config/storage.key').read(), (config / 'storage.key').read_bytes())
            with sqlite3.connect(p / 'restored.sqlite3') as restored:
                self.assertEqual(restored.execute('SELECT value FROM proof').fetchall(), [('committed',)])
                self.assertEqual(restored.execute('PRAGMA integrity_check').fetchone(), ('ok',))
            db.close()
            (config / 'unsafe').symlink_to(p / 'recipient')
            with self.assertRaises(ValueError):
                backup.snapshot(config, p / 'live.sqlite3', p / 'recipient', p / 'backups')
            self.assertEqual(list((p / 'backups').iterdir()), [output])
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            (config / 'unsafe').unlink()
            (p / 'recipient').write_text('invalid recipient')
            with self.assertRaises((RuntimeError, BrokenPipeError)):
                backup.snapshot(config, p / 'live.sqlite3', p / 'recipient', p / 'backups')
            self.assertEqual(list((p / 'backups').iterdir()), [output])


if __name__ == '__main__':
    unittest.main()
