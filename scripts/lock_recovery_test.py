"""Prove exclusive ownership and automatic recovery after an abrupt process kill."""
import argparse
import json
import pathlib
import subprocess
import tempfile
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--exe', required=True)
args = parser.parse_args()
with tempfile.TemporaryDirectory(prefix='odm-lock-') as temp:
    root = pathlib.Path(temp)
    command = [str(pathlib.Path(args.exe).resolve()), '--no-browser', '--engine', 'odm-missing-test-engine', '--data-dir', str(root / 'data'), '--downloads', str(root / 'downloads')]

    def start():
        child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        line = child.stdout.readline()
        assert line.startswith('Open Download Manager: '), line
        return child, line.split(': ', 1)[1].strip().split('/#')

    first, _ = start()
    try:
        second = subprocess.run(command, capture_output=True, text=True, timeout=15)
        assert second.returncode != 0 and 'another ODM instance' in second.stderr
        first.kill()
        first.wait(timeout=15)
        assert (root / 'data/running.lock/os-lock-v2').exists()
        replacement, (base, token) = start()
        try:
            request = urllib.request.Request(base + '/api/quit', data=b'{}', headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
            urllib.request.urlopen(request, timeout=15).close()
            assert replacement.wait(timeout=15) == 0
        finally:
            if replacement.poll() is None:
                replacement.kill()
                replacement.wait()
        assert not (root / 'data/running.lock').exists()
        assert (root / 'data/.instance.lock').is_file()
        print('PASS: second process excluded; abrupt kill recovered without manual lock deletion')
    finally:
        if first.poll() is None:
            first.kill()
            first.wait()
