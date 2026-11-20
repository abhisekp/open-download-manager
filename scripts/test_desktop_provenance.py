import pathlib
import tempfile
import unittest
from unittest.mock import patch
import desktop_provenance as provenance


class PreparedBackendTests(unittest.TestCase):
    def test_same_version_backend_replacement_and_changed_ui_are_rejected(self):
        with tempfile.TemporaryDirectory() as temp:
            resources = pathlib.Path(temp)
            backend = resources / 'backend'
            backend.mkdir()
            binary = backend / 'odm'
            binary.write_bytes(b'original backend with version 0.2.0-dev')
            original = {'revision': 'a' * 40, 'dirty': False, 'inputSha256': 'b' * 64}
            with patch.object(provenance, 'source_state', return_value=original):
                provenance.write(resources, original)
                self.assertEqual(provenance.verify(resources)['source'], original)
                binary.write_bytes(b'other backend with version 0.2.0-dev')
                with self.assertRaisesRegex(RuntimeError, 'resource bytes changed'):
                    provenance.verify(resources)
                binary.write_bytes(b'original backend with version 0.2.0-dev')
            with patch.object(provenance, 'source_state', return_value={**original, 'inputSha256': 'c' * 64}):
                with self.assertRaisesRegex(RuntimeError, 'stale'):
                    provenance.verify(resources)
            # A dirty local artifact must retain that distinction even if its
            # source revision happens to equal a reviewed clean CI build.
            with patch.object(provenance, 'source_state', return_value={**original, 'dirty': True}):
                with self.assertRaisesRegex(RuntimeError, 'stale'):
                    provenance.verify(resources)

    def test_concurrent_source_change_cannot_be_labeled_as_prepared(self):
        with tempfile.TemporaryDirectory() as temp:
            resources = pathlib.Path(temp)
            with patch.object(provenance, 'source_state', return_value={'revision': 'new'}):
                with self.assertRaisesRegex(RuntimeError, 'changed during'):
                    provenance.write(resources, {'revision': 'old'})
            self.assertFalse((resources / provenance.MANIFEST).exists())
