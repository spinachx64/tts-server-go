import datetime as dt
import hashlib
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
TOOL = ROOT / 'tools/token_admin.py'


class TokenAdminTests(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.TemporaryDirectory()
        self.addCleanup(self.folder.cleanup)
        self.root = Path(self.folder.name)
        self.db = self.root / 'tokens.db'
        self.call('init')

    def call(self, *args, ok=True):
        result = subprocess.run([sys.executable, str(TOOL), '--db', str(self.db), *args], capture_output=True, text=True)
        if ok:
            self.assertEqual(result.returncode, 0, result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0)
        return result

    def records(self):
        return json.loads(self.call('list').stdout)

    def test_legacy_migration_is_idempotent_and_preserves_rows(self):
        self.db.unlink()
        with sqlite3.connect(self.db) as db:
            db.execute('CREATE TABLE tokens(token_hash TEXT PRIMARY KEY,user_id TEXT,enabled INTEGER,expires_at INTEGER,created_at INTEGER)')
            db.execute('INSERT INTO tokens VALUES (?,?,?,?,?)', ('a' * 64, 'legacy', 1, 4916535281, 100))
        self.call('init'); self.call('init')
        row = self.records()[0]
        self.assertEqual((row['username'], row['note'], row['enabled']), ('legacy', '', True))
        self.assertTrue(row['expires_at'].startswith('2125-10-19'))

    def test_batch_generation_renew_notes_and_revocation(self):
        output = self.root / 'credentials.json'
        result = self.call('create', '--prefix', 'member-', '--count', '3', '--days', '30', '--note', '测试批次', '--out', str(output))
        credentials = json.loads(output.read_text())
        self.assertEqual(len(credentials), 3)
        self.assertEqual(len({r['token'] for r in credentials}), 3)
        self.assertEqual({r['username'] for r in credentials}, {'member-0001', 'member-0002', 'member-0003'})
        self.assertEqual(output.stat().st_mode & 0o777, 0o600)
        rows = self.records()
        self.assertEqual(len(rows), 3)
        for credential in credentials:
            self.assertNotIn(credential['token'], result.stdout)
            self.assertNotIn(credential['token'], self.call('list').stdout)
            self.assertEqual(credential['token_id'], hashlib.sha256(credential['token'].encode()).hexdigest())
        target = rows[0]
        old = dt.datetime.fromisoformat(target['expires_at'])
        self.call('renew', '--id', target['token_id'][:12], '--days', '10')
        fresh = json.loads(self.call('show', '--id', target['token_id']).stdout)[0]
        self.assertEqual(dt.datetime.fromisoformat(fresh['expires_at']) - old, dt.timedelta(days=10))
        self.call('note', '--user', target['username'], '--text', '续期用户')
        self.assertEqual(json.loads(self.call('list', '--note', '续期').stdout)[0]['note'], '续期用户')
        self.call('disable', '--id', target['token_id'])
        self.assertFalse(json.loads(self.call('show', '--user', target['username']).stdout)[0]['enabled'])
        self.call('enable', '--id', target['token_id'])
        self.call('delete', '--id', target['token_id'])
        self.assertEqual(len(self.records()), 2)

    def test_existing_export_not_overwritten_and_failure_rolls_back(self):
        output = self.root / 'credentials.csv'
        self.call('create', '--user', 'alice', '--count', '2', '--out', str(output))
        original = output.read_bytes()
        self.call('create', '--user', 'bob', '--out', str(output), ok=False)
        self.assertEqual(output.read_bytes(), original)
        self.call('create', '--user', 'bob', '--out', str(self.root / 'missing' / 'export.json'), ok=False)
        self.assertEqual(len(self.records()), 2)
        self.call('renew', '--user', 'alice', '--expires-at', '2020-01-01T00:00:00Z', ok=False)
        self.assertEqual(len(self.records()), 2)
        self.call('renew', '--user', 'alice', '--days', '1')
        self.call('delete', '--user', 'alice')
        self.assertEqual(self.records(), [])

    def test_ambiguous_id_cannot_delete_multiple_tokens(self):
        with sqlite3.connect(self.db) as db:
            db.execute('INSERT INTO tokens(token_hash,user_id) VALUES (?,?)', ('a' * 64, 'alice'))
            db.execute('INSERT INTO tokens(token_hash,user_id) VALUES (?,?)', ('a' * 8 + 'b' * 56, 'bob'))
        self.call('delete', '--id', 'aaaaaaaa', ok=False)
        self.assertEqual(len(self.records()), 2)

    def test_expired_renewal_and_no_expiry(self):
        output = self.root / 'tokens.json'
        self.call('create', '--user', 'alice', '--never', '--out', str(output))
        self.assertIsNone(self.records()[0]['expires_at'])
        self.call('renew', '--user', 'alice', '--days', '1', ok=False)
        with sqlite3.connect(self.db) as db:
            db.execute('UPDATE tokens SET expires_at=1')
        before = dt.datetime.now(dt.timezone.utc)
        self.call('renew', '--user', 'alice', '--days', '2')
        after = dt.datetime.fromisoformat(self.records()[0]['expires_at'])
        self.assertGreaterEqual(after, before + dt.timedelta(days=2, seconds=-2))
        self.assertLess(after, before + dt.timedelta(days=2, seconds=5))


if __name__ == '__main__':
    unittest.main()
