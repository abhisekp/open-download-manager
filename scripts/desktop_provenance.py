"""Bind a prepared desktop backend to exact source inputs and resource bytes."""
import argparse
import hashlib
import json
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
MANIFEST = 'ODM-BUILD.json'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def source_state(root=ROOT):
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
    dirty = bool(subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=all'], cwd=root))
    names = subprocess.check_output(['git', 'ls-files', '-co', '--exclude-standard', '-z'], cwd=root)
    paths = {name.decode('utf-8') for name in names.split(b'\0') if name}
    # Go embeds these ignored build outputs, so source revision alone cannot
    # identify the interface actually compiled into the backend.
    client = root / 'ui/dist/client'
    if not (client / 'index.html').is_file():
        raise RuntimeError('Build the UI before preparing desktop resources')
    paths.update(p.relative_to(root).as_posix() for p in client.rglob('*') if p.is_file())
    h = hashlib.sha256()
    for name in sorted(paths):
        path = root / name
        if path.is_symlink():
            raise RuntimeError('Unexpected source-input symlink')
        value = digest(path) if path.is_file() else 'deleted'
        h.update(name.encode('utf-8') + b'\0' + value.encode('ascii') + b'\0')
    return {'revision': revision, 'dirty': dirty, 'inputSha256': h.hexdigest()}


def resource_hashes(resources):
    result = {}
    shipped = {'backend', 'odm-icon.png', 'ODM-LICENSE.txt',
               'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt'}
    for path in resources.rglob('*'):
        if path.relative_to(resources).parts[0] not in shipped:
            continue
        if path.is_symlink():
            raise RuntimeError('Unexpected prepared-resource symlink')
        if path.is_file() and path.name != MANIFEST:
            result[path.relative_to(resources).as_posix()] = digest(path)
    if not result:
        raise RuntimeError('No prepared desktop resources')
    return dict(sorted(result.items()))


def write(resources, source, root=ROOT):
    if source != source_state(root):
        raise RuntimeError('Source changed during desktop preparation; build again')
    record = {'schema': 1, 'source': source, 'resources': resource_hashes(resources)}
    (resources / MANIFEST).write_text(json.dumps(record, indent=2) + '\n', encoding='utf-8')
    return record


def verify(resources, root=ROOT):
    record = json.loads((resources / MANIFEST).read_text(encoding='utf-8'))
    if record.get('schema') != 1 or record.get('source') != source_state(root):
        raise RuntimeError('Prepared backend/source inputs are stale; prepare desktop resources again')
    if record.get('resources') != resource_hashes(resources):
        raise RuntimeError('Prepared desktop resource bytes changed; prepare resources again')
    return record


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--verify', type=pathlib.Path, required=True)
    verify(parser.parse_args().verify.resolve())
    print('Prepared backend, source/UI inputs and resource hashes verified.')
