'use strict';
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { listPackage, extractFile } = require('@electron/asar');
const archives = [];
function visit(folder) {
  for (const item of fs.readdirSync(folder, {withFileTypes:true})) {
    const filename = path.join(folder, item.name);
    if (item.isDirectory()) visit(filename);
    else if (item.name === 'app.asar') archives.push(filename);
  }
}
visit(path.join(__dirname, 'out'));
assert.equal(archives.length, 1, 'exactly one native package');
const archive = archives[0];
const entries = listPackage(archive).map(name => name.replaceAll('\\', '/')).sort();
assert.deepEqual(entries, ['/app.cjs', '/main.cjs', '/package.json', '/policy.cjs']);
for (const name of ['app.cjs', 'main.cjs', 'policy.cjs']) {
  assert.deepEqual(extractFile(archive, name), fs.readFileSync(path.join(__dirname, name)));
}
const resources = path.dirname(archive);
for (const name of ['ODM-LICENSE.txt', 'THIRD-PARTY-NOTICES.txt', 'THIRD-PARTY-GO-NOTICES.txt', 'odm-icon.png']) {
  assert.deepEqual(fs.readFileSync(path.join(resources, name)), fs.readFileSync(path.join(__dirname, 'resources', name)));
}
const runtime = process.platform === 'darwin' ? path.resolve(resources, '../../..') : path.dirname(resources);
// Upstream runtime notices must survive packaging alongside the Mac app bundle.
for (const name of ['LICENSE', 'LICENSES.chromium.html']) {
  assert.ok(fs.existsSync(path.join(runtime, name)), 'runtime notice: ' + name);
}
console.log('Native package content verified: exact application sources and licenses; no SDK, tests or development dependencies.');
// The following argument is for the separate test harness, never the product entry point.
console.log(resources);
