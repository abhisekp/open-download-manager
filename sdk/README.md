# ODM add-on SDK — API 1

Original SDK code is MIT licensed. API 1 currently supports `url-resolver` modules using WASI Preview 1. This is separate from browser companion extensions and the legacy built-in tracker feed. It does not yet provide provider network requests, arbitrary UI contributions, storage, event subscriptions or automatic updates.

## Contract

The module receives one JSON request on stdin:

```json
{"api":1,"operation":"resolve","url":"https://example.com/file.zip?utm_source=test"}
```

It writes exactly one JSON response to stdout, then exits successfully:

```json
{"urls":["https://example.com/file.zip"]}
```

Return between 1 and 50 HTTP/HTTPS URLs. Input and output hosts must match the manifest's explicit host lists; no wildcards, credentials, control characters or other schemes are accepted. Hosts cover all ports on that host. User-provided query parameters are visible to the add-on. Resolution alone never starts a download: the user reviews results before adding them.

Build a WASI Preview 1 module in a compatible language. The Go example uses only standard-library JSON/URL handling:

```sh
GOOS=wasip1 GOARCH=wasm go build -o /outside-source/clean-link.wasm ./sdk/examples/clean-link
cat LICENSE "$(go env GOROOT)/LICENSE" > /outside-source/LICENSE
go run ./sdk/cmd/odm-addon-pack --generate-key /outside-source/publisher.key
go run ./sdk/cmd/odm-addon-pack --manifest sdk/examples/clean-link/manifest.json --module /outside-source/clean-link.wasm --key /outside-source/publisher.key --license /outside-source/LICENSE --out /outside-source/clean-link.odmx
```

On PowerShell, set `$env:GOOS='wasip1'` and `$env:GOARCH='wasm'` for the module build, then remove those environment variables before running the host packaging command. Choose a real writable directory outside source in place of `/outside-source`. Never commit the generated private key. The packer refuses to overwrite keys/packages and replaces the manifest module hash in the packaged copy.

The manifest declares API version, stable ID, display name, numeric `major.minor.patch` version, description, `url-resolver` capability, `input_hosts`, `output_hosts` `module_sha256` and `license_sha256`. The packer creates exactly five ZIP entries: manifest JSON, module, Ed25519 public key, signature and LICENSE. The signature covers the exact manifest bytes and the manifest binds both module and license digests. Provide the complete license text and compiled dependency notices for your module; packaging does not grant redistribution rights. The example combines original MIT and Go standard-library notices. A valid signature establishes key continuity, not a verified organization or safe code. The UI displays both publisher and package fingerprints for trust review.

## Lifecycle

Choose the package under Extensions, review its hosts/key, approve that exact package, install, then enable. Grants persist locally. An update from the same key requires another exact-package approval and disables the previous grant. A different key cannot replace an installed ID; uninstalling is a deliberate trust reset. Installing different content under the same version is rejected. Uninstall removes the grant and cached package. No extension runs at startup, on a timer or automatically in response to browser submissions.

## Isolation and limits

Each invocation starts an isolated ODM worker process containing the wazero interpreter. WASI receives no filesystem mounts, inherited environment, network sockets or subprocess imports. The core verifies installed package integrity before each invocation. Two workers may run concurrently, with a five-second overall deadline, a two-second VM deadline, 64 MiB maximum WASM linear memory and 256 KiB output per stream. Packages/uncompressed entries are bounded to 8 MiB; installation is limited to 16 add-ons. Worker startup receives only a Windows system path where necessary and Go runtime resource settings.

The worker's 128 MiB Go memory target is a soft garbage-collection target, **not an OS-enforced RSS limit**. OS job/cgroup enforcement and deeper independent runtime hardening remain production gates. Revocation waits for an already-running bounded invocation to finish; once revocation returns, no subsequent invocation is allowed. No claim of arbitrary-code safety beyond these configured and tested boundaries is made.

## Verification

`go test -race ./...` builds and runs real WASI fixtures, checks signature/digest tampering, publisher continuity, update grant revocation, persistence, input/output permissions, filesystem/network/environment denial, memory rejection and runaway-module cancellation. `python scripts/addons_test.py --exe <ODM executable>` builds and signs a local fixture, installs and invokes it through the authenticated API, performs an explicitly confirmed real transfer through aria2, then revokes and uninstalls the extension. This integration test needs aria2 installed.

The sandbox-probe example is a test fixture, not a useful end-user add-on. Full source-provider fetching and richer extension APIs remain separate milestones.
