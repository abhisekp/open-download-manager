'use strict';

const { app, BrowserWindow, Menu, Tray, dialog, session, nativeImage, Notification } = require('electron');
const { spawn } = require('node:child_process');
const path = require('node:path');
const fs = require('node:fs');
const { launchAddress, localResource, documentNavigation, dataPaths } = require('./policy.cjs');
const { loadPreference, savePreference, readStatus, NotificationMonitor } = require('./notifications.cjs');

// No renderer preload, Node bridge, arbitrary external navigation or shell handoff.
async function start(onReady, testResources) {
  app.enableSandbox();
  app.setName('Open Download Manager');
  let directories;
  try {
    directories = dataPaths(process.argv);
    if (directories['data-dir']) {
      const profile = path.join(directories['data-dir'], 'desktop-profile');
      fs.mkdirSync(profile, {recursive:true, mode:0o700});
      app.setPath('userData', profile);
    }
  } catch {
    await app.whenReady();
    dialog.showErrorBox('Open Download Manager', 'The data/download directory could not be used. Choose an absolute path in a writable folder.');
    app.exit(1); return;
  }
  if (!app.requestSingleInstanceLock()) { app.quit(); return; }
  let window, tray, backend, address, stopping = false, exited = false;
  let notifications;
  const liveNotifications = new Set();
  const closeNotifications = () => {
    for (const note of liveNotifications) {
      try { note.close(); } catch { /* A missing OS service must not block quit. */ }
    }
    liveNotifications.clear();
  };
  const show = () => { if (window && !window.isDestroyed()) { window.show(); window.restore(); window.focus(); } };
  app.on('second-instance', show);
  app.on('activate', show);
  const backendExit = new Promise(resolve => {
    app.once('backend-exited', resolve);
  });
  async function stop() {
    if (stopping) return;
    stopping = true;
    notifications?.setEnabled(false);
    closeNotifications();
    if (backend && !exited) {
      if (address) {
        try {
          await fetch(address.origin + '/api/quit', {
            method: 'POST', headers: { Authorization: 'Bearer ' + address.token, Origin: address.origin, 'Content-Type': 'application/json' },
            body: '{}',
            signal: AbortSignal.timeout(3000),
          });
        } catch { /* Bounded fallback below if the backend cannot answer. */ }
      }
      let timer;
      await Promise.race([backendExit, new Promise(resolve => { timer = setTimeout(resolve, 10000); })]);
      clearTimeout(timer);
      if (!exited) {
        process.exitCode = 1;
        backend.kill();
        let killTimer;
        await Promise.race([backendExit, new Promise(resolve => { killTimer = setTimeout(resolve, 2000); })]);
        clearTimeout(killTimer);
      }
    }
    tray?.destroy();
    app.exit(process.exitCode || 0);
  }
  app.on('before-quit', event => { event.preventDefault(); void stop(); });
  app.on('window-all-closed', () => { if (!stopping) app.quit(); });
  await app.whenReady();
  const root = testResources || (app.isPackaged ? process.resourcesPath : path.join(__dirname, 'resources'));
  const executable = path.join(root, 'backend', process.platform === 'win32' ? 'odm.exe' : 'odm');
  const icon = path.join(root, 'odm-icon.png');
  const fail = message => {
    if (stopping) return;
    dialog.showErrorBox('Open Download Manager', message);
    process.exitCode = 1;
    app.quit();
  };
  try {
    // Only the two directory options are accepted. Launch credentials stay in a pipe.
    backend = spawn(executable, ['--no-browser',
      '--data-dir', directories['data-dir'] || path.join(app.getPath('appData'), 'OpenDownloadManager'),
      '--downloads', directories.downloads || path.join(app.getPath('downloads'), 'OpenDownloadManager')],
    { windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] });
    backend.on('error', () => fail('The download service could not start. Reinstall the complete application package.'));
    backend.once('exit', code => {
      exited = true;
      app.emit('backend-exited');
      // Quit in the application UI also closes the native window.
      if (!stopping && address) {
        if (code !== 0) fail('The download service stopped unexpectedly. Reopen ODM to recover the saved transfers.');
        else app.quit();
      }
    });
    // Do not echo backend output: it contains the private launch credential.
    backend.stderr.resume();
    address = await new Promise((resolve, reject) => {
      let buffer = '';
      const timer = setTimeout(() => finish(new Error('startup timeout')), 20000);
      const earlyExit = () => finish(new Error('backend exited'));
      const failed = () => finish(new Error('backend unavailable'));
      const receive = chunk => {
        buffer += chunk.toString('utf8');
        if (buffer.length > 16384) { finish(new Error('invalid startup')); return; }
        let end;
        while ((end = buffer.indexOf('\n')) >= 0) {
          const line = buffer.slice(0, end); buffer = buffer.slice(end + 1);
          const found = launchAddress(line);
          if (found) { finish(null, found); return; }
        }
      };
      const finish = (error, found) => {
        clearTimeout(timer);
        backend.stdout.off('data', receive);
        backend.off('exit', earlyExit); backend.off('error', failed);
        backend.stdout.resume();
        error ? reject(error) : resolve(found);
      };
      backend.stdout.on('data', receive);
      backend.once('exit', earlyExit); backend.once('error', failed);
    });
    const isolated = session.fromPartition('odm-desktop'); // Memory-only cookies/cache.
    isolated.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
    isolated.setPermissionCheckHandler(() => false);
    isolated.on('will-download', event => event.preventDefault());
    isolated.webRequest.onBeforeRequest((details, callback) => {
      callback({ cancel: !localResource(address, details.url) });
    });
    window = new BrowserWindow({
      width: 1240, height: 820, minWidth: 800, minHeight: 580, show: false, icon,
      title: 'Open Download Manager', backgroundColor: '#f5f8fc',
      webPreferences: { session: isolated, sandbox: true, contextIsolation: true,
        nodeIntegration: false, webSecurity: true, webviewTag: false,
        allowRunningInsecureContent: false, navigateOnDragDrop: false, devTools: !app.isPackaged },
    });
    window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
    window.webContents.on('will-navigate', (event, url) => {
      if (!documentNavigation(address, url)) event.preventDefault();
    });
    window.webContents.on('will-redirect', (event, url) => {
      if (!documentNavigation(address, url)) event.preventDefault();
    });
    window.webContents.on('will-attach-webview', event => event.preventDefault());
    window.webContents.on('render-process-gone', () => fail('The application window stopped. Please reopen ODM. Your download data is retained.'));
    const preferenceFile = path.join(app.getPath('userData'), 'notifications.json');
    let notificationEnabled = false, notificationReadable = true;
    try { notificationEnabled = loadPreference(preferenceFile); } catch { notificationReadable = false; }
    notifications = new NotificationMonitor(signal => readStatus(address, signal), body => {
      if (stopping || !Notification.isSupported()) return;
      // Keep at most one live object; summaries avoid a toast for every job.
      closeNotifications();
      const note = new Notification({ title: 'Open Download Manager', body, silent: true });
      liveNotifications.add(note);
      note.on('click', show);
      note.on('close', () => liveNotifications.delete(note));
      note.on('failed', () => liveNotifications.delete(note));
      note.show();
    });
    const notificationOption = {
      label: 'Notify when downloads finish or fail', type: 'checkbox',
      checked: notificationEnabled, enabled: notificationReadable && Notification.isSupported(),
      click: item => {
        try {
          savePreference(preferenceFile, item.checked);
          notificationEnabled = item.checked;
          notifications.setEnabled(notificationEnabled);
          if (!notificationEnabled) closeNotifications();
        } catch {
          item.checked = notificationEnabled;
          dialog.showErrorBox('Open Download Manager', 'The notification preference could not be saved. Your previous setting was kept.');
        }
      },
    };
    const menu = [
      ...(process.platform === 'darwin' ? [{ label: app.name, submenu: [{ role: 'about' }, { type: 'separator' }, { label: 'Quit Open Download Manager', click: () => app.quit() }] }] : []),
      { label: 'File', submenu: [{ label: 'Show downloads', click: show }, { label: 'Quit Open Download Manager', accelerator: 'CmdOrCtrl+Q', click: () => app.quit() }] },
      { label: 'Edit', submenu: [{ role: 'undo' }, { role: 'redo' }, { type: 'separator' }, { role: 'cut' }, { role: 'copy' }, { role: 'paste' }, { role: 'selectAll' }] },
      { label: 'Settings', submenu: [notificationOption,
        { label: notificationReadable ? 'Notifications hide file names and links' : 'Notification settings unavailable; original file kept', enabled: false }] },
      { label: 'View', submenu: [{ role: 'resetZoom' }, { role: 'zoomIn' }, { role: 'zoomOut' }, { role: 'togglefullscreen' }] },
    ];
    Menu.setApplicationMenu(Menu.buildFromTemplate(menu));
    try {
      tray = new Tray(nativeImage.createFromPath(icon).resize({ width: 24, height: 24 }));
      tray.setToolTip('Open Download Manager');
      tray.setContextMenu(Menu.buildFromTemplate([
        { label: 'Show downloads', click: show },
        { label: 'Hide window (keep downloading)', click: () => window.hide() },
        { type: 'separator' }, { label: 'Quit and save downloads', click: () => app.quit() },
      ]));
      tray.on('click', show);
    } catch { /* No tray service: closing the window still quits cleanly. */ }
    await window.loadURL(address.url);
    show();
    notifications.setEnabled(notificationEnabled && notificationReadable && Notification.isSupported());
    // Test harness receives main-process objects; no renderer bridge is exposed.
    if (onReady) await onReady({ window, backend, address, show, notifications, quit: () => app.quit() });
  } catch {
    fail('ODM could not open its local download service. Another version may already be running. Close it, then try again.');
  }
}

module.exports = { start };
