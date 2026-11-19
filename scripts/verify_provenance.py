"""Verify GitHub/Sigstore identity, artifact digest and exact source commit."""
import argparse
import json
import subprocess


def verify(artifact, repository, source, workflow='release.yml'):
    output = subprocess.check_output([
        'gh', 'attestation', 'verify', artifact, '--repo', repository,
        '--signer-workflow', repository + '/.github/workflows/' + workflow,
        '--deny-self-hosted-runners', '--format', 'json'], text=True, encoding='utf-8')
    records = json.loads(output)
    for record in records:
        statement = record.get('verificationResult', {}).get('statement', {})
        dependencies = statement.get('predicate', {}).get('buildDefinition', {}).get('resolvedDependencies', [])
        if any(item.get('digest', {}).get('gitCommit') == source for item in dependencies):
            print('PASS: verified provenance for exact source:', artifact)
            return
    raise ValueError('No verified provenance matches the approved source commit')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('artifact')
    parser.add_argument('--repo', required=True)
    parser.add_argument('--source', required=True)
    parser.add_argument('--workflow', default='release.yml', choices=['release.yml', 'store-package.yml'])
    args = parser.parse_args()
    verify(args.artifact, args.repo, args.source, args.workflow)
