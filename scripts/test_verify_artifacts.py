"""Exercise package rejection paths without building application binaries."""
import hashlib
import pathlib
import tempfile
import unittest
import zipfile

from verify_artifacts import verify


class PackageVerificationTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.dist = pathlib.Path(self.temp.name)
        for arch in ['x86', 'x64', 'arm64']:
            with zipfile.ZipFile(self.dist / f'odm-0.1.0.0-{arch}-unsigned.msix', 'w') as archive:
                archive.writestr('AppxManifest.xml', f'<Package><Identity ProcessorArchitecture="{arch}"/></Package>')
                for name in ['odm.exe', 'LICENSE', 'THIRD-PARTY-GO-NOTICES.txt']:
                    archive.writestr(name, 'test fixture')
        self.checksum()

    def checksum(self):
        (self.dist / 'SHA256SUMS').write_text(''.join(
            hashlib.sha256(path.read_bytes()).hexdigest() + '  ' + path.name + '\n'
            for path in sorted(self.dist.glob('*.msix'))))

    def test_complete_packages(self):
        verify(self.dist, 'msix')

    def test_tampered_checksum(self):
        sums = self.dist / 'SHA256SUMS'
        sums.write_text(sums.read_text().replace(sums.read_text()[:64], '0' * 64))
        with self.assertRaisesRegex(ValueError, 'checksum'):
            verify(self.dist, 'msix')

    def test_missing_architecture(self):
        next(self.dist.glob('*arm64*.msix')).unlink()
        with self.assertRaises(FileNotFoundError):
            verify(self.dist, 'msix')

    def test_wrong_architecture(self):
        path = next(self.dist.glob('*x86*.msix'))
        with zipfile.ZipFile(path, 'w') as archive:
            archive.writestr('AppxManifest.xml', '<Package><Identity ProcessorArchitecture="arm64"/></Package>')
        self.checksum()
        with self.assertRaisesRegex(ValueError, 'architecture'):
            verify(self.dist, 'msix')

    def test_unsafe_checksum_path(self):
        (self.dist / 'SHA256SUMS').write_text('0' * 64 + '  ../outside\n')
        with self.assertRaisesRegex(ValueError, 'Unsafe'):
            verify(self.dist, 'msix')


if __name__ == '__main__':
    unittest.main()
