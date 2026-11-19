import unittest
from version import formats, check_desktop


class VersionTests(unittest.TestCase):
    def test_prerelease_is_older_than_stable_in_debian_and_numeric_elsewhere(self):
        self.assertEqual(formats('1.2.3-dev.2'), {'source':'1.2.3-dev.2', 'core':'1.2.3', 'browser':'1.2.3', 'msix':'1.2.3.0', 'debian':'1.2.3~dev.2'})
        self.assertEqual(formats('1.2.3')['debian'], '1.2.3')

    def test_invalid_or_unrepresentable_versions_fail_before_packaging(self):
        for value in ['v1.2.3', '1.2', '01.2.3', '1.2.3+build', '65536.0.0', '1.2.3\nmalicious', '../1.2.3', '1.2.3-', '1.2.3-dev.01']:
            with self.assertRaises(ValueError, msg=value):
                formats(value)

    def test_desktop_manifest_and_lock_are_synchronized(self):
        check_desktop()
