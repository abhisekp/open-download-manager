"""Create a CycloneDX 1.6 inventory of shipped Go/UI dependencies and artifacts.

Build-time npm dependencies are excluded; separately installed aria2 is not shipped.
This inventory describes components, not a claim of vulnerability/license clearance.
"""
import argparse
import datetime
import hashlib
import json
import os
import pathlib
import subprocess
import uuid
from urllib.parse import quote
from version import VERSION

ROOT = pathlib.Path(__file__).resolve().parents[1]


def json_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        text = text.lstrip()
        value, end = decoder.raw_decode(text)
        yield value
        text = text[end:]


def generate(dist):
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    modules = {}
    for system, arch in [('windows', '386'), ('windows', 'amd64'), ('windows', 'arm64'),
                         ('linux', '386'), ('linux', 'amd64'), ('linux', 'arm'), ('linux', 'arm64'), ('darwin', 'amd64'), ('darwin', 'arm64')]:
        env = {**os.environ, 'GOOS': system, 'GOARCH': arch, 'CGO_ENABLED': '0', 'GOARM': '7'}
        output = subprocess.check_output(['go', 'list', '-deps', '-json', '.'], cwd=ROOT,
                                         env=env, text=True, encoding='utf-8')
        for package in json_stream(output):
            module = package.get('Module')
            if module and not module.get('Main'):
                modules[module['Path']] = module
    components = []
    for name, module in sorted(modules.items()):
        version = module.get('Version', 'local')
        replacement = module.get('Replace', {})
        properties = []
        if replacement:
            # Only a repository-relative replacement is included, never a developer's path.
            relative = pathlib.Path(replacement['Dir']).resolve().relative_to(ROOT).as_posix()
            properties.append({'name': 'odm:local-replacement', 'value': relative})
            properties.append({'name': 'odm:replaces-module', 'value': name + '@' + version})
            name = 'github.com/abhisekp/open-download-manager/' + relative
            version = revision
        purl = f'pkg:golang/{quote(name, safe="/")}@{quote(version, safe="")}'
        components.append({'type': 'library', 'name': name, 'version': version,
                           'bom-ref': purl, 'purl': purl, 'properties': properties})
    lock = json.loads((ROOT / 'ui/package-lock.json').read_text())
    seen = set()
    # These are the reviewed imports in ui/src. Vite/plugin-react are build tools
    # despite being listed under dependencies in the current package manifest.
    pending = ['node_modules/' + name for name in ['react', 'react-dom', 'bootstrap-icons']]
    packages = lock['packages']
    runtime = set()
    while pending:
        path = pending.pop()
        if path in runtime:
            continue
        runtime.add(path)
        package = packages[path]
        for dependency in package.get('dependencies', {}):
            parent = path
            while True:
                candidate = parent + '/node_modules/' + dependency if parent else 'node_modules/' + dependency
                if candidate in packages:
                    pending.append(candidate)
                    break
                if not parent:
                    raise ValueError('Unresolved npm dependency: ' + dependency)
                parent = parent.rsplit('/node_modules/', 1)[0] if '/node_modules/' in parent else ''
    for path in sorted(runtime):
        package = packages[path]
        name = package.get('name') or path.rsplit('node_modules/', 1)[-1]
        version = package['version']
        purl = f'pkg:npm/{quote(name, safe="/")}@{quote(version, safe="")}'
        if purl in seen:
            continue
        seen.add(purl)
        component = {'type': 'library', 'name': name, 'version': version, 'bom-ref': purl, 'purl': purl}
        if package.get('license'):
            component['licenses'] = [{'expression': package['license']}]
        components.append(component)
    toolchain = subprocess.check_output(['go', 'env', 'GOVERSION'], cwd=ROOT, text=True).strip()
    components.append({'type': 'framework', 'name': 'Go standard library', 'version': toolchain,
                       'bom-ref': 'go-stdlib', 'licenses': [{'license': {'id': 'BSD-3-Clause'}}]})
    for line in (dist / 'SHA256SUMS').read_text().splitlines():
        digest, name = line.split('  ', 1)
        if pathlib.Path(name).name != name or hashlib.sha256((dist / name).read_bytes()).hexdigest() != digest:
            raise ValueError('Artifact checksum mismatch or unsafe artifact name')
        components.append({'type': 'file', 'name': name, 'bom-ref': 'artifact:' + name,
                           'hashes': [{'alg': 'SHA-256', 'content': digest}]})
    root_ref = 'odm:' + revision
    bom = {'$schema': 'http://cyclonedx.org/schema/bom-1.6.schema.json', 'bomFormat': 'CycloneDX',
           'specVersion': '1.6', 'version': 1, 'serialNumber': 'urn:uuid:' + str(uuid.uuid4()),
           'metadata': {'timestamp': datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z'),
                        'tools': {'components': [{'type': 'application', 'name': 'odm-sbom', 'version': '1'}]},
                        'component': {'type': 'application', 'name': 'Open Download Manager',
                                      'version': VERSION, 'bom-ref': root_ref,
                                      'licenses': [{'license': {'id': 'MIT'}}]},
                        'properties': [{'name': 'odm:source-commit', 'value': revision},
                                       {'name': 'odm:external-engine-bundled', 'value': 'false'},
                                       {'name': 'odm:inventory-scope', 'value': 'Resolved Go dependencies across nine targets, production npm lockfile components, Go standard library and package hashes; excludes build-only npm tools and external aria2.'}]},
           'components': components,
           'dependencies': [{'ref': root_ref, 'dependsOn': [c['bom-ref'] for c in components]}]}
    path = dist / 'odm.cdx.json'
    path.write_text(json.dumps(bom, indent=2) + '\n', encoding='utf-8')
    print(f'Created {path.name}: {len(components)} components, source {revision}')
    return bom


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--dist', type=pathlib.Path, default=ROOT / 'dist')
    generate(parser.parse_args().dist)
