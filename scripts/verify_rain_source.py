"""Reject unreviewed drift in the attributed local torrent-runtime source."""
import hashlib
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[1] / 'internal/raincompat'


def verify():
    upstream = json.loads((ROOT / 'UPSTREAM-FILES.json').read_text(encoding='utf-8'))
    patches = json.loads((ROOT / 'ODM-PATCH-HASHES.json').read_text(encoding='utf-8'))
    if upstream['version'] != 'v2.5.1' or upstream['module'] != 'github.com/cenkalti/rain/v2':
        raise ValueError('Unexpected torrent-runtime baseline')
    expected = {**upstream['files'], **patches}
    allowed = set(expected) | {'UPSTREAM-FILES.json', 'ODM-PATCH-HASHES.json', 'ODM-PATCHES.md'}
    actual = {p.relative_to(ROOT).as_posix() for p in ROOT.rglob('*') if p.is_file()}
    if actual != allowed:
        raise ValueError('Unexpected or missing compatibility source files')
    for name, digest in expected.items():
        path = pathlib.PurePosixPath(name)
        if path.is_absolute() or '..' in path.parts or (ROOT / name).is_symlink():
            raise ValueError('Unsafe compatibility source entry')
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != digest:
            raise ValueError('Torrent-runtime source differs from reviewed hashes: ' + name)
    print('Verified exact upstream runtime subset and reviewed local patch hashes:', len(expected), 'files.')


if __name__ == '__main__':
    verify()
