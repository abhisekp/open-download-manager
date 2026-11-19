"""One application version, with explicit formats for supported package systems."""
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[1]


def formats(source):
    match = re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([a-z][a-z0-9]*(?:\.[a-z0-9]+)*))?', source)
    if not match or any(int(value) > 65535 for value in match.groups()[:3]):
        raise ValueError('VERSION must be a supported numeric major.minor.patch with optional lowercase prerelease')
    if match[4] and any(part.isdigit() and len(part) > 1 and part.startswith('0') for part in match[4].split('.')):
        raise ValueError('Numeric prerelease identifiers cannot have leading zeroes')
    core = '.'.join(match.groups()[:3])
    return {'source': source, 'core': core, 'browser': core, 'msix': core + '.0',
            'debian': core + ('~' + match[4] if match[4] else '')}


VERSIONS = formats((ROOT / 'VERSION').read_text().strip())
VERSION = VERSIONS['source']


def check_desktop():
    manifest = json.loads((ROOT / 'desktop/package.json').read_text())
    lock = json.loads((ROOT / 'desktop/package-lock.json').read_text())
    if manifest['version'] != VERSION or lock['version'] != VERSION or lock['packages']['']['version'] != VERSION:
        raise ValueError('Desktop manifests and lockfile must match VERSION')


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--json', action='store_true')
    parser.add_argument('--check', action='store_true')
    parser.add_argument('--format', choices=VERSIONS, default='source')
    args = parser.parse_args()
    if args.check:
        check_desktop()
    print(json.dumps(VERSIONS) if args.json else VERSIONS[args.format])
