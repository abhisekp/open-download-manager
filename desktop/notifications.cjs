'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');

// Main-process only. No transfer names, paths, URLs or error text reach the OS.
function loadPreference(file) {
  try {
    const stat = fs.lstatSync(file);
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 512) throw new Error('Invalid preferences');
    const value = JSON.parse(fs.readFileSync(file, 'utf8'));
    if (!value || value.version !== 1 || typeof value.enabled !== 'boolean' || Object.keys(value).length !== 2) throw new Error('Invalid preferences');
    return value.enabled;
  } catch (error) {
    if (error.code === 'ENOENT') return false;
    throw new Error('Notification preferences could not be read');
  }
}

function savePreference(file, enabled) {
  if (typeof enabled !== 'boolean') throw new Error('Invalid preference');
  // Refuse to overwrite a damaged or linked original file.
  loadPreference(file);
  const temporary = path.join(path.dirname(file), '.notifications-' + crypto.randomUUID());
  let fd;
  try {
    fd = fs.openSync(temporary, 'wx', 0o600);
    fs.writeFileSync(fd, JSON.stringify({ version: 1, enabled }) + '\n', 'utf8');
    fs.fsyncSync(fd);
    fs.closeSync(fd); fd = undefined;
    fs.renameSync(temporary, file);
  } finally {
    if (fd !== undefined) fs.closeSync(fd);
    try { fs.unlinkSync(temporary); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
}

async function readStatus(address, signal) {
  const response = await fetch(address.origin + '/api/status', {
    headers: { Authorization: 'Bearer ' + address.token },
    redirect: 'error', signal,
  });
  if (!response.ok) throw new Error('Status unavailable');
  const reader = response.body.getReader();
  let length = 0;
  const chunks = [];
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      length += value.length;
      if (length > 2 * 1024 * 1024) throw new Error('Status too large');
      chunks.push(Buffer.from(value));
    }
    const value = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (!Array.isArray(value.downloads) || value.downloads.length > 2048) throw new Error('Invalid status');
    const rows = value.downloads.map(row => {
      if (!row || typeof row.gid !== 'string' || row.gid.length > 128 || !row.gid ||
          !['active', 'paused', 'waiting', 'scheduled', 'seeding', 'complete', 'error', 'removed'].includes(row.status)) throw new Error('Invalid status');
      return [row.gid, row.status];
    });
    if (new Set(rows.map(([id]) => id)).size !== rows.length) throw new Error('Duplicate transfer');
    return new Map(rows);
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

class NotificationMonitor {
  constructor(read, notify, interval = 5000) {
    this.read = read; this.notify = notify; this.interval = interval;
    this.generation = 0; this.enabled = false; this.previous = null;
  }
  setEnabled(enabled) {
    this.generation++;
    this.enabled = enabled;
    clearTimeout(this.timer); this.controller?.abort();
    this.previous = null;
    if (enabled) return this.tick(this.generation);
  }
  async tick(generation) {
    clearTimeout(this.timer);
    const controller = new AbortController();
    this.controller = controller;
    const timeout = setTimeout(() => controller.abort(), 3000);
    try {
      const current = await this.read(controller.signal);
      if (!this.enabled || generation !== this.generation) return;
      let complete = 0, failed = 0;
      if (this.previous) {
        for (const [id, status] of current) {
          if (this.previous.get(id) === status) continue;
          if (status === 'complete' && this.previous.get(id) !== 'seeding') complete++;
          if (status === 'error') failed++;
        }
      }
      this.previous = current;
      const messages = [];
      if (complete) messages.push(`${complete} download${complete === 1 ? '' : 's'} completed.`);
      if (failed) messages.push(`${failed} download${failed === 1 ? '' : 's'} need${failed === 1 ? 's' : ''} attention.`);
      if (messages.length) this.notify(messages.join(' '));
    } catch { /* Status/OS failures never interrupt downloads or reveal details. */ }
    finally {
      clearTimeout(timeout);
      if (this.enabled && generation === this.generation) {
        this.timer = setTimeout(() => void this.tick(generation), this.interval);
        this.timer.unref?.();
      }
    }
  }
}

module.exports = { loadPreference, savePreference, readStatus, NotificationMonitor };
