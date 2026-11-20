'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const { loadPreference, savePreference, readStatus, NotificationMonitor } = require('./notifications.cjs');

test('notification opt-in persists, damaged originals and failed saves are preserved', t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'odm-notifications-'));
  t.after(() => fs.rmSync(dir, { recursive: true }));
  const file = path.join(dir, 'notifications.json');
  assert.equal(loadPreference(file), false);
  savePreference(file, true);
  assert.equal(loadPreference(file), true);
  savePreference(file, false);
  assert.equal(loadPreference(file), false);
  for (const original of ['null', '{"version":1,"enabled":"yes"}', 'x'.repeat(513), '{"version":1,"enabled":true,"extra":1}']) {
    fs.writeFileSync(file, original);
    assert.throws(() => loadPreference(file));
    assert.throws(() => savePreference(file, true));
    assert.equal(fs.readFileSync(file, 'utf8'), original);
  }
  assert.throws(() => savePreference(path.join(dir, 'missing', 'notifications.json'), true));
  assert.deepEqual(fs.readdirSync(dir), ['notifications.json']);
});

test('only new terminal transitions produce generic summaries, with no startup flood', async () => {
  let rows = new Map([['old', 'complete'], ['running', 'active']]);
  const notices = [];
  const monitor = new NotificationMonitor(async () => rows, body => notices.push(body), 60000);
  try {
    await monitor.setEnabled(true);
    assert.deepEqual(notices, []);
    rows = new Map([['old', 'complete'], ['running', 'complete'], ['new', 'error']]);
    await monitor.tick(monitor.generation);
    assert.deepEqual(notices, ['1 download completed. 1 download needs attention.']);
    await monitor.tick(monitor.generation);
    assert.equal(notices.length, 1);
    rows = new Map([['running', 'seeding'], ['new', 'active']]);
    await monitor.tick(monitor.generation);
    assert.equal(notices.length, 1);
    rows = new Map([['running', 'seeding'], ['new', 'error']]);
    await monitor.tick(monitor.generation);
    assert.equal(notices.at(-1), '1 download needs attention.');
    rows = new Map([['running', 'complete'], ['new', 'error']]);
    await monitor.tick(monitor.generation);
    assert.equal(notices.length, 2, 'stopping completed torrent seeding is not a second download completion');
    monitor.setEnabled(false);
    await monitor.setEnabled(true);
    assert.equal(notices.length, 2, 'reenabling establishes a fresh baseline');
  } finally { monitor.setEnabled(false); }
});

test('disabled and replaced polls cannot notify or restore their stale baseline', async () => {
  let resolve, oldSignal;
  const notices = [];
  const monitor = new NotificationMonitor(signal => {
    oldSignal = signal;
    return new Promise(done => { resolve = done; });
  }, body => notices.push(body), 60000);
  const pending = monitor.setEnabled(true);
  monitor.setEnabled(false);
  assert.equal(oldSignal.aborted, true);
  monitor.read = async () => new Map([['fresh', 'active']]);
  await monitor.setEnabled(true);
  resolve(new Map([['stale', 'error']]));
  await pending;
  assert.deepEqual([...monitor.previous], [['fresh', 'active']]);
  assert.deepEqual(notices, []);
  monitor.setEnabled(false);
});

test('status failure preserves the last baseline and OS failures stay isolated', async () => {
  let fail = false;
  const monitor = new NotificationMonitor(async () => {
    if (fail) throw new Error('fixture unavailable');
    return new Map([['job', fail === null ? 'complete' : 'active']]);
  }, () => { throw new Error('OS unavailable'); }, 60000);
  try {
    await monitor.setEnabled(true);
    fail = true;
    await monitor.tick(monitor.generation);
    assert.equal(monitor.previous.get('job'), 'active');
    fail = null;
    await monitor.tick(monitor.generation);
    assert.equal(monitor.previous.get('job'), 'complete');
  } finally { monitor.setEnabled(false); }
});

test('authenticated loopback status reads are bounded, redirect-free and redact row data', async t => {
  let mode = 'ok', calls = 0;
  const server = http.createServer((request, response) => {
    calls++;
    assert.equal(request.headers.authorization, 'Bearer fixture-only');
    if (mode === 'redirect') { response.writeHead(302, { Location: '/other' }); response.end(); return; }
    const bodies = {
      ok: JSON.stringify({ downloads: [{ gid: 'id', status: 'complete', files: [{ path: 'PRIVATE' }], errorMessage: 'PRIVATE' }] }),
      duplicate: JSON.stringify({ downloads: [{ gid: 'id', status: 'complete' }, { gid: 'id', status: 'complete' }] }),
      invalid: JSON.stringify({ downloads: [{ gid: 'id', status: 'unknown' }] }),
      oversize: 'x'.repeat(2 * 1024 * 1024 + 1),
    };
    response.end(bodies[mode]);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); return new Promise(resolve => server.close(resolve)); });
  const address = { origin: `http://127.0.0.1:${server.address().port}`, token: 'fixture-only' };
  assert.deepEqual([...await readStatus(address, AbortSignal.timeout(3000))], [['id', 'complete']]);
  for (mode of ['duplicate', 'invalid', 'oversize', 'redirect']) await assert.rejects(readStatus(address, AbortSignal.timeout(3000)));
  assert.equal(calls, 5, 'no redirect request');
  const controller = new AbortController(); controller.abort();
  await assert.rejects(readStatus(address, controller.signal));
});
