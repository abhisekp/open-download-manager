"""Archive the native desktop candidate, preserving Unix modes and Mac symlinks."""
import hashlib
import json
import pathlib
import platform
import subprocess
import sys
import tarfile
import zipfile

root = pathlib.Path(__file__).resolve().parents[1]
folders = list((root / 'desktop/out').iterdir())
if len(folders) != 1 or not folders[0].is_dir():
    raise RuntimeError('Expected exactly one packaged native application')
folder = folders[0]
metadata = json.loads((root / 'desktop/package.json').read_text())
hardening = json.loads((folder / 'ODM-HARDENING.json').read_text())
system = {'win32': 'windows', 'darwin': 'macos', 'linux': 'linux'}[sys.platform]
arch = {'amd64': 'x64', 'x86_64': 'x64', 'arm64': 'arm64', 'aarch64': 'arm64'}[platform.machine().lower()]
stem = f'odm-desktop-{metadata["version"]}-{system}-{arch}'
destination = root / 'dist/desktop'
destination.mkdir(parents=True, exist_ok=True)
archive = destination / (stem + ('.zip' if sys.platform == 'win32' else '.tar.gz'))
if sys.platform == 'win32':
    with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as output:
        for item in folder.rglob('*'):
            if item.is_file():
                output.write(item, stem + '/' + item.relative_to(folder).as_posix())
else:
    with tarfile.open(archive, 'w:gz', dereference=False) as output:
        output.add(folder, arcname=stem)
digest = hashlib.sha256(archive.read_bytes()).hexdigest()
(destination / 'SHA256SUMS').write_text(f'{digest}  {archive.name}\n')
(destination / 'candidate.json').write_text(json.dumps({
    'source': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip(),
    'version': metadata['version'], 'platform': system, 'arch': arch,
    'electron': metadata['devDependencies']['electron'], 'sha256': digest,
    'artifact': archive.name, 'distribution': 'development-candidate',
    'publisherSigned': False, 'notarized': False,
    'hardening': hardening,
}, indent=2) + '\n')
print('Archived native desktop candidate:', archive.name)
