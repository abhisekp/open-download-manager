"""Real webseed completion, opt-in sharing, process death and expired restart."""
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


def encode(value):
    if isinstance(value, int):
        return b'i' + str(value).encode() + b'e'
    if isinstance(value, str):
        value = value.encode()
    if isinstance(value, bytes):
        return str(len(value)).encode() + b':' + value
    if isinstance(value, list):
        return b'l' + b''.join(encode(item) for item in value) + b'e'
    if isinstance(value, dict):
        return b'd' + b''.join(encode(key) + encode(value[key]) for key in sorted(value)) + b'e'
    raise TypeError('unsupported fixture type')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--exe', required=True, type=pathlib.Path)
    args = parser.parse_args()
    payload = b'original deadline recovery fixture\n' * 1024
    requests = []

    class Source(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def do_GET(self):
            requests.append(self.path)
            match = re.fullmatch(r'bytes=(\d+)-(\d+)', self.headers.get('Range', ''))
            if not match:
                self.send_error(400)
                return
            begin, end = map(int, match.groups())
            if not 0 <= begin <= end < len(payload):
                self.send_error(416)
                return
            self.send_response(206)
            self.send_header('Content-Range', f'bytes {begin}-{end}/{len(payload)}')
            self.send_header('Content-Length', str(end - begin + 1))
            self.end_headers()
            self.wfile.write(payload[begin:end + 1])

    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Source)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    process = None
    base = token = None
    scratch = tempfile.TemporaryDirectory(prefix='odm-seed-recovery-')
    try:
        root = pathlib.Path(scratch.name)

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
                raise RuntimeError('Disposable seeding app failed to start')
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
                row = next((j for j in api('status')['downloads'] if j['gid'] == gid), None)
                if row and row['status'] == status:
                    return row
                time.sleep(.05)
            raise RuntimeError('Seeding fixture did not reach expected state')

        start()
        pieces = b''.join(hashlib.sha1(payload[i:i + 16384]).digest() for i in range(0, len(payload), 16384))
        metadata = encode({'info': {'name': 'seed.bin', 'length': len(payload), 'piece length': 16384,
                                   'pieces': pieces}, 'url-list': [f'http://127.0.0.1:{server.server_port}/seed.bin']})
        request = urllib.request.Request(base + '/api/torrent-file', data=metadata,
            headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/x-bittorrent'})
        with urllib.request.urlopen(request, timeout=10) as response:
            gid = json.load(response)['result']
        complete = wait(gid, 'complete')
        file = pathlib.Path(complete['files'][0]['path'])
        assert file.read_bytes() == payload
        until = api('torrent-seeding', {'gid': gid, 'minutes': 1, 'approved': True})['until']
        wait(gid, 'seeding')
        process.kill()
        process.wait(timeout=10)
        print('Seeding fixture killed; waiting for its approved one-minute deadline to expire.', flush=True)
        time.sleep(max(0, until - time.time()) + .2)
        count = len(requests)
        start()
        wait(gid, 'complete')
        policy = api('torrent-seeding?gid=' + gid)
        assert policy['until'] == until and not policy['active'] and policy['ready']
        assert file.read_bytes() == payload and len(requests) == count
        api('quit', {})
        process.wait(timeout=20)
        print('PASS: exact webseed bytes, explicit bounded sharing, real process death and expired restart without sharing or repeated source requests.')
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait(timeout=10)
        server.shutdown()
        server.server_close()
        scratch.cleanup()


if __name__ == '__main__':
    main()
