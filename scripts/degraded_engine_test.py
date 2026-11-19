"""Corrupt optional component state must not prevent ordinary HTTP downloads."""
import argparse
import http.server
import json
import pathlib
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--exe', required=True)
args = parser.parse_args()
payload = b'original engine isolation fixture'


class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length', str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


with tempfile.TemporaryDirectory(prefix='odm-degraded-') as temp:
    root = pathlib.Path(temp)
    data = root / 'data'
    (data / 'addons').mkdir(parents=True)
    original_db = b'original deliberately corrupt database fixture' * 256
    (data / 'torrents.db').write_bytes(original_db)
    (data / 'addons/registry.json').write_text('invalid add-on registry fixture')
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    child = subprocess.Popen([str(pathlib.Path(args.exe).resolve()), '--no-browser', '--engine', 'odm-missing-test-engine', '--data-dir', str(data), '--downloads', str(root / 'files')], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    line = child.stdout.readline()
    assert line.startswith('Open Download Manager: '), line
    base, token = line.split(': ', 1)[1].strip().split('/#')

    def api(path, body=None):
        request = urllib.request.Request(base + '/api/' + path, data=json.dumps(body).encode() if body is not None else None, headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
        return json.load(urllib.request.urlopen(request, timeout=15))

    try:
        status = api('status')
        assert status['capabilities'] == {'http': True, 'torrents': False, 'ftp_sftp': False}
        gid = api('add', {'url': f'http://127.0.0.1:{server.server_port}/healthy.bin'})['result']
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            row = next((row for row in api('status')['downloads'] if row['gid'] == gid and row['status'] == 'complete'), None)
            if row:
                assert pathlib.Path(row['files'][0]['path']).read_bytes() == payload
                break
            time.sleep(.05)
        else:
            raise AssertionError('Healthy HTTP engine blocked by failed components')
        try:
            api('addons')
            raise AssertionError('Corrupt add-on registry treated as healthy')
        except urllib.error.HTTPError as error:
            assert error.code == 503
        assert (data / 'torrents.db').read_bytes() == original_db
        assert (data / 'addons/registry.json').read_text() == 'invalid add-on registry fixture'
        print('PASS: HTTP works with corrupt torrent/add-on state; component data preserved and unavailable capabilities reported')
    finally:
        api('quit', {})
        child.wait(timeout=20)
        server.shutdown()
