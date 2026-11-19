'use strict';
const path = require('node:path');
const fs = require('node:fs');
const { packager } = require('@electron/packager');
const platform = process.platform;
const arch = process.arch;
if (!['win32', 'linux', 'darwin'].includes(platform) || !['x64', 'arm64'].includes(arch)) {
  throw new Error('This desktop runtime supports x64/arm64 only. Use the portable browser-interface build on other targets.');
}
const resources = path.join(__dirname, 'resources');
for (const name of ['backend', 'odm-icon.png', 'ODM-LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt']) {
  if (!fs.existsSync(path.join(resources, name))) throw new Error('Missing prepared desktop resource: ' + name);
}
packager({
  dir: __dirname, name: 'Open Download Manager', executableName: 'open-download-manager',
  platform, arch, electronVersion: require('./package.json').devDependencies.electron,
  icon: path.join(resources, platform === 'win32' ? 'odm.ico' : platform === 'darwin' ? 'odm.icns' : 'odm-icon.png'),
  appVersion: require('./package.json').version, appBundleId: 'org.opendownloadmanager.desktop',
  out: path.join(__dirname, 'out'), overwrite: true, asar: true, prune: true,
  ignore: [/^\/(?:resources|out|node_modules)(?:\/|$)/, /(?:\.test\.cjs|smoke\.cjs|package\.cjs|package-lock\.json)$/],
  extraResource: ['backend', 'odm-icon.png', 'ODM-LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt'].map(name => path.join(resources, name)),
}).then(packages => { for (const folder of packages) console.log(folder); }).catch(error => { console.error(error.message); process.exitCode = 1; });
