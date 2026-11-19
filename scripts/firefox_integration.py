"""Real Firefox extension pairing/transfer/revocation in disposable browser/app profiles.

Requires a separately verified Firefox and geckodriver. No personal browser profile,
publisher credentials or remote website is used. The disposable driver enables
Firefox UI access solely to open its own installed extension's internal popup.
"""
import argparse
import hashlib
import http.server
import json
import pathlib
import queue
import re
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser()
    for name in ['exe', 'firefox', 'driver', 'extension']:
        parser.add_argument('--' + name, required=True, type=pathlib.Path)
    args = parser.parse_args()
    paths = {name: str(getattr(args, name).resolve()) for name in ['exe', 'firefox', 'driver', 'extension']}
    for value in paths.values():
        if not pathlib.Path(value).is_file():
            raise RuntimeError('Required application/browser/driver/package is missing')
    flags = getattr(subprocess, 'CREATE_NO_WINDOW', 0)
    with tempfile.TemporaryDirectory(prefix='odm-firefox-') as temp:
        scratch = pathlib.Path(temp)
        backend = subprocess.Popen([paths['exe'], '--no-browser', '--engine', 'odm-test-no-aria2',
            '--data-dir', str(scratch / 'data'), '--downloads', str(scratch / 'files')],
            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, creationflags=flags)
        driver = None
        session_id = None
        server = None
        base = token = webdriver = None

        def request(url, method='GET', body=None, headers=None, timeout=20):
            data = None if body is None else json.dumps(body).encode()
            req = urllib.request.Request(url, data=data, method=method,
                headers={'Content-Type': 'application/json', **(headers or {})})
            try:
                with urllib.request.urlopen(req, timeout=timeout) as response:
                    return json.load(response)
            except urllib.error.HTTPError as error:
                # Never echo browser script arguments or pairing credentials.
                detail = json.load(error).get('value', {})
                message = str(detail.get('message', '')).splitlines()[0][:350]
                message = re.sub(r'[a-f0-9]{64}', '[private token]', message)
                raise RuntimeError('Local test endpoint rejected a request: ' + str(error.code) + ' ' + message) from None

        def app(route, body=None):
            return request(base + route, 'GET' if body is None else 'POST', body,
                {'Authorization': 'Bearer ' + token, 'Origin': base})

        def wd(route, body=None, method=None):
            result = request(webdriver + '/session/' + session_id + route,
                method or ('GET' if body is None else 'POST'), body)
            if isinstance(result.get('value'), dict) and result['value'].get('error'):
                raise RuntimeError('Browser test operation failed')
            return result.get('value')

        def script(source, arguments=None):
            return wd('/execute/sync', {'script': source, 'args': arguments or []})

        def wait_status(text):
            deadline = time.monotonic() + 15
            while time.monotonic() < deadline:
                value = script('return document.getElementById("status")?.textContent || "";')
                if text in value:
                    return
                time.sleep(.1)
            raise RuntimeError('Expected browser extension feedback did not appear: ' + text)

        try:
            launch = queue.Queue()
            threading.Thread(target=lambda: launch.put(backend.stdout.readline()), daemon=True).start()
            line = launch.get(timeout=25)
            if not line.startswith('Open Download Manager: '):
                raise RuntimeError('Disposable ODM backend did not start')
            base, token = line.split(': ', 1)[1].strip().split('/#')
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0))
                port = listener.getsockname()[1]
            webdriver = f'http://127.0.0.1:{port}'
            driver = subprocess.Popen([paths['driver'], '--host', '127.0.0.1', '--port', str(port), '--log', 'error', '--allow-system-access'],
                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, creationflags=flags)
            deadline = time.monotonic() + 15
            while True:
                try:
                    if request(webdriver + '/status', timeout=2)['value']['ready']:
                        break
                except (OSError, RuntimeError):
                    if time.monotonic() >= deadline:
                        raise RuntimeError('Disposable browser driver did not start') from None
                time.sleep(.1)
            identity_id = 'odm-browser@open-download-manager.local'
            extension_uuid = str(uuid.uuid4())
            session = request(webdriver + '/session', 'POST', {'capabilities': {'alwaysMatch': {
                'browserName': 'firefox', 'acceptInsecureCerts': False,
                'moz:firefoxOptions': {'binary': paths['firefox'], 'args': ['-headless'],
                    'prefs': {'extensions.webextensions.uuids': json.dumps({identity_id: extension_uuid}),
                              'browser.shell.checkDefaultBrowser': False, 'app.update.auto': False,
                              'datareporting.healthreport.uploadEnabled': False,
                              'toolkit.telemetry.enabled': False}},
            }}}, timeout=45)['value']
            session_id = session['sessionId']
            installed = wd('/moz/addon/install', {'path': paths['extension'], 'temporary': True})
            if installed != identity_id:
                raise RuntimeError('Unexpected installed extension identity')
            origin = 'moz-extension://' + extension_uuid
            # WebDriver content navigation intentionally cannot open internal extension
            # pages. Use the browser UI only for this synthetic, installed extension.
            wd('/moz/context', {'context': 'chrome'})
            script('window.gBrowser.selectedBrowser.loadURI(Services.io.newURI(arguments[0]), {triggeringPrincipal:Services.scriptSecurityManager.getSystemPrincipal()}); return true;', [origin + '/popup.html'])
            wd('/moz/context', {'context': 'content'})
            deadline = time.monotonic() + 10
            while True:
                actual = script('return document.getElementById("identity")?.value;')
                if actual or time.monotonic() >= deadline:
                    break
                time.sleep(.1)
            if actual != origin:
                raise RuntimeError('Extension popup did not expose its installed identity')
            pairing = app('/api/pair', {'extensionOrigin': origin})
            script('document.getElementById("pairing").value=arguments[0]; document.getElementById("pair").requestSubmit();', [json.dumps(pairing)])
            wait_status('Pairing saved on this device.')
            payload = b'Firefox extension real transfer\n' * 4096

            class Fixture(http.server.BaseHTTPRequestHandler):
                def log_message(self, *_args):
                    pass

                def do_GET(self):
                    self.send_response(200)
                    self.send_header('Content-Length', str(len(payload)))
                    self.end_headers()
                    self.wfile.write(payload)

            server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Fixture)
            threading.Thread(target=server.serve_forever, daemon=True).start()
            url = f'http://127.0.0.1:{server.server_port}/firefox-fixture.bin'
            script('document.getElementById("url").value=arguments[0]; document.getElementById("send").requestSubmit();', [url])
            wait_status('Download added to ODM.')
            deadline = time.monotonic() + 20
            while time.monotonic() < deadline:
                completed = next((item for item in app('/api/status')['downloads'] if item['status'] == 'complete'), None)
                if completed:
                    if pathlib.Path(completed['files'][0]['path']).read_bytes() != payload:
                        raise RuntimeError('Extension transfer bytes do not match')
                    break
                time.sleep(.1)
            else:
                raise RuntimeError('Extension transfer did not finish')
            app('/api/revoke-browsers', {})
            script('document.getElementById("url").value=arguments[0]; document.getElementById("send").requestSubmit();', [url + '?revoked=1'])
            wait_status('ODM rejected this request.')
            if len(app('/api/status')['downloads']) != 1:
                raise RuntimeError('Revoked extension unexpectedly created another transfer')
            script('document.getElementById("disconnect").click();')
            wait_status('Pairing forgotten.')
            wd('/moz/addon/uninstall', {'id': installed})
            capabilities = session['capabilities']
            print('PASS: Firefox', capabilities['browserVersion'], 'installed extension, popup pairing, real transfer bytes, revocation, disconnect and uninstall.')
            print('Extension SHA-256:', hashlib.sha256(pathlib.Path(paths['extension']).read_bytes()).hexdigest())
        finally:
            if session_id:
                try:
                    wd('', method='DELETE')
                except Exception:
                    pass
            if driver:
                driver.terminate()
                driver.wait(timeout=10)
            if base and backend.poll() is None:
                try:
                    app('/api/quit', {})
                    backend.wait(timeout=15)
                except Exception:
                    backend.kill()
                    backend.wait(timeout=5)
            elif backend.poll() is None:
                backend.kill()
                backend.wait(timeout=5)
            if server:
                server.shutdown()
                server.server_close()


if __name__ == '__main__':
    main()
