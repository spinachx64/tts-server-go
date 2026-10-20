#!/usr/bin/env python3
"""Offline SQLite token administration. No API or network calls."""
import argparse
import csv
import datetime as dt
import hashlib
import io
import json
import os
from pathlib import Path
import secrets
import sqlite3
import sys
import tempfile

DEFAULT_DB = '/opt/tts-server-go/data/tokens.db'
SCHEMA = '''CREATE TABLE IF NOT EXISTS tokens (
 token_hash TEXT PRIMARY KEY NOT NULL CHECK(length(token_hash)=64),
 user_id TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
 expires_at INTEGER, created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
 note TEXT NOT NULL DEFAULT '')'''


def iso(value):
    return dt.datetime.fromtimestamp(value, dt.timezone.utc).isoformat() if value is not None else None


def date_value(value):
    try:
        parsed = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
        if parsed.tzinfo is None:
            raise ValueError('include a timezone, for example +08:00')
        result = int(parsed.timestamp())
        if result <= int(dt.datetime.now(dt.timezone.utc).timestamp()):
            raise ValueError('expiry must be in the future')
        return result
    except (ValueError, OverflowError) as error:
        raise argparse.ArgumentTypeError(str(error)) from error


def positive(value):
    number = int(value)
    if number <= 0:
        raise argparse.ArgumentTypeError('must be positive')
    return number


def connect(path, initialize=False):
    path = Path(path).expanduser().resolve()
    if initialize and not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        if hasattr(os, "geteuid") and os.geteuid() == 0:
            owner = path.parent.stat()
            os.fchown(descriptor, owner.st_uid, owner.st_gid)
        os.close(descriptor)
    db = sqlite3.connect(path.as_uri() + '?mode=rw', uri=True, timeout=10)
    db.row_factory = sqlite3.Row
    return db


def initialize(db):
    # A single transaction also serializes competing schema migrations.
    db.execute('BEGIN IMMEDIATE')
    try:
        db.execute(SCHEMA)
        if 'note' not in {row[1] for row in db.execute('PRAGMA table_info(tokens)')}:
            db.execute("ALTER TABLE tokens ADD COLUMN note TEXT NOT NULL DEFAULT ''")
        db.commit()
    except Exception:
        db.rollback()
        raise


def select_rows(db, args):
    if args.id:
        prefix = args.id.lower()
        if len(prefix) < 8 or len(prefix) > 64 or any(c not in '0123456789abcdef' for c in prefix):
            raise ValueError('--id must be 8–64 hexadecimal hash characters from list')
        rows = db.execute('SELECT * FROM tokens WHERE token_hash LIKE ?', (prefix + '%',)).fetchall()
        if len(rows) > 1:
            raise ValueError('ambiguous ID; use more hash characters')
    elif args.user:
        rows = db.execute('SELECT * FROM tokens WHERE user_id=?', (args.user,)).fetchall()
    else:
        token = Path(args.token_file).read_text(encoding='utf-8').strip()
        rows = db.execute('SELECT * FROM tokens WHERE token_hash=?', (hashlib.sha256(token.encode()).hexdigest(),)).fetchall()
    if not rows:
        raise ValueError('no matching token')
    return rows


def public_row(row):
    return dict(token_id=row['token_hash'], username=row['user_id'], enabled=bool(row['enabled']),
                expires_at=iso(row['expires_at']), created_at=iso(row['created_at']), note=row['note'])


def expiry(args, current=None):
    if args.never:
        return None
    if args.expires_at is not None:
        return args.expires_at
    now = int(dt.datetime.now(dt.timezone.utc).timestamp())
    if args.command == 'renew' and current is None:
        raise ValueError('never-expiring token: use --expires-at to set an expiry explicitly')
    return max(now, current or now) + args.days * 86400


def write_batch(db, args):
    output = Path(args.out).expanduser().resolve()
    if output.suffix.lower() not in ('.json', '.csv'):
        raise ValueError('--out must end in .json or .csv')
    if output.exists():
        raise ValueError('output file exists; choose a new file (will not overwrite)')
    if args.user and (not args.user.strip() or len(args.user.encode()) > 128):
        raise ValueError('username must be 1–128 bytes')
    if args.prefix and not args.prefix.strip():
        raise ValueError('prefix must not be empty')
    expires = expiry(args)
    rows = []
    temp = None
    linked = False
    db.execute('BEGIN IMMEDIATE')
    try:
        for index in range(args.count):
            user = args.user or f'{args.prefix}{args.start + index:04d}'
            if len(user.encode()) > 128 or any(ord(c) < 32 for c in user):
                raise ValueError('username too long or contains control characters')
            token = secrets.token_urlsafe(32)
            token_id = hashlib.sha256(token.encode()).hexdigest()
            db.execute('INSERT INTO tokens(token_hash,user_id,expires_at,note) VALUES (?,?,?,?)',
                       (token_id, user, expires, args.note))
            rows.append(dict(username=user, token=token, token_id=token_id,
                             expires_at=iso(expires), note=args.note))
        if output.suffix.lower() == '.csv':
            buffer = io.StringIO(newline='')
            writer = csv.DictWriter(buffer, fieldnames=list(rows[0]))
            writer.writeheader()
            writer.writerows(rows)
            content = buffer.getvalue()
        else:
            content = json.dumps(rows, ensure_ascii=False, indent=2) + '\n'
        # Refuse overwrites even if another process creates the output mid-batch.
        fd, temp = tempfile.mkstemp(prefix='.token-batch-', dir=output.parent)
        with os.fdopen(fd, 'w', encoding='utf-8', newline='') as file:
            file.write(content)
            file.flush()
            os.fsync(file.fileno())
        os.link(temp, output)
        linked = True
        db.commit()
    except Exception:
        db.rollback()
        if linked:
            output.unlink()
        raise
    finally:
        if temp is not None:
            Path(temp).unlink(missing_ok=True)
    print(json.dumps({'created': len(rows), 'credentials_file': str(output), 'expires_at': iso(expires)}, ensure_ascii=False))


