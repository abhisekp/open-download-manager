'use strict';
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const { app, Menu, Notification } = require('electron');
// Keep test profile and downloads out of the real user's application data.
const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'odm-desktop-'));
process.env.APPDATA = scratch;
process.env.XDG_CONFIG_HOME = scratch;
if (process.platform === 'darwin') process.env.HOME = scratch;
app.setPath('userData', path.join(scratch, 'shell'));
app.setPath('appData', scratch);
fs.mkdirSync(path.join(scratch, 'downloads'));
app.setPath('downloads', path.join(scratch, 'downloads'));
const watchdog = setTimeout(() => { console.error('Desktop smoke timed out'); app.exit(1); }, 45000);
const packagedResources = process.argv[2] ? path.resolve(process.argv[2]) : undefined;
const implementation = packagedResources ? path.join(packagedResources, 'app.asar', 'app.cjs') : './app.cjs';
require(implementation).start(async ({ window, backend, address, show, notifications, quit }) => {
  try {
    const prefs = window.webContents.getLastWebPreferences();
    assert.equal(prefs.sandbox, true);
    assert.equal(prefs.nodeIntegration, false);
    assert.equal(prefs.contextIsolation, true);
    assert.equal(prefs.webSecurity, true);
    const state = await window.webContents.executeJavaScript(`(async () => {
      const token = location.hash.slice(1) || sessionStorage.getItem('odm-token');
      const response = await fetch('/api/status', {headers:{Authorization:'Bearer '+token}});
      return {status:response.status, node:typeof require, text:document.body.innerText};
    })()`);
    assert.equal(state.status, 200);
    assert.equal(state.node, 'undefined');
    assert.match(state.text, /Open Download\s+Manager/);
    const remote = await window.webContents.executeJavaScript(`fetch('https://example.com').then(()=>true,()=>false)`);
    assert.equal(remote, false);
    window.hide(); assert.equal(window.isVisible(), false);
    show(); assert.equal(window.isVisible(), true);
    const reply = await fetch(address.origin + '/api/status', {headers:{Authorization:'Bearer '+address.token}});
    assert.equal(reply.status, 200);
    const preference = Menu.getApplicationMenu().items.find(item => item.label === 'Settings').submenu.items[0];
    assert.equal(preference.checked, false, 'notifications default off');
    assert.equal(notifications.enabled, false);
    assert.equal(preference.enabled, Notification.isSupported());
    const notices = [];
    if (preference.enabled) {
      const nativeNotify = notifications.notify;
      notifications.notify = body => { notices.push(body); nativeNotify(body); };
      preference.click();
      assert.equal(JSON.parse(fs.readFileSync(path.join(app.getPath('userData'), 'notifications.json'), 'utf8')).enabled, true);
      await notifications.setEnabled(true);
    }
    const payload = Buffer.from('real desktop transfer fixture\n'.repeat(4096));
    const fixture = http.createServer((_request, response) => { response.writeHead(200, {'Content-Length': payload.length}); response.end(payload); });
    await new Promise(resolve => fixture.listen(0, '127.0.0.1', resolve));
    try {
      const added = await fetch(address.origin + '/api/add', {method:'POST', headers:{Authorization:'Bearer '+address.token, Origin:address.origin, 'Content-Type':'application/json'}, body:JSON.stringify({url:`http://127.0.0.1:${fixture.address().port}/desktop-fixture.bin`})});
      assert.equal(added.status, 200);
      const { result: gid } = await added.json();
      let completed;
      for (let attempt = 0; attempt < 150; attempt++) {
        const status = await fetch(address.origin + '/api/status', {headers:{Authorization:'Bearer '+address.token}}).then(response => response.json());
        completed = status.downloads.find(item => item.gid === gid && item.status === 'complete');
        if (completed) break;
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      assert.ok(completed, 'desktop transfer completes');
      assert.deepEqual(fs.readFileSync(completed.files[0].path), payload);
      if (preference.enabled) {
        await notifications.tick(notifications.generation);
        assert.deepEqual(notices, ['1 download completed.']);
        await notifications.tick(notifications.generation);
        assert.equal(notices.length, 1, 'completion not repeated');
        preference.click();
        assert.equal(notifications.enabled, false);
        assert.equal(JSON.parse(fs.readFileSync(path.join(app.getPath('userData'), 'notifications.json'), 'utf8')).enabled, false);
        console.log('Native notification opt-in, real completion routing, deduplication and saved opt-out passed; OS banner delivery is not asserted.');
      }
    } finally { fixture.closeAllConnections(); await new Promise(resolve => fixture.close(resolve)); }
    const data = path.join(scratch, 'OpenDownloadManager');
    backend.once('exit', code => {
      try {
        assert.equal(code, 0);
        assert.equal(fs.existsSync(path.join(data, 'running.lock')), false);
        assert.ok(fs.existsSync(path.join(data, 'http-transfers.json')));
        console.log('Desktop graceful shutdown passed: checkpoint saved and data ownership released.');
      } catch (error) { console.error(error.message); process.exitCode = 1; }
    });
    console.log('Desktop smoke passed: real backend/UI and transfer bytes, renderer isolation, remote request rejection and hide/show.');
    clearTimeout(watchdog);
    quit();
  } catch (error) {
    console.error(error.stack); process.exitCode = 1; clearTimeout(watchdog); quit();
  }
}, packagedResources);
