"""Real native parallel-download crash recovery and changed-resource isolation."""
import argparse
import hashlib
import http.server
import json
import pathlib
import queue
import re
import subprocess
import tempfile
import threading
import time
import urllib.request


def scenario(executable, replace_after_crash, chunk_delay):
    original = bytes(range(256)) * (9 * 4096)
    replacement = b'R' * len(original)
    generation = 0
    ranges = []

    class Fixture(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            body = replacement if generation else original
            etag = '"replacement"' if generation else '"original"'
            start, end = 0, len(body) - 1
            header = self.headers.get('Range')
            if header and self.headers.get('If-Range') == etag:
                match = re.fullmatch(r'bytes=(\d+)-(\d+)', header)
                if match is None:
                    self.send_error(416)
                    return
                start, end = map(int, match.groups())
                if not 0 <= start <= end < len(body):
                    self.send_error(416)
                    return
                ranges.append((start, end))
                self.send_response(206)
                self.send_header('Content-Range', f'bytes {start}-{end}/{len(body)}')
            else:
                self.send_response(200)
            self.send_header('Accept-Ranges', 'bytes')
            self.send_header('ETag', etag)
            self.send_header('Content-Length', str(end - start + 1))
            self.end_headers()
            try:
                for offset in range(start, end + 1, 32768):
                    self.wfile.write(body[offset:min(end + 1, offset + 32768)])
                    self.wfile.flush()
                    time.sleep(chunk_delay)
            except (ConnectionError, OSError):
                pass

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    scratch = tempfile.TemporaryDirectory(prefix='odm-segment-recovery-')
    root = pathlib.Path(scratch.name)
    process = None
    base = token = None

    def start():
        nonlocal process, base, token
        process = subprocess.Popen([executable, '--no-browser', '--engine', 'odm-test-no-aria2',
            '--data-dir', str(root / 'data'), '--downloads', str(root / 'files')],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True,
            creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
        lines = queue.Queue()
        threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
        line = lines.get(timeout=25)
        if not line.startswith('Open Download Manager: '):
            raise RuntimeError('Disposable segmented-transfer process did not start')
        base, token = line.split(': ', 1)[1].strip().split('/#')

    def api(route, body=None):
        request = urllib.request.Request(base + route,
            data=None if body is None else json.dumps(body).encode(),
            headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=15) as response:
            return json.load(response)

    def wait_job(gid, condition):
        deadline = time.monotonic() + 120
        idle_deadline = time.monotonic() + 30
        previous = None
        job = None
        while time.monotonic() < deadline:
            job = next(row for row in api('/api/http-queue')['jobs'] if row['id'] == gid)
            progress = (job['status'], job['completed'], job['committed'])
            if progress != previous:
                previous = progress
                idle_deadline = time.monotonic() + 30
            if condition(job):
                return job
            if job['status'] == 'error':
                raise RuntimeError('Native segmented transfer reported an error')
            if time.monotonic() >= idle_deadline:
                break
            time.sleep(.05)
        # Only safe numerical/state diagnostics, never launch/pairing credentials.
        detail = 'no status' if job is None else f"status={job['status']}, completed={job['completed']}/{job['total']}, committed={job['committed']}"
        raise RuntimeError('Native segmented transfer did not reach expected state: ' + detail)

    try:
        start()
        gid = api('/api/http-queue', {'action': 'add', 'url': f'http://127.0.0.1:{server.server_port}/segment-recovery.bin'})['gid']
        before = wait_job(gid, lambda j: len(j.get('segments', [])) == 3 and 0 < j['committed'] < j['total'])
        process.kill()
        process.wait(timeout=10)
        if len(ranges) < 3:
            raise RuntimeError('Expected real parallel range requests before crash')
        generation = int(replace_after_crash)
        start()
        recovered = api('/api/http-queue')['jobs'][0]
        if recovered['status'] != 'paused' or recovered['committed'] < before['committed']:
            raise RuntimeError('Crash recovery did not retain a paused durable checkpoint')
        api('/api/control', {'gid': gid, 'action': 'resume'})
        completed = wait_job(gid, lambda j: j['status'] == 'complete')
        expected = replacement if replace_after_crash else original
        actual = (root / 'files' / completed['name']).read_bytes()
        if hashlib.sha256(actual).digest() != hashlib.sha256(expected).digest():
            raise RuntimeError('Segment recovery mixed resources or left a hole')
        starts = {0, len(original) // 3, 2 * (len(original) // 3)}
        if not replace_after_crash and not any(start not in starts for start, _end in ranges):
            raise RuntimeError('Resume did not request checkpoint-relative segment tails')
        if replace_after_crash and completed.get('segments'):
            raise RuntimeError('Changed resource did not discard its old segment layout')
        api('/api/quit', {})
        process.wait(timeout=15)
        print('PASS:', 'changed-resource isolation' if replace_after_crash else 'parallel checkpoint crash/resume', 'and exact file SHA-256.', flush=True)
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        server.shutdown()
        server.server_close()
        scratch.cleanup()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--exe', required=True, type=pathlib.Path)
    parser.add_argument('--chunk-delay', type=float, default=.02,
                        help='Local fixture pacing; 0.11 exercises transfers longer than the old 25-second test deadline')
    args = parser.parse_args()
    executable = str(args.exe.resolve())
    if not 0 <= args.chunk_delay <= .2:
        parser.error('fixture chunk delay must be between 0 and 0.2 seconds')
    for replace in (False, True):
        scenario(executable, replace, args.chunk_delay)


if __name__ == '__main__':
    main()
