"""Real signed package -> permission review -> WASI resolution -> file transfer."""
import argparse
import hashlib
import http.server
import json
import os
import pathlib
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--exe', required=True)
parser.add_argument('--native-http', action='store_true')
args = parser.parse_args()
payload = b'original legal add-on integration fixture\n' * 8192


class Fixture(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Length', str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


with tempfile.TemporaryDirectory(prefix='odm-addons-') as temp:
    folder = pathlib.Path(temp)
    module, key, package = [folder / name for name in ['resolver.wasm', 'private.key', 'resolver.odmx']]
    env = {**os.environ, 'GOOS': 'wasip1', 'GOARCH': 'wasm', 'CGO_ENABLED': '0'}
    subprocess.run(['go', 'build', '-o', str(module), './sdk/examples/clean-link'], cwd=ROOT, env=env, check=True)
    manifest = json.loads((ROOT / 'sdk/examples/clean-link/manifest.json').read_text())
    manifest['input_hosts'] = manifest['output_hosts'] = ['127.0.0.1']
    manifest_path = folder / 'manifest.json'
    manifest_path.write_text(json.dumps(manifest))
    command = ['go', 'run', './sdk/cmd/odm-addon-pack']
    go_root = pathlib.Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
    license_path = folder / 'LICENSE'
    license_path.write_text((ROOT / 'LICENSE').read_text() + '\nGo standard library:\n' + (go_root / 'LICENSE').read_text())
    subprocess.run(command + ['--generate-key', str(key)], cwd=ROOT, check=True)
    subprocess.run(command + ['--manifest', str(manifest_path), '--module', str(module), '--key', str(key), '--license', str(license_path), '--out', str(package)], cwd=ROOT, check=True)
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    child = subprocess.Popen([str(pathlib.Path(args.exe).resolve()), '--no-browser', '--data-dir', str(folder / 'data'), '--downloads', str(folder / 'downloads')] + (['--engine', 'odm-deliberately-missing-aria2'] if args.native_http else []), stdout=subprocess.PIPE, text=True, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    base = token = None
    try:
        line = child.stdout.readline()
        assert line.startswith('Open Download Manager: '), line
        base, token = line.split(': ', 1)[1].strip().split('/#')

        def api(path, data=None, raw=None, approve=None):
            headers = {'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'}
            if approve:
                headers['X-ODM-Approve-SHA256'] = approve
            body = raw if raw is not None else json.dumps(data).encode() if data is not None else None
            request = urllib.request.Request(base + '/api/' + path, data=body, headers=headers)
            return json.load(urllib.request.urlopen(request, timeout=20))

        blob = package.read_bytes()
        info = api('addons/inspect', raw=blob)
        assert info['digest'] == hashlib.sha256(blob).hexdigest()
        try:
            api('addons/install', raw=blob)
            raise AssertionError('Package installed without exact digest approval')
        except urllib.error.HTTPError as error:
            assert error.code == 400
        api('addons/install', raw=blob, approve=info['digest'])
        assert api('addons')[0]['enabled'] is False
        identity = manifest['id']
        api('addons/enable', {'id': identity, 'enabled': True})
        source = f'http://127.0.0.1:{server.server_port}/resolved.bin?utm_source=test#fragment'
        links = api('addons/resolve', {'id': identity, 'url': source})['urls']
        assert links == [f'http://127.0.0.1:{server.server_port}/resolved.bin']
        assert not api('status')['downloads'], 'Resolver started a transfer without user confirmation'
        gid = api('batch', {'urls': links})['results'][0]['gid']
        until = time.monotonic() + 30
        while time.monotonic() < until:
            row = next((row for row in api('status')['downloads'] if row['gid'] == gid and row['status'] == 'complete'), None)
            if row:
                assert pathlib.Path(row['files'][0]['path']).read_bytes() == payload
                break
            time.sleep(.1)
        else:
            raise AssertionError('Resolved-link transfer did not complete')
        api('addons/enable', {'id': identity, 'enabled': False})
        try:
            api('addons/resolve', {'id': identity, 'url': source})
            raise AssertionError('Revoked add-on ran')
        except urllib.error.HTTPError as error:
            assert error.code == 400
        api('addons/uninstall', {'id': identity})
        assert api('addons') == []
        print('PASS: signed package CLI, inspect, exact approval, disabled grant, WASI resolution, explicit real transfer, revocation and uninstall')
    finally:
        if base and token and child.poll() is None:
            try:
                request = urllib.request.Request(base + '/api/quit', data=b'{}', headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
                urllib.request.urlopen(request, timeout=15).close()
            except (OSError, urllib.error.URLError):
                child.terminate()
        if child.poll() is None:
            child.wait(timeout=20)
        server.shutdown()
