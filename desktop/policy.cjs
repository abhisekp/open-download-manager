'use strict';
const path = require('node:path');

function dataPaths(argv) {
  const result = {};
  for (let index = 0; index < argv.length; index++) {
    const match = /^--(data-dir|downloads)(?:=(.*))?$/.exec(argv[index]);
    if (!match) continue;
    const value = match[2] ?? argv[++index];
    if (!value || !path.isAbsolute(value) || value.length > 4096 || /[\x00-\x1f\x7f]/.test(value) || result[match[1]]) {
      throw new Error('Choose one absolute path for each data/download directory.');
    }
    result[match[1]] = path.normalize(value);
  }
  return result;
}

function launchAddress(line) {
  const match = /^Open Download Manager: (http:\/\/127\.0\.0\.1:([0-9]{1,5})\/#([a-f0-9]{64}))$/.exec(line.trim());
  if (!match || Number(match[2]) < 1 || Number(match[2]) > 65535) return null;
  return { url: match[1], origin: new URL(match[1]).origin, token: match[3] };
}

function localResource(address, candidate) {
  try {
    const url = new URL(candidate);
    return url.origin === address.origin && !url.username && !url.password;
  } catch { return false; }
}

function documentNavigation(address, candidate) {
  if (!localResource(address, candidate)) return false;
  const url = new URL(candidate);
  return url.pathname === '/' && !url.search;
}

module.exports = { launchAddress, localResource, documentNavigation, dataPaths };
