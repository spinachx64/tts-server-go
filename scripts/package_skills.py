#!/usr/bin/env python3
"""Build deterministic, self-contained macOS/Windows skill downloads from shared source."""
import argparse
import io
from pathlib import Path
import zipfile

ROOT = Path(__file__).resolve().parents[1]


def archive(platform):
    name = 'tts-cookup-' + platform
    source = 'macos' if platform == 'macos' else 'windows'
    files = {'SKILL.md': ROOT / 'skills' / source / 'SKILL.md',
             'scripts/tts_client.py': ROOT / 'skills/common/tts_client.py',
             'references/parameters.md': ROOT / 'skills/common/parameters.md'}
    output = io.BytesIO()
    with zipfile.ZipFile(output, 'w', compression=zipfile.ZIP_STORED) as z:
        for relative, path in sorted(files.items()):
            info = zipfile.ZipInfo(name + '/' + relative, date_time=(2020, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.compress_type = zipfile.ZIP_STORED
            info.external_attr = (0o100644 << 16)
            z.writestr(info, path.read_text(encoding='utf-8').encode('utf-8'))
    return output.getvalue()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--check', action='store_true', help='fail when committed downloads are stale')
    args = p.parse_args()
    folder = ROOT / 'server/public/downloads'
    folder.mkdir(exist_ok=True)
    for platform in ('macos', 'windows'):
        path = folder / ('tts-cookup-' + platform + '.zip')
        data = archive(platform)
        if args.check:
            if not path.exists() or path.read_bytes() != data:
                raise SystemExit('Outdated skill download: run python3 scripts/package_skills.py')
        else:
            path.write_bytes(data)
        print(('Checked: ' if args.check else 'Built: ') + path.name)


if __name__ == '__main__':
    main()
