'use strict';
// External test driver: launches the actual renamed executable with a disposable profile.
// Debugging is enabled only on this test invocation, bound to loopback, never in product code.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const http = require('node:http');
const {spawn} = require('node:child_process');
const {setTimeout: delay} = require('node:timers/promises');

class DebugPage {
  constructor(url) {
    this.socket = new WebSocket(url); this.pending = new Map(); this.id = 0;
    this.ready = new Promise((resolve, reject) => { this.socket.addEventListener('open', resolve, {once:true}); this.socket.addEventListener('error', () => reject(new Error('Window inspection unavailable')), {once:true}); });
    this.socket.addEventListener('message', event => {
      const message = JSON.parse(event.data), item = this.pending.get(message.id);
      if (!item) return;
      this.pending.delete(message.id); clearTimeout(item.timer);
      message.error ? item.reject(new Error('Window inspection command failed')) : item.resolve(message.result);
    });
  }
  async command(method, params) {
    await this.ready;
    const id = ++this.id;
    const result = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error('Window inspection timed out')); }, 5000);
      this.pending.set(id, {resolve, reject, timer});
      this.socket.send(JSON.stringify({id,method,params}));
    });
    return result;
  }
  async evaluate(expression) {
    const result = await this.command('Runtime.evaluate', {expression,returnByValue:true,awaitPromise:true});
    if (result.exceptionDetails) throw new Error('Application window evaluation failed');
    return result.result.value;
  }
  close() { this.socket.close(); }
}

async function run() {
  const folders = fs.readdirSync(path.join(__dirname, 'out'));
  assert.equal(folders.length, 1);
  const folder = path.join(__dirname, 'out', folders[0]);
  const executable = process.platform === 'darwin' ? path.join(folder, 'Open Download Manager.app/Contents/MacOS/open-download-manager') : path.join(folder, process.platform === 'win32' ? 'open-download-manager.exe' : 'open-download-manager');
  const scratch = fs.mkdtempSync(path.join(os.tmpdir(), 'odm-package-'));
  const data = path.join(scratch, 'data'), files = path.join(scratch, 'files');
  let child, page, fixture, exited = false, exitCode, phase = 'runtime startup';
  const watchdog = setTimeout(() => { console.error('Packaged application test timed out'); child?.kill(); process.exit(1); }, 60000);
  try {
    child = spawn(executable, ['--data-dir', data, '--downloads', files, '--remote-debugging-port=0', '--remote-debugging-address=127.0.0.1'], {windowsHide:true,stdio:['ignore','ignore','pipe']});
    const exit = new Promise(resolve => child.once('exit', code => { exited = true; exitCode = code; resolve(); }));
    const port = await new Promise((resolve, reject) => {
      let buffer = '';
      const timer = setTimeout(() => reject(new Error('Packaged runtime did not expose its test window')), 20000);
      child.once('error', () => { clearTimeout(timer); reject(new Error('Packaged executable could not start')); });
      child.once('exit', () => { clearTimeout(timer); reject(new Error('Packaged executable exited before startup')); });
      child.stderr.on('data', chunk => {
        buffer = (buffer + chunk.toString('utf8')).slice(-16384);
        const match = /DevTools listening on ws:\/\/127\.0\.0\.1:([0-9]+)\//.exec(buffer);
        if (match) { clearTimeout(timer); resolve(Number(match[1])); }
      });
    });
    let target;
    phase = 'waiting for the application window';
    for (let attempt = 0; attempt < 150 && !exited; attempt++) {
      let targets;
      try {
        targets = await fetch(`http://127.0.0.1:${port}/json/list`, {signal:AbortSignal.timeout(2000)}).then(reply => reply.json());
      } catch { await delay(100); continue; }
      target = targets.find(item => item.type === 'page' && /^http:\/\/127\.0\.0\.1:[0-9]+\//.test(item.url));
      if (target) break;
      await delay(100);
    }
    assert.ok(target, 'packaged window loads the owned backend');
    page = new DebugPage(target.webSocketDebuggerUrl);
    phase = 'checking the application window';
    let state;
    for (let attempt = 0; attempt < 100; attempt++) {
      state = await page.evaluate('(() => { let token = ""; try { token = sessionStorage.getItem("odm-token"); } catch {} return {origin:location.origin,token,text:document.body?.innerText || "",node:typeof require}; })()');
      if (state.token && /Open Download\s+Manager/.test(state.text)) break;
      await delay(100);
    }
    assert.ok(/^[a-f0-9]{64}$/.test(state.token || ''), 'private application session is ready');
    assert.match(state.text, /Open Download\s+Manager/);
    assert.equal(state.node, 'undefined');
    const headers = {Authorization:'Bearer '+state.token, Origin:state.origin, 'Content-Type':'application/json'};
    const payload = Buffer.from('packaged executable transfer fixture\n'.repeat(4096));
    fixture = http.createServer((_request, reply) => { reply.writeHead(200, {'Content-Length':payload.length}); reply.end(payload); });
    await new Promise(resolve => fixture.listen(0, '127.0.0.1', resolve));
    phase = 'adding the local transfer';
    const add = await fetch(state.origin + '/api/add', {method:'POST',headers,body:JSON.stringify({url:`http://127.0.0.1:${fixture.address().port}/packaged.bin`}),signal:AbortSignal.timeout(5000)});
    assert.equal(add.status, 200);
    const {result:gid} = await add.json();
    let completed;
    phase = 'checking the transfer';
    for (let attempt = 0; attempt < 150; attempt++) {
      const status = await fetch(state.origin + '/api/status', {headers,signal:AbortSignal.timeout(5000)}).then(reply => reply.json());
      completed = status.downloads.find(item => item.gid === gid && item.status === 'complete');
      if (completed) break;
      await delay(100);
    }
    assert.ok(completed, 'packaged executable transfer completes');
    assert.deepEqual(fs.readFileSync(completed.files[0].path), payload);
    if (process.env.ODM_DESKTOP_SCREENSHOT) {
      phase = 'capturing the window';
      const image = await page.command('Page.captureScreenshot', {format:'png'});
      fs.writeFileSync(process.env.ODM_DESKTOP_SCREENSHOT, Buffer.from(image.data, 'base64'));
    }
    page.close(); page = null;
    phase = 'graceful shutdown';
    const quit = await fetch(state.origin + '/api/quit', {method:'POST',headers,body:'{}',signal:AbortSignal.timeout(5000)});
    assert.equal(quit.status, 200);
    let quitTimer;
    try { await Promise.race([exit, new Promise(resolve => { quitTimer = setTimeout(resolve, 15000); })]); }
    finally { clearTimeout(quitTimer); }
    assert.ok(exited, 'packaged executable quits');
    assert.equal(exitCode, 0);
    assert.equal(fs.existsSync(path.join(data, 'running.lock')), false);
    assert.ok(fs.existsSync(path.join(data, 'http-transfers.json')));
    console.log('PASS: actual packaged runtime launch, isolated UI, real transfer bytes, saved state and clean exit.');
  } catch (error) {
    throw new Error(phase + ': ' + error.message);
  } finally {
    clearTimeout(watchdog);
    page?.close(); fixture?.closeAllConnections(); fixture?.close();
    if (child && !exited) child.kill();
    // Only this freshly allocated temporary profile may be removed.
    assert.equal(path.dirname(scratch), path.resolve(os.tmpdir()));
    assert.ok(path.basename(scratch).startsWith('odm-package-'));
    if (exited) fs.rmSync(scratch, {recursive:true,force:true,maxRetries:5,retryDelay:200});
  }
}
run().catch(error => { console.error(error.message); process.exitCode = 1; });