def parser():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--db', default=DEFAULT_DB)
    commands = p.add_subparsers(dest='command', required=True)
    commands.add_parser('init', help='create schema or add the note field, preserving existing rows')
    listing = commands.add_parser('list', help='list IDs, users, expiry and notes; never plaintext tokens')
    listing.add_argument('--user')
    listing.add_argument('--note', help='filter notes by substring')
    create = commands.add_parser('create', help='generate a batch and export plaintext credentials once')
    names = create.add_mutually_exclusive_group(required=True)
    names.add_argument('--user', help='same username for every token in this batch')
    names.add_argument('--prefix', help='separate usernames PREFIX0001, PREFIX0002, ...')
    create.add_argument('--count', type=positive, default=1)
    create.add_argument('--start', type=positive, default=1)
    create.add_argument('--note', default='')
    create.add_argument('--out', required=True, help='new .json or .csv file; mode 0600')
    life = create.add_mutually_exclusive_group()
    life.add_argument('--days', type=positive, default=365)
    life.add_argument('--expires-at', type=date_value)
    life.add_argument('--never', action='store_true')
    for command in ('show', 'renew', 'note', 'enable', 'disable', 'delete'):
        item = commands.add_parser(command)
        target = item.add_mutually_exclusive_group(required=True)
        target.add_argument('--id', help='full hash or unique prefix from list')
        target.add_argument('--user', help='apply to ALL tokens of this exact username')
        target.add_argument('--token-file', help='file containing one plaintext token')
        if command == 'renew':
            life = item.add_mutually_exclusive_group(required=True)
            life.add_argument('--days', type=positive, help='extend from the later of now and existing expiry')
            life.add_argument('--expires-at', type=date_value)
            life.add_argument('--never', action='store_true')
        if command == 'note':
            item.add_argument('--text', required=True, help='new note; empty string clears it')
    return p


def main(argv=None):
    p = parser()
    args = p.parse_args(argv)
    db = None
    try:
        db = connect(args.db, args.command == 'init')
        if args.command == 'init':
            initialize(db)
            print('Token table ready (note column included).')
        else:
            if 'note' not in {row[1] for row in db.execute('PRAGMA table_info(tokens)')}:
                raise ValueError('run init once to add the note column')
            if args.command == 'create':
                write_batch(db, args)
            elif args.command == 'list':
                rows = db.execute('SELECT * FROM tokens ORDER BY created_at,user_id,token_hash').fetchall()
                if args.user is not None:
                    rows = [r for r in rows if r['user_id'] == args.user]
                if args.note is not None:
                    rows = [r for r in rows if args.note in r['note']]
                print(json.dumps([public_row(r) for r in rows], ensure_ascii=False, indent=2))
            else:
                # Protect selectors and changes from concurrent writers.
                db.execute('BEGIN IMMEDIATE' if args.command != 'show' else 'BEGIN')
                rows = select_rows(db, args)
                if args.command == 'show':
                    print(json.dumps([public_row(r) for r in rows], ensure_ascii=False, indent=2))
                else:
                    for row in rows:
                        key = row['token_hash']
                        if args.command == 'renew':
                            db.execute('UPDATE tokens SET expires_at=? WHERE token_hash=?', (expiry(args, row['expires_at']), key))
                        elif args.command == 'note':
                            db.execute('UPDATE tokens SET note=? WHERE token_hash=?', (args.text, key))
                        elif args.command in ('enable', 'disable'):
                            db.execute('UPDATE tokens SET enabled=? WHERE token_hash=?', (int(args.command == 'enable'), key))
                        else:
                            db.execute('DELETE FROM tokens WHERE token_hash=?', (key,))
                    db.commit()
                    print(json.dumps({'operation': args.command, 'affected': len(rows),
                                      'token_ids': [r['token_hash'] for r in rows]}, ensure_ascii=False))
        return 0
    except (ValueError, OSError, sqlite3.Error, OverflowError) as error:
        if db is not None:
            db.rollback()
        print('Error: ' + str(error), file=sys.stderr)
        return 1
    finally:
        if db is not None:
            db.close()


if __name__ == '__main__':
    raise SystemExit(main())
