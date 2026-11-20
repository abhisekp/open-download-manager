"""Retain forgotten results across real process restarts and a stale engine checkpoint.

All files, credentials and HTTP traffic belong to this disposable fixture. Restoring
the old owned engine checkpoint models independent engine recovery; it does not
claim that the test triggered a naturally occurring torn write.
"""
import argparse
import hashlib
import http.server
import json
import pathlib
import queue
import subprocess
import tempfile
import threading
import time
import urllib.request


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--exe', required=True, type=pathlib.Path)
    args = parser.parse_args()
    executable = args.exe.resolve()
    payload = b'original retained history fixture\n' * 1024
    requests = 0

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            nonlocal requests
            requests += 1
            self.send_response(200)
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix='odm-history-recovery-') as temp:
            root = pathlib.Path(temp)
            child = None
            base = token = None

            def start():
                nonlocal child, base, token
                child = subprocess.Popen([
                    str(executable), '--no-browser', '--engine', 'odm-fixture-no-aria2',
                    '--data-dir', str(root / 'data'), '--downloads', str(root / 'files'),
                ], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
                    creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
                lines = queue.Queue()
                threading.Thread(target=lambda: lines.put(child.stdout.readline()), daemon=True).start()
                line = lines.get(timeout=25)
                assert line.startswith('Open Download Manager: '), 'Fixture app did not start'
                base, token = line.split(': ', 1)[1].strip().split('/#')

            def api(route, body=None):
                request = urllib.request.Request(
                    base + '/api/' + route,
                    data=None if body is None else json.dumps(body).encode(),
                    headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'},
                )
                with urllib.request.urlopen(request, timeout=15) as response:
                    return json.load(response)

            def stop():
                api('quit', {})
                assert child.wait(timeout=15) == 0, 'Fixture app did not shut down cleanly'
                child.stdout.close()

            try:
                start()
                gid = api('add', {'url': f'http://127.0.0.1:{server.server_port}/history.bin'})['result']
                deadline = time.monotonic() + 20
                completed = None
                while time.monotonic() < deadline:
                    completed = next((row for row in api('status')['downloads']
                                      if row['gid'] == gid and row['status'] == 'complete'), None)
                    if completed:
                        break
                    time.sleep(.05)
                assert completed, 'Original download did not complete'
                downloaded = pathlib.Path(completed['files'][0]['path'])
                expected = hashlib.sha256(payload).digest()
                assert hashlib.sha256(downloaded.read_bytes()).digest() == expected
                stop()
                engine_path = root / 'data/http-transfers.json'
                old_engine_checkpoint = engine_path.read_bytes()
                original = json.loads(old_engine_checkpoint)
                assert any(job['id'] == gid and job['status'] == 'complete' for job in original['jobs'])

                start()
                assert any(row['gid'] == gid for row in api('status')['downloads'])
                api('control', {'gid': gid, 'action': 'forget'})
                assert not any(row['gid'] == gid for row in api('status')['downloads'])
                stop()
                history = json.loads((root / 'data/history.json').read_text(encoding='utf-8'))
                assert history['version'] == 2 and history['forgotten'][gid]
                assert gid not in history['records']

                # Explicitly restore only the fixture's prior engine checkpoint,
                # preserving the user's later forgotten-history decision.
                engine_path.write_bytes(old_engine_checkpoint)
                start()
                state = api('status')
                assert not any(row['gid'] == gid for row in state['downloads']), 'Forgotten engine result returned'
                assert not state.get('warnings'), 'Recovered history reported a warning'
                assert requests == 1, 'Recovery fetched the completed source again'
                assert hashlib.sha256(downloaded.read_bytes()).digest() == expected, 'Original bytes changed'
                stop()
                print('PASS: real download/forget/restarts, stale engine checkpoint suppressed, no refetch and original bytes retained.')
            finally:
                if child is not None:
                    if child.poll() is None:
                        try:
                            stop()
                        except Exception:
                            child.kill()
                            child.wait(timeout=10)
                    if child.stdout and not child.stdout.closed:
                        child.stdout.close()
    finally:
        server.shutdown()
        server.server_close()


if __name__ == '__main__':
    main()
