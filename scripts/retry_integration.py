"""Verify durable retries and pause cancellation across actual app process death."""
import argparse
import collections
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
    payload = b'original durable retry fixture\n' * 8192
    counts = collections.Counter()

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            counts[self.path] += 1
            if self.path == '/cancel.bin' or counts[self.path] == 1:
                self.send_response(503)
                self.send_header('Retry-After', '3')
                self.send_header('Content-Length', '0')
                self.end_headers()
                return
            self.send_response(200)
            self.send_header('Content-Length', str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    process = None
    base = token = None
    try:
        with tempfile.TemporaryDirectory(prefix='odm-retry-recovery-') as temp:
            root = pathlib.Path(temp)

            def start():
                nonlocal process, base, token
                process = subprocess.Popen([str(args.exe.resolve()), '--no-browser', '--engine',
                    'odm-no-optional-engine', '--data-dir', str(root / 'data'), '--downloads', str(root / 'files')],
                    stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
                    creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
                lines = queue.Queue()
                threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
                line = lines.get(timeout=25)
                if not line.startswith('Open Download Manager: '):
                    raise RuntimeError('Disposable retry app failed to start')
                base, token = line.split(': ', 1)[1].strip().split('/#')

            def api(route, body=None):
                request = urllib.request.Request(base + '/api/' + route,
                    data=None if body is None else json.dumps(body).encode(),
                    headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
                with urllib.request.urlopen(request, timeout=10) as response:
                    return json.load(response)

            def wait(gid, status):
                deadline = time.monotonic() + 25
                while time.monotonic() < deadline:
                    job = next(j for j in api('http-queue')['jobs'] if j['id'] == gid)
                    if job['status'] == status:
                        return job
                    if job['status'] == 'error' and status != 'error':
                        raise RuntimeError('Retry fixture failed instead of recovering')
                    time.sleep(.05)
                raise RuntimeError('Retry fixture did not reach expected state')

            def quit():
                api('quit', {})
                process.wait(timeout=15)

            source = f'http://127.0.0.1:{server.server_port}'
            start()
            gid = api('http-queue', {'action': 'add', 'url': source + '/recover.bin'})['gid']
            pending = wait(gid, 'retrying')
            assert pending['retryCount'] == 1 and pending['retryLimit'] == 3
            process.kill()
            process.wait(timeout=10)
            assert counts['/recover.bin'] == 1
            time.sleep(max(0, pending['retryAt'] / 1000 - time.time()) + .15)
            start()
            finished = wait(gid, 'complete')
            assert counts['/recover.bin'] == 2 and finished['retryCount'] == 1
            assert (root / 'files' / finished['name']).read_bytes() == payload
            cancelled = api('http-queue', {'action': 'add', 'url': source + '/cancel.bin'})['gid']
            pending = wait(cancelled, 'retrying')
            api('control', {'gid': cancelled, 'action': 'pause'})
            quit()
            start()
            time.sleep(max(0, pending['retryAt'] / 1000 - time.time()) + .2)
            paused = wait(cancelled, 'paused')
            assert not paused.get('retryAt') and counts['/cancel.bin'] == 1
            assert counts['/recover.bin'] == 2, 'Completed retry repeated after another restart'
            quit()
        print('PASS: journaled automatic retry, real process kill/overdue recovery, exact bytes, retained attempt budget, pause/restart cancellation and no repeated completed request.')
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        server.shutdown()
        server.server_close()


if __name__ == '__main__':
    main()
