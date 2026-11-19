"""A valid repository signature alone must never authorize another source revision."""
import json
import subprocess
import unittest
from unittest.mock import patch

from verify_provenance import verify


class ProvenancePolicyTests(unittest.TestCase):
    @patch('verify_provenance.subprocess.check_output')
    def test_rejects_different_source_after_signature_verification(self, cli):
        cli.return_value = json.dumps([{'verificationResult': {'statement': {
            'predicate': {'buildDefinition': {'resolvedDependencies': [{'digest': {'gitCommit': 'other'}}]}}
        }}}])
        with self.assertRaisesRegex(ValueError, 'approved source'):
            verify('artifact.zip', 'owner/repo', 'approved')

    @patch('verify_provenance.subprocess.check_output', return_value='[]')
    def test_rejects_empty_verification(self, cli):
        with self.assertRaises(ValueError):
            verify('artifact.zip', 'owner/repo', 'approved')

    @patch('verify_provenance.subprocess.check_output', side_effect=subprocess.CalledProcessError(1, 'gh'))
    def test_never_accepts_failed_signature_verification(self, cli):
        with self.assertRaises(subprocess.CalledProcessError):
            verify('artifact.zip', 'owner/repo', 'approved')


if __name__ == '__main__':
    unittest.main()
