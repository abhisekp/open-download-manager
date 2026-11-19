'use strict';

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

module.exports = { launchAddress, localResource, documentNavigation };
