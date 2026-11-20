"""Exercise one exact disposable OS credential, without reading owner entries."""
import argparse
import os
import pathlib
import shlex
import subprocess
import sys
import tempfile
import time


def check(env=None):
    subprocess.run(['go', 'test', './internal/credentials', '-run',
        'TestOSKeyringOwnedLifecycle', '-count=1', '-timeout=45s'],
        env={**os.environ, **(env or {}), 'ODM_TEST_OS_KEYRING': '1'}, check=True, timeout=60)


def security(*args, capture=False):
    return subprocess.run(['security', *args], check=True, text=True,
        stdout=subprocess.PIPE if capture else subprocess.DEVNULL, stderr=subprocess.DEVNULL).stdout


def macos():
    if os.environ.get('CI') != 'true':
        raise RuntimeError('Private macOS test keychain setup is restricted to the disposable CI runner')
    original_default = security('default-keychain', '-d', 'user', capture=True).strip().strip('"')
    original_search = shlex.split(security('list-keychains', '-d', 'user', capture=True))
    with tempfile.TemporaryDirectory(prefix='odm-owned-keyring-') as temp:
        keychain = pathlib.Path(temp) / 'fixture.keychain-db'
        assert keychain.resolve().parent == pathlib.Path(temp).resolve()
        created = changed = False
        try:
            # This public fixture password protects only this new empty QA keychain.
            password = 'ODM-public-disposable-fixture-only'
            security('create-keychain', '-p', password, str(keychain)); created = True
            changed = True
            security('list-keychains', '-d', 'user', '-s', str(keychain))
            security('default-keychain', '-d', 'user', '-s', str(keychain))
            security('unlock-keychain', '-p', password, str(keychain))
            security('set-keychain-settings', '-lut', '21600', str(keychain))
            check()
        finally:
            if changed:
                security('list-keychains', '-d', 'user', '-s', *original_search)
                security('default-keychain', '-d', 'user', '-s', original_default)
            if created:
                security('delete-keychain', str(keychain))


def linux_session():
    with tempfile.TemporaryDirectory(prefix='odm-owned-keyring-') as temp:
        root = pathlib.Path(temp)
        for name in ['data', 'config', 'runtime', 'control']:
            (root / name).mkdir(mode=0o700)
        env = {**os.environ, 'XDG_DATA_HOME': str(root / 'data'),
               'XDG_CONFIG_HOME': str(root / 'config'), 'XDG_RUNTIME_DIR': str(root / 'runtime')}
        daemon = subprocess.Popen(['gnome-keyring-daemon', '--foreground', '--unlock',
            '--components=secrets', '--control-directory', str(root / 'control')],
            stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
        try:
            daemon.stdin.write(b'ODM-public-disposable-fixture-only'); daemon.stdin.close()
            deadline = time.monotonic() + 10
            while True:
                result = subprocess.run(['dbus-send', '--session', '--print-reply',
                    '--dest=org.freedesktop.DBus', '/org/freedesktop/DBus',
                    'org.freedesktop.DBus.GetNameOwner', 'string:org.freedesktop.secrets'],
                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, env=env)
                if result.returncode == 0:
                    break
                if daemon.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError('Private Secret Service did not become ready')
                time.sleep(.1)
            check(env)
        finally:
            daemon.terminate()
            try:
                daemon.wait(timeout=10)
            except subprocess.TimeoutExpired:
                daemon.kill(); daemon.wait(timeout=5)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--linux-session', action='store_true')
    args = parser.parse_args()
    if sys.platform == 'win32':
        check()
    elif sys.platform == 'darwin':
        macos()
    elif sys.platform == 'linux':
        if args.linux_session:
            linux_session()
        else:
            subprocess.run(['dbus-run-session', '--', sys.executable,
                str(pathlib.Path(__file__).resolve()), '--linux-session'], check=True, timeout=90)
    else:
        raise RuntimeError('OS keyring test target unsupported')
    print('PASS: exact owned OS keyring entry stored, read, removed and absence verified; no owner entry was enumerated.')


if __name__ == '__main__':
    main()
