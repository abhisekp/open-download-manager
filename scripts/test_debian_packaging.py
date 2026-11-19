"""Regression: dpkg requires archive directory entries before nested files."""
import io
import tarfile
import unittest

from linux_packages import tar


class DebianPackagingTests(unittest.TestCase):
    def test_directories_precede_files_and_are_searchable(self):
        body = tar([('usr/share/doc/odm/copyright', b'MIT', 0o644),
                    ('usr/bin/odm', b'fixture', 0o755)])
        with tarfile.open(fileobj=io.BytesIO(body)) as archive:
            members = archive.getmembers()
            indexes = {member.name: index for index, member in enumerate(members)}
            for name in ['usr', 'usr/share', 'usr/share/doc', 'usr/share/doc/odm', 'usr/bin']:
                self.assertTrue(archive.getmember(name).isdir())
                self.assertEqual(archive.getmember(name).mode, 0o755)
            self.assertLess(indexes['usr/share/doc/odm'], indexes['usr/share/doc/odm/copyright'])
            self.assertEqual(archive.extractfile('usr/share/doc/odm/copyright').read(), b'MIT')


if __name__ == '__main__':
    unittest.main()
