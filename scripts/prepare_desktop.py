"""Prepare one native desktop backend; npm installation is a separate explicit step."""
import os
import pathlib
import shutil
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
resources = root / 'desktop' / 'resources'
(resources / 'backend').mkdir(parents=True, exist_ok=True)
name = 'odm.exe' if os.name == 'nt' else 'odm'
subprocess.run(['go', 'build', '-trimpath', '-ldflags=-s -w', '-o', str(resources / 'backend' / name), '.'], cwd=root, check=True)
shutil.copy2(root / 'assets/branding/odm-icon.png', resources / 'odm-icon.png')
shutil.copy2(root / 'assets/branding/odm.ico', resources / 'odm.ico')
if sys.platform == 'darwin':
    iconset = resources / 'odm.iconset'
    iconset.mkdir(exist_ok=True)
    for size in [16, 32, 128, 256, 512]:
        for scale in [1, 2]:
            suffix = '@2x' if scale == 2 else ''
            subprocess.run(['sips', '-z', str(size * scale), str(size * scale), str(resources / 'odm-icon.png'),
                            '--out', str(iconset / f'icon_{size}x{size}{suffix}.png')], check=True, stdout=subprocess.DEVNULL)
    subprocess.run(['iconutil', '-c', 'icns', str(iconset), '-o', str(resources / 'odm.icns')], check=True)
for name in ['LICENSE', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt']:
    shutil.copy2(root / name, resources / ('ODM-LICENSE.txt' if name == 'LICENSE' else name))
print('Prepared native desktop resources; no signing or publication performed.')
