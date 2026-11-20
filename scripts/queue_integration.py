"""Real process-crash recovery for a persisted, not-yet-started HTTP schedule."""
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
    executable = str(args.exe.resolve())
    flags = getattr(subprocess, 'CREATE_NO_WINDOW', 0)
    payload = b'original scheduled recovery fixture\n' * 8192
    requests = []

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            requests.append(self.path)
            self.send_response(200)
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    process = None
    base = token = None
    scratch = tempfile.TemporaryDirectory(prefix="odm-queue-recovery-")
    try:
        root = pathlib.Path(scratch.name)

        def start():
            nonlocal process, base, token
            process = subprocess.Popen([executable, '--no-browser', '--engine', 'odm-test-no-aria2',
                '--data-dir', str(root / 'data'), '--downloads', str(root / 'files')],
                stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, creationflags=flags)
            lines = queue.Queue()
            threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
            line = lines.get(timeout=25)
            if not line.startswith('Open Download Manager: '):
                raise RuntimeError('Disposable scheduled download app did not start')
            base, token = line.split(': ', 1)[1].strip().split('/#')

        def api(route, body=None):
            request = urllib.request.Request(base + route,
                data=None if body is None else json.dumps(body).encode(),
                headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
            with urllib.request.urlopen(request, timeout=10) as response:
                return json.load(response)

        start()
        api('/api/http-queue', {'action': 'settings', 'settings': {'concurrent': 1, 'perHost': 1}})
        when = int(time.time() * 1000) + 1600
        result = api('/api/http-queue', {'action': 'add', 'url': f'http://127.0.0.1:{server.server_port}/scheduled.bin', 'startAt': when})
        process.kill()
        process.wait(timeout=10)
        time.sleep(max(0, (when - int(time.time() * 1000)) / 1000) + .2)
        if requests:
            raise RuntimeError('Schedule contacted its source before process restart')
        start()
        deadline = time.monotonic() + 20
        while True:
            state = api('/api/http-queue')
            job = next(row for row in state['jobs'] if row['id'] == result['gid'])
            if job['status'] == 'complete':
                break
            if job['status'] == 'error' or time.monotonic() >= deadline:
                raise RuntimeError('Recovered schedule did not complete')
            time.sleep(.1)
        if state['settings'] != {'concurrent': 1, 'perHost': 1}:
            raise RuntimeError('Queue limits were not durable')
        if requests != ['/scheduled.bin'] or (root / 'files' / job['name']).read_bytes() != payload:
            raise RuntimeError('Recovered scheduled request count or download integrity failed')
        api('/api/quit', {})
        process.wait(timeout=15)
        start()
        time.sleep(.3)
        if requests != ['/scheduled.bin'] or api('/api/http-queue')['jobs'][0]['status'] != 'complete':
            raise RuntimeError('Completed schedule was repeated after restart')
        api('/api/quit', {})
        process.wait(timeout=15)
        print('PASS: persisted schedule and limits, real process kill, overdue restart, exact bytes, and no repeat after completion.')
        print('Fixture SHA-256:', hashlib.sha256(payload).hexdigest())
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        server.shutdown()
        server.server_close()
        scratch.cleanup()


if __name__ == '__main__':
    main()
