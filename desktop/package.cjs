'use strict';
const path = require('node:path');
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { packager } = require('@electron/packager');
const platform = process.platform;
const arch = process.arch;
if (!['win32', 'linux', 'darwin'].includes(platform) || !['x64', 'arm64'].includes(arch)) {
  throw new Error('This desktop runtime supports x64/arm64 only. Use the portable browser-interface build on other targets.');
}
const resources = path.join(__dirname, 'resources');
// Same VERSION does not prove this backend embeds the current UI or source.
execFileSync(process.platform === 'win32' ? 'python' : 'python3',
  [path.join(__dirname, '../scripts/desktop_provenance.py'), '--verify', resources],
  {stdio:'inherit', windowsHide:true});
const version = fs.readFileSync(path.join(__dirname, '../VERSION'), 'utf8').trim();
if (require('./package.json').version !== version || require('./package-lock.json').version !== version) throw new Error('Desktop metadata must match VERSION');
for (const name of ['backend', 'odm-icon.png', 'ODM-LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt']) {
  if (!fs.existsSync(path.join(resources, name))) throw new Error('Missing prepared desktop resource: ' + name);
}
const backend = path.join(resources, 'backend', platform === 'win32' ? 'odm.exe' : 'odm');
if (execFileSync(backend, ['--version'], {encoding:'utf8', timeout:10000, windowsHide:true}).trim() !== version) throw new Error('Rebuild the backend for the current VERSION before packaging');
packager({
  // Chromium's Linux setuid launcher currently splits paths containing spaces.
  // Keep the distributed Linux directory name safe while retaining the UI brand.
  dir: __dirname, name: platform === 'linux' ? 'open-download-manager' : 'Open Download Manager', executableName: 'open-download-manager',
  platform, arch, electronVersion: require('./package.json').devDependencies.electron,
  icon: path.join(resources, platform === 'win32' ? 'odm.ico' : platform === 'darwin' ? 'odm.icns' : 'odm-icon.png'),
  appVersion: require('./package.json').version, appBundleId: 'org.opendownloadmanager.desktop',
  out: path.join(__dirname, 'out'), overwrite: true, asar: true, prune: true,
  ignore: [/^\/(?:resources|out|node_modules)(?:\/|$)/, /(?:\.test\.cjs|smoke\.cjs|package\.cjs|hardening\.cjs|package-lock\.json)$/],
  extraResource: ['backend', 'odm-icon.png', 'ODM-LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt', 'ODM-BUILD.json'].map(name => path.join(resources, name)),
}).then(async packages => {
  for (const folder of packages) {
    const hardening = await require('./hardening.cjs').harden(folder, platform);
    fs.writeFileSync(path.join(folder, 'ODM-HARDENING.json'), JSON.stringify(hardening, null, 2) + '\n');
    console.log(folder);
  }
}).catch(error => { console.error(error.message); process.exitCode = 1; });
