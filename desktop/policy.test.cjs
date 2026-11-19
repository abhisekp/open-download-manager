'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { launchAddress, localResource, documentNavigation, dataPaths } = require('./policy.cjs');
const line = 'Open Download Manager: http://127.0.0.1:8431/#' + 'a'.repeat(64);
test('only the exact loopback launch protocol is accepted', () => {
  assert.ok(launchAddress(line));
  for (const invalid of [line.replace('127.0.0.1', 'evil.test'), line.replace(':8431', ':65536'), line + ' suffix', line.replace('http:', 'https:'), line.replace('/#', '/api/#')]) {
    assert.equal(launchAddress(invalid), null);
  }
});
test('renderer resources and navigation stay on the owned backend', () => {
  const address = launchAddress(line);
  assert.ok(localResource(address, address.origin + '/api/status'));
  assert.ok(documentNavigation(address, address.origin + '/'));
  for (const invalid of ['file:///etc/passwd', 'javascript:alert(1)', 'https://example.com', 'http://127.0.0.1:8432/', 'http://user@127.0.0.1:8431/']) {
    assert.equal(localResource(address, invalid), false);
  }
  assert.equal(documentNavigation(address, address.origin + '/api/status'), false);
  assert.equal(documentNavigation(address, address.origin + '/?remote=1'), false);
});
test('custom profiles accept only explicit absolute directory options', () => {
  const folder = path.resolve('test-profile');
  assert.deepEqual(dataPaths(['--data-dir', folder, '--engine', 'untrusted-command']), {'data-dir': folder});
  assert.deepEqual(dataPaths(['--downloads=' + folder]), {downloads: folder});
  for (const args of [['--data-dir'], ['--data-dir=relative'], ['--data-dir', folder + '\n'], ['--data-dir', folder, '--data-dir', folder]]) {
    assert.throws(() => dataPaths(args));
  }
});
