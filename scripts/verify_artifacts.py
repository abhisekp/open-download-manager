"""Fail closed on incomplete packages, wrong architectures or corrupt checksums."""
import argparse
import hashlib
import pathlib
import tarfile
import xml.etree.ElementTree as ET
import zipfile


def verify(dist, kind):
    sums = {}
    for line in (dist / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        if pathlib.Path(name).name != name or name in sums:
            raise ValueError('Unsafe or duplicate checksum entry')
        sums[name] = digest
    expected = []
    if kind == 'portable':
        for system, arches, suffix in [('windows', ['386', 'amd64', 'arm64'], '.zip'),
                                        ('linux', ['386', 'amd64', 'arm', 'arm64'], '.tar.gz')]:
            for arch in arches:
                name = f'odm-0.1.0-dev-{system}-{arch}{suffix}'
                expected.append(name)
                if suffix == '.zip':
                    with zipfile.ZipFile(dist / name) as archive:
                        if archive.testzip():
                            raise ValueError(f'Corrupt archive: {name}')
                        members = archive.namelist()
                else:
                    with tarfile.open(dist / name) as archive:
                        members = archive.getnames()
                for required in ['LICENSE', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt',
                                 'odm.exe' if system == 'windows' else 'odm']:
                    if not any(member.endswith('/' + required) for member in members):
                        raise ValueError(f'{name} missing {required}')
        expected += [f'open-download-manager_0.1.0-dev_{arch}.deb' for arch in ['i386', 'amd64', 'armhf', 'arm64']]
        expected += [f'odm-extension-{browser}-0.1.0.zip' for browser in ['chromium', 'firefox', 'safari-source']]
    else:
        for arch in ['x86', 'x64', 'arm64']:
            name = f'odm-0.1.0.0-{arch}-unsigned.msix'
            expected.append(name)
            with zipfile.ZipFile(dist / name) as archive:
                if archive.testzip():
                    raise ValueError(f'Corrupt MSIX: {name}')
                manifest = ET.fromstring(archive.read('AppxManifest.xml'))
                identity = manifest.find('{*}Identity')
                if identity is None or identity.get('ProcessorArchitecture') != arch:
                    raise ValueError(f'Wrong MSIX architecture: {name}')
                for required in ['odm.exe', 'LICENSE', 'THIRD-PARTY-GO-NOTICES.txt']:
                    archive.getinfo(required)
    for name in expected:
        actual = hashlib.sha256((dist / name).read_bytes()).hexdigest()
        if sums.get(name) != actual:
            raise ValueError(f'Missing or incorrect checksum: {name}')
    print(f'Verified {len(expected)} {kind} packages and SHA-256 checksums')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--kind', choices=['portable', 'msix'], required=True)
    parser.add_argument('--dist', type=pathlib.Path, default=pathlib.Path(__file__).resolve().parents[1] / 'dist')
    args = parser.parse_args()
    verify(args.dist, args.kind)
