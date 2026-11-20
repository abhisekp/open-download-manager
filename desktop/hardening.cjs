'use strict';
const path = require('node:path');

function applicationPath(folder, platform) {
  return platform === 'darwin' ? path.join(folder, 'Open Download Manager.app') :
    path.join(folder, platform === 'win32' ? 'open-download-manager.exe' : 'open-download-manager');
}

async function configuration(platform) {
  if (!['win32', 'linux', 'darwin'].includes(platform)) throw new Error('Unsupported desktop platform');
  const {FuseVersion, FuseV1Options: F} = await import('@electron/fuses');
  return {
    version: FuseVersion.V1,
    strictlyRequireAllFuses: true,
    resetAdHocDarwinSignature: platform === 'darwin',
    [F.RunAsNode]: false,
    // Renderer sessions are memory-only. Do not introduce a macOS Keychain
    // dependency for persistent cookies that this application does not use.
    [F.EnableCookieEncryption]: false,
    [F.EnableNodeOptionsEnvironmentVariable]: false,
    [F.EnableNodeCliInspectArguments]: false,
    [F.EnableEmbeddedAsarIntegrityValidation]: platform !== 'linux',
    [F.OnlyLoadAppFromAsar]: true,
    // Prebuilt Electron ships the shared snapshot, not the optional custom one.
    [F.LoadBrowserProcessSpecificV8Snapshot]: false,
    [F.GrantFileProtocolExtraPrivileges]: false,
    [F.WasmTrapHandlers]: true,
  };
}

async function verifyHardening(folder, platform = process.platform) {
  const {getCurrentFuseWire, FuseState} = await import('@electron/fuses');
  const expected = await configuration(platform);
  const wire = await getCurrentFuseWire(applicationPath(folder, platform));
  if (wire.version !== expected.version) throw new Error('Unexpected native fuse schema');
  const keys = Object.keys(wire).filter(key => /^\d+$/.test(key));
  const wanted = Object.keys(expected).filter(key => /^\d+$/.test(key));
  if (keys.length !== wanted.length) throw new Error('Review new or missing native fuse options');
  for (const key of wanted) {
    const state = expected[key] ? FuseState.ENABLE : FuseState.DISABLE;
    if (wire[key] !== state) throw new Error('Native hardening read-back failed for fuse ' + key);
  }
  return {schema: wire.version, runAsNode: false, nodeOptions: false, nodeInspector: false,
    onlyAsar: true, asarIntegrity: platform !== 'linux', fileProtocolExtraPrivileges: false};
}

async function harden(folder, platform = process.platform) {
  const {flipFuses} = await import('@electron/fuses');
  await flipFuses(applicationPath(folder, platform), await configuration(platform));
  return verifyHardening(folder, platform);
}

module.exports = {harden, verifyHardening, applicationPath};
